# Task API

A Go-based API for managing tree-structured tasks using PostgreSQL and RabbitMQ.

## Prerequisites

- Go 1.20+
- PostgreSQL
- RabbitMQ

## Configuration

Copy `.env.example` to `.env` (or create one) and configure your connection strings:
```properties
POSTGRES_URL=postgres://user:pass@localhost:5432/dbname?sslmode=disable
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
PORT=8080
```
*(A default `.env` is created for you if you asked the assistant)*

## Database Migrations

The database schema is located in `migrations/schema.sql`.

### Manual Application
To apply the schema manually (recommended for production or dev environments):

```bash
psql "$POSTGRES_URL" -f migrations/schema.sql
```

Or using `psql` CLI params:
```bash
psql -h localhost -U postgres -d task-api -f migrations/schema.sql
```

### Automated Testing
The `make test` command automatically attempts to apply the schema if the `tasks` table does not exist in the configured database.

## Building and Running

### Build
```bash
make build
```
This produces binaries in `bin/`: `bin/api` and `bin/tester`.

### Run API
```bash
make run
```
Starts the API server on the configured port.

### Run Tests
```bash
make test
```
Runs the integration test suite. This requires the API to be running? 
**NOTE**: The `make test` command currently runs the **tester app**, but the tester app communicates with the API. 
**You must have `make run` running in a separate terminal**, OR modify the `Makefile` / `tester` to start the API.
*Wait, looking at `tester/main.go`, it hits `http://localhost:8080`. So yes, you need the API running.*

Steps to test:
1. Terminal 1: `make run`
2. Terminal 2: `make test`

