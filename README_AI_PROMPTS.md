# LLM Prompt Log – GPU Telemetry Pipeline

This file lists the prompts I used (or planned to use) with an LLM to build the **Elastic GPU Telemetry Pipeline with Message Queue**.

Each prompt represents one step. I ran them in order and then integrated / adjusted the generated output in the codebase.

---

## Prompt 1 – Understand Requirements and Data

You will help me build an end-to-end GPU Telemetry Pipeline.

I am providing two files:

1. `GPU Telemetry Pipeline Message Queue.pdf` – full assignment problem statement.  
2. `dcgm_metrics_20250718_134233 (1)(1).csv` – sample GPU telemetry metrics.

In this step:

- Read both files.
- Extract all **functional requirements** (streamer, collector, queue, APIs, etc.).
- Extract all **non-functional requirements** (scalability up to ~10 streamers/collectors, extensibility, observability/logging, OpenAPI generation via Makefile, Helm deployment, kind for local testing, testing, etc.).
- Summarize the expected end-to-end system in your own words.
- From the CSV, list all column names and what they represent.
- Identify which column(s) can act as a **unique key** or **composite key** for a GPU and for a single telemetry record.  
- Note: ignore the timestamp column in the CSV for persistence; we will use the **real time of ingestion** as the timestamp later.

Do **not** generate code yet. Only output:

- Requirements summary  
- Main components  
- Key entities / domain objects  
- Unique/composite key analysis  
- Any assumptions you needed to make  

---

## Prompt 2 – Architecture, Design Patterns, and SOLID

Based on the requirements and your summary:

- Propose a **high-level architecture** for the system in **Go**.
- List the main components:
  - Telemetry Streamer(s)
  - Telemetry Collector(s)
  - Custom Message Queue (no Kafka/RabbitMQ/etc.)
  - Storage abstraction (in-memory now, extensible to Mongo later)
  - API Gateway / HTTP service
- Draw simple **text / ASCII diagrams**:
  - Component diagram
  - Sequence diagram from CSV → Streamer → Queue → Collector → Storage → API client
- Suggest a clean **package / module layout**, e.g.:
  - `cmd/streamer`, `cmd/collector`, `cmd/api-gateway`
  - `internal/domain`, `internal/mq`, `internal/telemetry`, `internal/storage`, etc.
- Explain where and how to apply:
  - SOLID principles
  - Common design patterns (Repository, Factory, Strategy, Publisher–Subscriber / Observer, Adapter, etc.).
- Explain how the design supports:
  - Multiple Streamer and Collector instances
  - Scaling up to ~10 instances
  - Swapping in a different storage backend (e.g., MongoDB) later
  - Adding observability/logging/metrics.

Do **not** generate code yet. Only architecture and design.

---

## Prompt 3 – Project Skeleton in Go

Now create the initial project structure in Go, without full implementations.

- Use Go modules.
- Create a monorepo layout with (for example):

  - `cmd/streamer/main.go`  
  - `cmd/collector/main.go`  
  - `cmd/api-gateway/main.go`  
  - `internal/domain/...`  
  - `internal/mq/...` (custom message queue)  
  - `internal/telemetry/...`  
  - `internal/storage/...`  
  - `pkg/...` for reusable helpers (if needed).

- Each `main.go` should just log a simple startup message and wire nothing complex yet.
- Add a top-level `go.mod`.
- Add a simple `Makefile` with targets (they can be placeholders for now):

  - `make build`  
  - `make test`  
  - `make run-streamer`  
  - `make run-collector`  
  - `make run-api`

Generate the folder structure and minimal files so I can drop them into a new repo.

---

## Prompt 4 – Domain Models and CSV Mapping

Using the CSV and requirement details:

- Design Go structs for the main domain models, for example:
  - `GPU`
  - `TelemetryPoint`
  - Anything needed to represent metric type, units, etc.
- Map **each CSV column** to fields in these structs.
- Use the previously identified unique/composite keys for:
  - GPU identity (for `/api/v1/gpus/{id}`)
  - Individual telemetry records
- Remember: for the stored telemetry, we will **ignore** the CSV timestamp and use `time.Now()` when we ingest.

Output:

