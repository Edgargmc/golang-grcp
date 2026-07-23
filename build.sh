#!/bin/bash

echo "Building gRPC Todo service..."

# Install required dependencies if not already installed
echo "Installing dependencies..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate gRPC code from proto files
echo "Generating gRPC code..."
mkdir -p gen
protoc --go_out=gen --go-grpc_out=gen proto/todo.proto

echo "Build complete!"
echo "You can now run:"
echo "  cd server && go run main.go"
echo "  cd client && go run main.go"