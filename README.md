# gRPC Todo Service

This is a simple gRPC-based Todo service to help learn Go and gRPC concepts.

## Features

- Create, read, update, and delete todo items
- Simple in-memory storage (for demonstration purposes)
- gRPC service with protobuf definitions

## Project Structure

```
grpc-todo/
├── proto/           # Protocol buffer definitions
├── server/          # gRPC server implementation
├── client/          # gRPC client implementation
├── gen/             # Generated gRPC code (will be created during build)
├── go.mod           # Go module file
├── Makefile         # Build and run commands
├── build.sh         # Build script for generating gRPC code
└── README.md        # This file
```

## Learning Goals

This project will help you understand:

1. **Go basics**: Packages, structs, methods, concurrency
2. **gRPC concepts**: Services, messages, RPC calls
3. **Protocol Buffers**: Defining service contracts
4. **Server/Client architecture**: How to build distributed systems
5. **Error handling in gRPC**

## Getting Started

### Prerequisites

1. Install Go (1.20 or higher)
2. Install protoc compiler (Protocol Buffer compiler)

### Quick Setup

1. Clone this project:
   ```
   cd grpc-todo
   ```

2. Install dependencies:
   ```
   go mod tidy
   ```

3. For the full gRPC experience with code generation:
   ```
   ./build.sh
   ```

4. Run the server:
   ```
   make run-server
   ```

5. In another terminal, run the client:
   ```
   make run-client
   ```

## Project Components

### Server
The server implements a Todo service that handles CRUD operations for todo items.

### Client  
The client demonstrates how to connect to and call the gRPC service.

### Proto Definitions
The `proto/todo.proto` file defines the service contract using Protocol Buffers.

## Next Steps

Once you understand the basics, consider extending this project with:

- Database integration (SQLite, PostgreSQL)
- Authentication and authorization
- gRPC middleware
- Streaming RPCs
- Load balancing