- The Go structs.
- Brief comments explaining field → CSV column mapping.
- How we will query using these keys in the APIs.

---

## Prompt 5 – Custom In-Memory Message Queue

Implement the custom message queue layer.

Requirements:

- Implement a `Message` struct to wrap telemetry events.
- Define a `MessageQueue` interface with methods like:
  - `Publish(ctx, msg)`
  - `Subscribe(...)` (or a way for consumers to receive messages)
- Provide an `InMemoryMessageQueue` implementation:
  - Internally use Go channels.
  - Support **multiple producers (Streamers)** and **multiple consumers (Collectors)**.
  - Be safe for concurrent use.
- Design it so that a different queue implementation could be plugged in later (e.g., via dependency injection).
- Follow SOLID and good separation of concerns.
- Add unit tests to show:
  - Multiple producers and consumers working together.
  - Graceful shutdown behavior.

Generate the Go code and the tests.

---

## Prompt 6 – Storage Layer with Extensible Repository Pattern

Implement the storage abstraction.

- Define repository interfaces such as:
  - `TelemetryRepository`
  - `GPURepository`
- For now, implement **in-memory** versions using Go maps:
  - `InMemoryTelemetryRepository`
  - `InMemoryGPURepository`
- Use mutexes or similar to make them safe for concurrent read/write.
- The repositories must support:

  - Listing all GPUs with telemetry (`ListGPUs`)
  - Storing telemetry points
  - Querying telemetry by GPU id, **ordered by timestamp**
  - Optional `start_time` and `end_time` filters

- Add comments showing how a MongoDB-backed implementation could be added later and plugged in using interfaces / dependency injection.
- Add unit tests that verify the repository behavior.

Generate the Go code and tests.

---

## Prompt 7 – Telemetry Streamer Implementation

Implement the **Telemetry Streamer**.

- It should:
  - Read from `dcgm_metrics_20250718_134233 (1)(1).csv` in a loop to simulate a continuous telemetry stream.
  - For each row:
    - Parse into the domain structs (`TelemetryPoint`, `GPU`, etc.).
    - Assign the timestamp as `time.Now()` when sending (ignore CSV timestamp).
    - Publish the telemetry message to the `MessageQueue`.
- Add configuration via environment variables or flags:

  - CSV file path
  - Stream interval / rate
  - Instance ID (so logs can show which Streamer instance is sending data)

- Handle graceful shutdown on SIGINT/SIGTERM.
- Show how to structure the code so multiple Streamer instances can run in parallel.

Add:

- Implementation under `cmd/streamer` and internal packages.
- Unit tests for CSV parsing and the send loop (testable pieces).

---

## Prompt 8 – Telemetry Collector Implementation

Implement the **Telemetry Collector**.

- It should:
  - Subscribe to the `MessageQueue`.
  - Receive telemetry messages.
  - Convert them back into domain structs if needed.
  - Store data using the repository interfaces (`TelemetryRepository`, `GPURepository`).
- Design so that multiple Collector instances can run in parallel, each consuming from the queue.
- Add configuration via env/flags:

  - Instance ID
  - Any batch size or polling options if needed

- Log errors and handle malformed data robustly.
- Handle graceful shutdown.

Add:

- Implementation under `cmd/collector` and internal packages.
- Unit tests for its message-handling logic.

---

## Prompt 9 – API Contract and Handler Design

Now define the API contract and how handlers will work.

From the assignment:

- `GET /api/v1/gpus`
  - Returns all GPUs for which telemetry exists.
- `GET /api/v1/gpus/{id}/telemetry`
  - Returns telemetry entries for a specific GPU, ordered by time.
  - Supports optional `start_time` and `end_time` query parameters.

In this step:

- Choose a Go HTTP framework (e.g. `chi` or `gin`) and briefly justify.
- Define:

  - Routes
  - Path and query parameters
  - Request/response models (DTOs)
  - Error response format

- Make sure the API uses the previously defined unique/composite key for GPU id.
- Design handler interfaces that:

  - Call the repositories
  - Do not expose internal domain structs directly

Do **not** fully implement yet. Focus on contract and handler design.

---

## Prompt 10 – API Implementation + OpenAPI (Swagger) Generation

