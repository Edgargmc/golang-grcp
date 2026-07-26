package main

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "grpc-todo/gen"
)

const bufSize = 1024 * 1024

// newTestServer levanta el servidor real (mismos interceptors que producción)
// sobre bufconn, sin abrir ningún puerto TCP real, y devuelve tanto el
// *todoServer (por si un test necesita inspeccionar su estado interno)
// como el cliente ya conectado.
func newTestServer(t *testing.T) (*todoServer, pb.TodoServiceClient) {
	t.Helper()

	lis := bufconn.Listen(bufSize)

	srv := newTodoServer(newInMemoryRepository())

	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(loggingInterceptor, authInterceptor),
		grpc.ChainStreamInterceptor(streamAuthInterceptor),
	)
	pb.RegisterTodoServiceServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()
	t.Cleanup(s.Stop)

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return srv, pb.NewTodoServiceClient(conn)
}

// newTestClient es un atajo para los tests que no necesitan tocar el
// *todoServer directamente, la mayoría.
func newTestClient(t *testing.T) pb.TodoServiceClient {
	t.Helper()
	_, client := newTestServer(t)
	return client
}

// authContext agrega el mismo token que exige authInterceptor.
func authContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", authToken)
}

func TestCreateTodo_Success(t *testing.T) {
	client := newTestClient(t)
	ctx := authContext(context.Background())

	resp, err := client.CreateTodo(ctx, &pb.CreateTodoRequest{
		Title:       "Test desde bufconn",
		Description: "sin red real",
	})
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}

	if resp.Title != "Test desde bufconn" {
		t.Errorf("expected title %q, got %q", "Test desde bufconn", resp.Title)
	}
	if resp.Id == 0 {
		t.Errorf("expected non-zero id, got 0")
	}
}

func TestCreateTodo_EmptyTitle_ReturnsInvalidArgument(t *testing.T) {
	client := newTestClient(t)
	ctx := authContext(context.Background())

	_, err := client.CreateTodo(ctx, &pb.CreateTodoRequest{Title: "   "})
	if err == nil {
		t.Fatal("expected an error for empty title, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected code %v, got %v", codes.InvalidArgument, st.Code())
	}
}

func TestCreateTodo_MissingToken_ReturnsUnauthenticated(t *testing.T) {
	client := newTestClient(t)

	// A propósito NO uso authContext acá: sin metadata de auth.
	_, err := client.CreateTodo(context.Background(), &pb.CreateTodoRequest{Title: "Sin token"})
	if err == nil {
		t.Fatal("expected an error without an auth token, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected code %v, got %v", codes.Unauthenticated, st.Code())
	}
}

