// Binario chico y autocontenido para el HEALTHCHECK del Dockerfile.
// No usa nada de la lógica del servidor: solo abre una conexión gRPC
// y pregunta al servicio estándar grpc.health.v1.Health si está SERVING.
// Sale con código 0 si está sano, distinto de 0 si no — la semántica
// que Docker espera de HEALTHCHECK.
package main

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("healthcheck: failed to dial: %v", err)
	}
	defer conn.Close()

	client := grpc_health_v1.NewHealthClient(conn)
	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		log.Fatalf("healthcheck: check failed: %v", err)
	}
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		log.Fatalf("healthcheck: status is %v", resp.Status)
	}
}
