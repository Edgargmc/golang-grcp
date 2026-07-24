# ---- Build stage ----
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY gen/ gen/
COPY server/ server/

# CGO_ENABLED=0: modernc.org/sqlite es Go puro, no necesita cgo ni gcc,
# así que el binario queda estático y la imagen final puede ser mínima.
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/healthcheck ./server/healthcheck

# ---- Runtime stage ----
FROM alpine:3.20

RUN adduser -D -u 10001 appuser
WORKDIR /app
RUN chown appuser:appuser /app

RUN mkdir -p /data && chown appuser:appuser /data
ENV DB_PATH=/data/todos.db

COPY --from=builder --chown=appuser:appuser /out/server .
COPY --from=builder --chown=appuser:appuser /out/healthcheck .

USER appuser

VOLUME ["/data"]

EXPOSE 50051

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app/healthcheck"]

ENTRYPOINT ["./server"]