func TestListTodos_ExpiredDeadline_ReturnsDeadlineExceeded(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithTimeout(authContext(context.Background()), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // asegurar que el deadline ya venció al momento de llamar

	_, err := client.ListTodos(ctx, &pb.ListTodosRequest{Page: 1, Limit: 10})
	if err == nil {
		t.Fatal("expected a deadline exceeded error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.DeadlineExceeded {
		t.Errorf("expected code %v, got %v", codes.DeadlineExceeded, st.Code())
	}
}

func TestWatchTodos_DeadlineExceeded_ClosesStream(t *testing.T) {
	client := newTestClient(t)

	watchCtx, cancel := context.WithTimeout(authContext(context.Background()), 200*time.Millisecond)
	defer cancel()

	stream, err := client.WatchTodos(watchCtx, &pb.WatchTodosRequest{})
	if err != nil {
		t.Fatalf("WatchTodos failed: %v", err)
	}

	start := time.Now()
	_, err = stream.Recv() // no hay eventos: se queda esperando hasta que venza el deadline
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when the deadline expires, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.DeadlineExceeded {
		t.Errorf("expected code %v, got %v", codes.DeadlineExceeded, st.Code())
	}

	// A diferencia del paso 1, acá el request SÍ llegó al servidor y se quedó
	// esperando ahí: confirmamos que tardó ~200ms, no que falló al instante.
	if elapsed < 150*time.Millisecond {
		t.Errorf("expected to wait close to the deadline, only waited %v", elapsed)
	}
}

func TestWatchTodos_DeadlineExceeded_UnsubscribesListener(t *testing.T) {
	srv, client := newTestServer(t)
	b := srv.events.(*broadcaster)

	watchCtx, cancel := context.WithTimeout(authContext(context.Background()), 200*time.Millisecond)
	defer cancel()

	stream, err := client.WatchTodos(watchCtx, &pb.WatchTodosRequest{})
	if err != nil {
		t.Fatalf("WatchTodos failed: %v", err)
	}

	// El Subscribe() del lado servidor corre en su propio goroutine: darle
	// un instante a que se registre antes de seguir.
	waitUntil(t, 500*time.Millisecond, func() bool { return b.listenerCount() == 1 })

	// Se queda esperando hasta que el deadline corte el stream solo.
	if _, err := stream.Recv(); err == nil {
		t.Fatal("expected the stream to fail once the deadline expires")
	}

	// El defer Unsubscribe() del handler también corre en su propio goroutine.
	waitUntil(t, 500*time.Millisecond, func() bool { return b.listenerCount() == 0 })

	if got := b.listenerCount(); got != 0 {
		t.Errorf("expected 0 listeners after disconnect, got %d (posible fuga)", got)
	}
}

// waitUntil sondea condition() hasta que sea true o venza el timeout,
// para no depender de sleeps fijos en asserts sobre goroutines concurrentes.
func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// assertNotFound centraliza el chequeo repetido en los 4 tests de abajo.
func assertNotFound(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected a NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected a gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("expected code %v, got %v", codes.NotFound, st.Code())
	}
}

func TestGetTodo_UnknownId_ReturnsNotFound(t *testing.T) {
	client := newTestClient(t)
	ctx := authContext(context.Background())

	_, err := client.GetTodo(ctx, &pb.GetTodoRequest{Id: 999})
	assertNotFound(t, err)
}

func TestUpdateTodo_UnknownId_ReturnsNotFound(t *testing.T) {
	client := newTestClient(t)
	ctx := authContext(context.Background())

	_, err := client.UpdateTodo(ctx, &pb.UpdateTodoRequest{Id: 999, Title: "cualquiera"})
	assertNotFound(t, err)
}

func TestDeleteTodo_UnknownId_ReturnsNotFound(t *testing.T) {
	client := newTestClient(t)
	ctx := authContext(context.Background())

	_, err := client.DeleteTodo(ctx, &pb.DeleteTodoRequest{Id: 999})
	assertNotFound(t, err)
}

func TestCompleteTodo_UnknownId_ReturnsNotFound(t *testing.T) {
	client := newTestClient(t)
	ctx := authContext(context.Background())

	_, err := client.CompleteTodo(ctx, &pb.CompleteTodoRequest{Id: 999})
	assertNotFound(t, err)
}

func TestWatchTodos_ReceivesCreatedEvent(t *testing.T) {
	client := newTestClient(t)

	watchCtx, cancel := context.WithTimeout(authContext(context.Background()), 2*time.Second)
	defer cancel()

	stream, err := client.WatchTodos(watchCtx, &pb.WatchTodosRequest{})
	if err != nil {
		t.Fatalf("WatchTodos failed: %v", err)
	}

	events := make(chan *pb.TodoEvent, 1)
	errs := make(chan error, 1)
	go func() {
		event, err := stream.Recv()
		if err != nil {
			errs <- err
			return
		}
		events <- event
	}()

	// Le doy un instante al goroutine de arriba para que el servidor
	// termine de suscribirlo antes de publicar el evento.
	time.Sleep(50 * time.Millisecond)

	created, err := client.CreateTodo(authContext(context.Background()), &pb.CreateTodoRequest{
		Title: "Evento de test",
	})
	if err != nil {
		t.Fatalf("CreateTodo failed: %v", err)
	}

	select {
	case event := <-events:
		if event.Type != "created" {
			t.Errorf("expected type %q, got %q", "created", event.Type)
		}
		if event.Todo.Id != created.Id {
			t.Errorf("expected todo id %d, got %d", created.Id, event.Todo.Id)
		}
	case err := <-errs:
		t.Fatalf("stream.Recv failed: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSyncTodos_SendCreate_ReceivesCreatedEventBack(t *testing.T) {
	client := newTestClient(t)

	syncCtx, cancel := context.WithTimeout(authContext(context.Background()), 2*time.Second)
	defer cancel()

	stream, err := client.SyncTodos(syncCtx)
	if err != nil {
		t.Fatalf("SyncTodos failed: %v", err)
	}

	err = stream.Send(&pb.TodoChange{
		Operation: &pb.TodoChange_Create{
			Create: &pb.CreateTodoRequest{Title: "Creado via SyncTodos"},
		},
	})
	if err != nil {
		t.Fatalf("stream.Send failed: %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv failed: %v", err)
	}
	if event.Type != "created" {
		t.Errorf("expected type %q, got %q", "created", event.Type)
	}
	if event.Todo.Title != "Creado via SyncTodos" {
		t.Errorf("expected title %q, got %q", "Creado via SyncTodos", event.Todo.Title)
	}
}

// El client_id es lo que le permite a un cliente reconciliar un create
// optimista (hecho con un id temporal, típicamente offline) contra el id
// real que el servidor recién asigna acá.
func TestSyncTodos_SendCreateWithClientId_EchoesItBack(t *testing.T) {
	client := newTestClient(t)

	syncCtx, cancel := context.WithTimeout(authContext(context.Background()), 2*time.Second)
	defer cancel()

	stream, err := client.SyncTodos(syncCtx)
	if err != nil {
		t.Fatalf("SyncTodos failed: %v", err)
	}

	err = stream.Send(&pb.TodoChange{
		Operation: &pb.TodoChange_Create{
			Create: &pb.CreateTodoRequest{Title: "Con client_id"},
		},
		ClientId: "temp-123",
	})
	if err != nil {
		t.Fatalf("stream.Send failed: %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv failed: %v", err)
	}
	if event.ClientId != "temp-123" {
		t.Errorf("expected client_id %q, got %q", "temp-123", event.ClientId)
	}
}

// Un cambio SIN client_id (el caso normal, ej. desde Python/grpcurl/otro
// Android sin este feature) no debe traer uno inventado.
func TestSyncTodos_SendCreateWithoutClientId_EventHasEmptyClientId(t *testing.T) {
	client := newTestClient(t)

	syncCtx, cancel := context.WithTimeout(authContext(context.Background()), 2*time.Second)
	defer cancel()

	stream, err := client.SyncTodos(syncCtx)
	if err != nil {
		t.Fatalf("SyncTodos failed: %v", err)
	}

	err = stream.Send(&pb.TodoChange{
		Operation: &pb.TodoChange_Create{Create: &pb.CreateTodoRequest{Title: "Sin client_id"}},
	})
	if err != nil {
		t.Fatalf("stream.Send failed: %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv failed: %v", err)
	}
	if event.ClientId != "" {
		t.Errorf("expected empty client_id, got %q", event.ClientId)
	}
}

// Una operación inválida (título vacío) no debe tirar abajo el stream: el
// cliente recibe un evento type="error" con el motivo, y un cambio válido
// mandado después debe seguir funcionando con normalidad.
func TestSyncTodos_InvalidChange_ReceivesErrorEventAndKeepsWorking(t *testing.T) {
	client := newTestClient(t)

	syncCtx, cancel := context.WithTimeout(authContext(context.Background()), 2*time.Second)
	defer cancel()

	stream, err := client.SyncTodos(syncCtx)
	if err != nil {
		t.Fatalf("SyncTodos failed: %v", err)
	}

	if err := stream.Send(&pb.TodoChange{
		Operation: &pb.TodoChange_Create{Create: &pb.CreateTodoRequest{Title: "   "}},
	}); err != nil {
		t.Fatalf("stream.Send (invalid) failed: %v", err)
	}

	errEvent, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv (error event) failed: %v", err)
	}
	if errEvent.Type != "error" {
		t.Errorf("expected type %q, got %q", "error", errEvent.Type)
	}
	if errEvent.Error == "" {
		t.Errorf("expected a non-empty error message")
	}

	if err := stream.Send(&pb.TodoChange{
		Operation: &pb.TodoChange_Create{Create: &pb.CreateTodoRequest{Title: "Este sí vale"}},
	}); err != nil {
		t.Fatalf("stream.Send (valid) failed: %v", err)
	}

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv failed: %v", err)
	}
	if event.Todo.Title != "Este sí vale" {
		t.Errorf("expected only the valid change to produce an event, got title %q", event.Todo.Title)
	}
}