Now implement the API Gateway.

- Implement the handlers and wire them to the repositories.
- Add validation for:

  - GPU id path parameter
  - Time query parameters (`start_time`, `end_time`)

- Add basic error handling middleware.

Then:

- Add Swagger/OpenAPI annotations (e.g. using `swaggo/swag` or a similar tool).
- Configure a Makefile target such as:

  - `make openapi` or `make swagger`

  to generate the OpenAPI spec (e.g., `api/openapi.yaml` or `docs/swagger`).

- Provide a short README snippet that explains:

  - How to generate the OpenAPI spec.
  - How to view it (e.g., using Swagger UI or a simple viewer).

Generate:

- API Gateway code under `cmd/api-gateway` and internal packages.
- OpenAPI annotations.
- Makefile changes.

---

## Prompt 11 – Unit Tests and Coverage

Add testing and coverage support.

- Write unit tests for:

  - Message queue implementation
  - Repository implementations
  - Streamer CSV parsing logic
  - API handlers (use mocked repositories)

- Update the Makefile:

  - `make test` should run all tests (preferably with `-race`) and print basic coverage.
  - `make cover` should generate a coverage profile and an HTML report (e.g., `coverage.out` + `coverage.html`).

Output:

- All test files.
- Updated Makefile with commands to generate and view coverage.

---

## Prompt 12 – Dockerfiles for All Services

Create Dockerfiles for:

- Telemetry Streamer
- Telemetry Collector
- API Gateway

Requirements:

- Use multi-stage builds to keep images small.
- Use environment variables for configuration (CSV path, instance IDs, etc.).
- Update the Makefile with targets like:

  - `make docker-streamer`
  - `make docker-collector`
  - `make docker-api`

Output:

- Dockerfiles.
- Makefile changes.

---

## Prompt 13 – Helm Charts and kind Deployment

I want to run everything on **Kubernetes** locally using **kind** and Helm.

In this step:

- Create a Helm setup, for example:

  - An umbrella chart: `charts/gpu-telemetry`
  - Templates (or subcharts) for:
    - Streamer
    - Collector
    - API Gateway
    - Custom queue
    - (Optionally) a separate MQ service if needed later

- In `values.yaml`, include:

  - Replica counts for Streamer and Collector so they can scale.
  - Environment variable values (CSV path, etc.).
  - Service configuration for the API Gateway.

- Ensure it works with a local kind cluster:

  - Show how to build Docker images.
  - Show how to load them into kind: `kind load docker-image <image>:<tag>`.
  - Configure Service type or instructions for `kubectl port-forward` so I can reach the API Gateway.

Generate:

- `Chart.yaml`
- `values.yaml`
- Templates (Deployments, Services, etc.)
- Example `helm install`/`helm upgrade` commands.

---

## Prompt 14 – End-to-End Flow and kind Test Instructions

Provide a simple manual end-to-end test plan using **kind**.

Describe steps:

1. Create a kind cluster.
2. Build Docker images and load them into kind.
3. Install the Helm chart.
4. Verify that Streamer(s) and Collector(s) are running.
5. Use `kubectl port-forward` (or NodePort) to call:
   - `GET /api/v1/gpus`
   - `GET /api/v1/gpus/{id}/telemetry?start_time=...&end_time=...`
6. Check that:
   - Data is ingested from the CSV.
   - Messages flow through the queue.
   - Telemetry is stored and returned correctly.

Optionally:

- Suggest a small Go system test that:
  - Waits for data to appear.
  - Calls the API.
  - Asserts on the response.

Output:

- Markdown instructions I can paste into my main `README.md`.

---

## Prompt 15 – Final Review, AI Usage Section, and Interview Summary

Finally:

**Code and Design Review**
   - Review the overall codebase for:
     - SOLID compliance
     - Clear separation of concerns
     - Proper use of patterns (Repository, Factory, Strategy, etc.).
   - Suggest small refactors or cleanup where needed (naming, error handling, logging, package layout).

Output:

- Review notes / suggested improvements.

---

✅ **End of Prompt Log**

This file can be used to reproducibly guide an LLM through building the entire GPU Telemetry Pipeline project step by step, while meeting all assignment requirements.
