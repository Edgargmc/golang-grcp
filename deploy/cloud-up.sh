#!/bin/bash
# Deploya (o redeploya) el servidor a Cloud Run desde el Dockerfile local.
set -e

PROJECT_ID="grpc-todo-demo-24258"
SERVICE="grpc-todo"
REGION="us-central1"

# Corre siempre desde la raíz del repo, sin importar desde dónde se
# invoque el script.
cd "$(dirname "$0")/.."

# Credenciales de Turso generadas al vuelo (requiere `turso auth login`
# hecho antes) — nunca quedan hardcodeadas en este archivo.
TURSO_DATABASE_URL=$(turso db show grpc-todo --url)
TURSO_AUTH_TOKEN=$(turso db tokens create grpc-todo)

# A diferencia de Turso, el AUTH_TOKEN de la app NO se genera solo acá:
# es el mismo token real que ya está en GitHub Secrets / local.properties
# de Android. Sin fallback silencioso — si no está seteado, cortamos,
# para no desplegar accidentalmente con el token de desarrollo.
if [ -z "$AUTH_TOKEN" ]; then
  echo "Error: falta la variable de entorno AUTH_TOKEN (el token real de producción)."
  echo "Exportala antes de correr este script: export AUTH_TOKEN=..."
  exit 1
fi

echo "Deployando '$SERVICE' al proyecto '$PROJECT_ID'..."
gcloud run deploy "$SERVICE" \
  --project="$PROJECT_ID" \
  --source=. \
  --region="$REGION" \
  --allow-unauthenticated \
  --port=50051 \
  --use-http2 \
  --set-env-vars="TURSO_DATABASE_URL=${TURSO_DATABASE_URL},TURSO_AUTH_TOKEN=${TURSO_AUTH_TOKEN},AUTH_TOKEN=${AUTH_TOKEN}" \
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
