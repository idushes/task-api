# Task API

A Go-based API for managing tree-structured tasks using PostgreSQL and RabbitMQ.

## Prerequisites

- Go 1.20+
- PostgreSQL
- RabbitMQ

## Configuration

The application automatically loads environment variables from a `.env` file in the current directory if it exists.

1.  Copy `.env.example` to `.env` (or create one):
    ```properties
    POSTGRES_URL=postgres://user:pass@localhost:5432/dbname?sslmode=disable
    RABBITMQ_URL=amqp://guest:guest@localhost:5672/
    PORT=8080
    ```

You can also set these variables in your shell environment, which will take precedence (except for `.env` which is loaded if present, but standard env precedence applies).

To change the port, you can update `.env` or pass it when running:
```bash
PORT=9000 make run
```

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
Starts the API server.

### Run Tests
```bash
make test
```
This command:
1.  Starts the API in the background.
2.  Runs the integration test suite (`cmd/tester`).
3.  Cleans up the background API process.
