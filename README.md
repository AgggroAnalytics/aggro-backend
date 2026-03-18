# aggro-backend

Go service: HTTP API (fields, seasons, orgs, analytics), Postgres, RabbitMQ (legacy tile/geo/ML paths).

## Temporal worker (field processing → DB)

The **aggro-field-worker** workflow ends by calling activity `finalize_field_processing_db` on queue **`aggro-backend`**. That activity runs only in this repo:

```bash
export DATABASE_URL=postgresql://...
export TEMPORAL_ADDRESS=localhost:7233
export TEMPORAL_NAMESPACE=default
export TEMPORAL_BACKEND_TASK_QUEUE=aggro-backend   # default

go run ./cmd/temporal-worker
```

Deploy this worker wherever the API DB is reachable (same DB as `cmd/server`).

## Temporal (HTTP)

If **`TEMPORAL_ADDRESS`** is set, **`GET /fields/{id}/workflows`** lists `FieldProcessingWorkflow` runs for that field (`workflow_id = field-{field_id}`) with status and a coarse **stage** (from pending activities when running).

## OpenAPI

- **`openapi/openapi.yaml`** — source of truth in repo (embedded in binary).
- **HTTP:** `GET /openapi.yaml` — same file, **без JWT** (удобно для codegen в CI: `npx @openapitools/openapi-generator-cli generate -i https://api.example.com/openapi.yaml …`).
