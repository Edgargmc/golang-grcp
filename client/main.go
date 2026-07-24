package main

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "grpc-todo/gen"
)

// Debe coincidir con el authToken hardcodeado en server/main.go.
const authToken = "secret-token-123"

func authInterceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", authToken)
	return invoker(ctx, method, req, reply, cc, opts...)
}

func main() {
	conn, err := grpc.NewClient("localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(authInterceptor),
	)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := pb.NewTodoServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.CreateTodo(ctx, &pb.CreateTodoRequest{Title: "   "}); err != nil {
		st, _ := status.FromError(err)
		if st.Code() == codes.InvalidArgument {
			log.Printf("Got expected validation error: %s", st.Message())
		} else {
			log.Printf("Got unexpected error code=%s message=%s", st.Code(), st.Message())
		}
	} else {
		log.Println("Expected an InvalidArgument error for empty title, but the call succeeded")
	}

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
