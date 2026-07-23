package main

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "grpc-todo/gen"
)

func main() {
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := pb.NewTodoServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	created, err := client.CreateTodo(ctx, &pb.CreateTodoRequest{
		Title:       "Learn gRPC",
		Description: "Build a todo service with Go and gRPC",
	})
	if err != nil {
		log.Fatalf("CreateTodo failed: %v", err)
	}
	log.Printf("Created todo: %+v", created)

	got, err := client.GetTodo(ctx, &pb.GetTodoRequest{Id: created.Id})
	if err != nil {
		log.Fatalf("GetTodo failed: %v", err)
	}
	log.Printf("Fetched todo: %+v", got)

	updated, err := client.UpdateTodo(ctx, &pb.UpdateTodoRequest{
		Id:          created.Id,
		Title:       got.Title,
		Description: got.Description,
		Completed:   true,
	})
	if err != nil {
		log.Fatalf("UpdateTodo failed: %v", err)
	}
	log.Printf("Updated todo: %+v", updated)

	list, err := client.ListTodos(ctx, &pb.ListTodosRequest{Page: 1, Limit: 10})
	if err != nil {
		log.Fatalf("ListTodos failed: %v", err)
	}
	log.Printf("Todo list (total=%d): %+v", list.Total, list.Todos)

	deleted, err := client.DeleteTodo(ctx, &pb.DeleteTodoRequest{Id: created.Id})
	if err != nil {
		log.Fatalf("DeleteTodo failed: %v", err)
	}
	log.Printf("Deleted: %v", deleted.Deleted)
}
