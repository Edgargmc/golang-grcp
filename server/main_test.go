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

// newTestClient levanta el servidor real (mismos interceptors que producción)
// sobre bufconn, sin abrir ningún puerto TCP real.
func newTestClient(t *testing.T) pb.TodoServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)

	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(loggingInterceptor, authInterceptor),
		grpc.ChainStreamInterceptor(streamAuthInterceptor),
	)
	pb.RegisterTodoServiceServer(s, newTodoServer())

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

	return pb.NewTodoServiceClient(conn)
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
