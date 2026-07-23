package main

import (
	"context"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	pb "grpc-todo/gen"
)

type todoServer struct {
	pb.UnimplementedTodoServiceServer

	mu     sync.Mutex
	todos  map[int32]*pb.TodoItem
	nextID int32
}

func newTodoServer() *todoServer {
	return &todoServer{
		todos:  make(map[int32]*pb.TodoItem),
		nextID: 1,
	}
}

func (s *todoServer) CreateTodo(ctx context.Context, req *pb.CreateTodoRequest) (*pb.CreateTodoResponse, error) {
	if strings.TrimSpace(req.GetTitle()) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "title is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item := &pb.TodoItem{
		Id:          s.nextID,
		Title:       req.GetTitle(),
		Description: req.GetDescription(),
		Completed:   req.GetCompleted(),
		CreatedAt:   time.Now().Format(time.RFC3339),
	}
	s.todos[item.Id] = item
	s.nextID++

	return &pb.CreateTodoResponse{
		Id:          item.Id,
		Title:       item.Title,
		Description: item.Description,
		Completed:   item.Completed,
		CreatedAt:   item.CreatedAt,
	}, nil
}

func (s *todoServer) GetTodo(ctx context.Context, req *pb.GetTodoRequest) (*pb.GetTodoResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.todos[req.GetId()]
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
	s.mu.Lock()
	defer s.mu.Unlock()

	page := req.GetPage()
	limit := req.GetLimit()
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	ids := make([]int32, 0, len(s.todos))
	for id := range s.todos {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	total := int32(len(ids))
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	todos := make([]*pb.TodoItem, 0, end-start)
	for _, id := range ids[start:end] {
		todos = append(todos, s.todos[id])
	}

	return &pb.ListTodosResponse{
		Todos: todos,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

func (s *todoServer) UpdateTodo(ctx context.Context, req *pb.UpdateTodoRequest) (*pb.UpdateTodoResponse, error) {
	if strings.TrimSpace(req.GetTitle()) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "title is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.todos[req.GetId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "todo with id %d not found", req.GetId())
	}

	item.Title = req.GetTitle()
	item.Description = req.GetDescription()
	item.Completed = req.GetCompleted()

	return &pb.UpdateTodoResponse{
		Id:          item.Id,
		Title:       item.Title,
		Description: item.Description,
		Completed:   item.Completed,
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}, nil
}

func (s *todoServer) DeleteTodo(ctx context.Context, req *pb.DeleteTodoRequest) (*pb.DeleteTodoResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.todos[req.GetId()]; !ok {
		return nil, status.Errorf(codes.NotFound, "todo with id %d not found", req.GetId())
	}
	delete(s.todos, req.GetId())

	return &pb.DeleteTodoResponse{Deleted: true}, nil
}

func (s *todoServer) CompleteTodo(ctx context.Context, req *pb.CompleteTodoRequest) (*pb.CompleteTodoResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.todos[req.GetId()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "todo with id %d not found", req.GetId())
	}
	item.Completed = true

	return &pb.CompleteTodoResponse{Id: item.Id, Completed: item.Completed}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterTodoServiceServer(s, newTodoServer())
	reflection.Register(s)

	log.Println("gRPC Todo server listening on port 50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
