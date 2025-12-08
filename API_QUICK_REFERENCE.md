# API Gateway - Quick Reference

## Start API Gateway

```bash
# Build and run
make run-api

# Or manually
go run cmd/api-gateway/main.go
```

Server starts on: **http://localhost:8080**

---

## View Swagger UI

**URL**: http://localhost:8080/swagger/index.html

Interactive API documentation with try-it-out functionality.

---

## API Endpoints

### List All GPUs
```bash
curl http://localhost:8080/api/v1/gpus | jq .
```

### Get Specific GPU
```bash
curl http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50 | jq .
```

### Get Telemetry (All Time)
```bash
curl http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry | jq .
```

### Get Telemetry (Time Filtered)
```bash
curl "http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?start_time=2025-01-18T00:00:00Z&end_time=2025-01-18T12:00:00Z" | jq .
```

---

## Regenerate OpenAPI Spec

```bash
make swagger
```

Generated files:
- `docs/swagger/swagger.json`
- `docs/swagger/swagger.yaml`
- `docs/swagger/docs.go`

---

## Environment Variables

- `PORT` - API server port (default: 8080)

Example:
```bash
PORT=9000 go run cmd/api-gateway/main.go
```

---

## Complete Documentation

- **API Contract**: `docs/API_CONTRACT_DESIGN.md`
- **User Guide**: `docs/API_DOCUMENTATION.md`
- **Implementation**: `docs/PROMPT_10_COMPLETION.md`
- **OpenAPI Spec**: `docs/swagger/swagger.yaml`
