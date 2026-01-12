FROM golang:1.20-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/bin/api cmd/api/main.go

# Runner Stage
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/bin/api .
COPY migrations ./migrations

EXPOSE 8080

CMD ["./api"]
