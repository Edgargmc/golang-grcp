#!/bin/bash
# Deploya (o redeploya) el servidor a Cloud Run desde el Dockerfile local.
set -e

PROJECT_ID="grpc-todo-demo-24258"
SERVICE="grpc-todo"
REGION="us-central1"

# Corre siempre desde la raíz del repo, sin importar desde dónde se
# invoque el script.
cd "$(dirname "$0")/.."

echo "Deployando '$SERVICE' al proyecto '$PROJECT_ID'..."
gcloud run deploy "$SERVICE" \
  --project="$PROJECT_ID" \
  --source=. \
  --region="$REGION" \
  --allow-unauthenticated \
  --port=50051 \
  --use-http2 \
  --quiet

RAW_URL=$(gcloud run services describe "$SERVICE" \
  --project="$PROJECT_ID" \
  --region="$REGION" \
  --format="value(status.url)")

# Postman/grpcurl esperan host:puerto, sin el esquema https:// adelante
# (Postman le agrega grpc:// solo). Ojo: esta URL puede cambiar cada vez
# que se borra y se vuelve a crear el servicio con cloud-down.sh.
HOST_PORT="${RAW_URL#https://}:443"

echo ""
echo "================================================================"
echo " URL para pegar en Postman / grpcurl (sin https://, TLS ON):"
echo ""
echo "   $HOST_PORT"
echo ""
echo "================================================================"
