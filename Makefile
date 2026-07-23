.PHONY: run-server run-client build

# Run the gRPC server
run-server:
	cd server && go run main.go

# Run the gRPC client
run-client:
	cd client && go run main.go

# Build both
build:
	go build -o server/server ./server
	go build -o client/client ./client

# Install dependencies
deps:
	go get -u google.golang.org/grpc
	go get -u google.golang.org/protobuf

# Generate gRPC code (requires protoc)
generate:
	protoc --go_out=. --go-grpc_out=. proto/todo.proto

help:
	@echo "Available targets:"
	@echo "  run-server    - Run the gRPC server"
	@echo "  run-client    - Run the gRPC client"
	@echo "  build         - Build both server and client"
	@echo "  deps          - Install dependencies"
	@echo "  generate      - Generate gRPC code from proto files"
	@echo "  help          - Show this help"