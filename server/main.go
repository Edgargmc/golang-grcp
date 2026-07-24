package main

import (
	"context"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "grpc-todo/gen"
)

// Token fijo solo para fines didácticos. En un caso real esto vendría
// de una base de datos, un JWT firmado, o un proveedor de identidad.
const authToken = "secret-token-123"

type todoServer struct {
	pb.UnimplementedTodoServiceServer

	repo TodoRepository

	publisher EventPublisher
	events    EventSubscriber
}

func newTodoServer(repo TodoRepository) *todoServer {
	b := newBroadcaster()
	return &todoServer{
		repo:      repo,
		publisher: b,
		events:    b,
	}
}

func (s *todoServer) CreateTodo(ctx context.Context, req *pb.CreateTodoRequest) (*pb.CreateTodoResponse, error) {
	if strings.TrimSpace(req.GetTitle()) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "title is required")
	}

	item := &pb.TodoItem{
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Completed:   req.GetCompleted(),
		CreatedAt:   time.Now().Format(time.RFC3339),
	}
	if err := s.repo.Insert(item); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save todo: %v", err)
	}

	s.publisher.Publish(&pb.TodoEvent{Type: "created", Todo: item})

	return &pb.CreateTodoResponse{
		Id:          item.Id,
		Title:       item.Title,
		Description: item.Description,
		Completed:   item.Completed,
		CreatedAt:   item.CreatedAt,
	}, nil
}

func (s *todoServer) GetTodo(ctx context.Context, req *pb.GetTodoRequest) (*pb.GetTodoResponse, error) {
	item, ok := s.repo.FindByID(req.GetId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "todo with id %d not found", req.GetId())
	}

	return &pb.GetTodoResponse{
		Id:          item.Id,
		Title:       item.Title,
		Description: item.Description,
		Completed:   item.Completed,
		CreatedAt:   item.CreatedAt,
	}, nil
}

func (s *todoServer) ListTodos(ctx context.Context, req *pb.ListTodosRequest) (*pb.ListTodosResponse, error) {
	page := req.GetPage()
	limit := req.GetLimit()
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	all := s.repo.FindAll()
	sort.Slice(all, func(i, j int) bool { return all[i].Id < all[j].Id })

	total := int32(len(all))
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	return &pb.ListTodosResponse{
		Todos: all[start:end],
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

func (s *todoServer) UpdateTodo(ctx context.Context, req *pb.UpdateTodoRequest) (*pb.UpdateTodoResponse, error) {
	if strings.TrimSpace(req.GetTitle()) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "title is required")
	}

	item, ok := s.repo.FindByID(req.GetId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "todo with id %d not found", req.GetId())
	}

	item.Title = req.GetTitle()
	item.Description = req.GetDescription()
	item.Completed = req.GetCompleted()

	if err := s.repo.Save(item); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save todo: %v", err)
	}

	s.publisher.Publish(&pb.TodoEvent{Type: "updated", Todo: item})

	return &pb.UpdateTodoResponse{
		Id:          item.Id,
		Title:       item.Title,
		Description: item.Description,
		Completed:   item.Completed,
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}, nil
}

func (s *todoServer) DeleteTodo(ctx context.Context, req *pb.DeleteTodoRequest) (*pb.DeleteTodoResponse, error) {
	item, ok := s.repo.Delete(req.GetId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "todo with id %d not found", req.GetId())
	}

	s.publisher.Publish(&pb.TodoEvent{Type: "deleted", Todo: item})

	return &pb.DeleteTodoResponse{Deleted: true}, nil
}

func (s *todoServer) CompleteTodo(ctx context.Context, req *pb.CompleteTodoRequest) (*pb.CompleteTodoResponse, error) {
	item, ok := s.repo.FindByID(req.GetId())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "todo with id %d not found", req.GetId())
	}
	item.Completed = true

	if err := s.repo.Save(item); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save todo: %v", err)
	}

	s.publisher.Publish(&pb.TodoEvent{Type: "completed", Todo: item})

	return &pb.CompleteTodoResponse{Id: item.Id, Completed: item.Completed}, nil
}

func (s *todoServer) WatchTodos(req *pb.WatchTodosRequest, stream pb.TodoService_WatchTodosServer) error {
	id, events := s.events.Subscribe()
	defer s.events.Unsubscribe(id)

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()

	resp, err := handler(ctx, req)

	duration := time.Since(start)
	if err != nil {
		log.Printf("[%s] failed in %s: %v", info.FullMethod, duration, err)
	} else {
		log.Printf("[%s] ok in %s", info.FullMethod, duration)
	}

	return resp, err
}

func authInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.Health/") {
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	tokens := md.Get("authorization")
	if len(tokens) == 0 || tokens[0] != authToken {
		return nil, status.Error(codes.Unauthenticated, "invalid or missing token")
	}

	return handler(ctx, req)
}

func streamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if strings.HasPrefix(info.FullMethod, "/grpc.reflection.") ||
		strings.HasPrefix(info.FullMethod, "/grpc.health.v1.Health/") {
		return handler(srv, ss)
	}

	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	tokens := md.Get("authorization")
	if len(tokens) == 0 || tokens[0] != authToken {
		return status.Error(codes.Unauthenticated, "invalid or missing token")
	}

	return handler(srv, ss)
}

func main() {
	var repo TodoRepository
	var err error

	tursoURL := os.Getenv("TURSO_DATABASE_URL")
	tursoToken := os.Getenv("TURSO_AUTH_TOKEN")

	if tursoURL != "" && tursoToken != "" {
		log.Println("Usando Turso como base de datos")
		repo, err = newTursoRepository(tursoURL, tursoToken)
	} else {
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "todos.db"
		}
		log.Printf("Usando SQLite local (%s)", dbPath)
		repo, err = newSQLiteRepository(dbPath)
	}

	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(loggingInterceptor, authInterceptor),
		grpc.ChainStreamInterceptor(streamAuthInterceptor),
	)
	pb.RegisterTodoServiceServer(s, newTodoServer(repo))
	reflection.Register(s)

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(s, healthServer)

	log.Println("gRPC Todo server listening on port 50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
