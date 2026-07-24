#!/bin/bash
# Borra el servicio de Cloud Run por completo. Garantiza $0 de costo
# mientras no lo uses (aunque, con la config actual, Cloud Run ya
# escala a cero solo y no cobra en reposo).
set -e

PROJECT_ID="grpc-todo-demo-24258"
SERVICE="grpc-todo"
REGION="us-central1"

echo "Borrando el servicio '$SERVICE' en el proyecto '$PROJECT_ID'..."
gcloud run services delete "$SERVICE" \
  --project="$PROJECT_ID" \
  --region="$REGION" \
  --quiet

echo "Listo. El servicio ya no existe."
