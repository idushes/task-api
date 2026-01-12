.PHONY: build run test clean

build:
	go build -o bin/api cmd/api/main.go
	go build -o bin/tester cmd/tester/main.go

run:
	go run cmd/api/main.go

test:
	@echo "Starting API in background..."
	@go run cmd/api/main.go > api.log 2>&1 & echo $$! > api.pid
	@echo "Waiting for API to be ready..."
	@sleep 3
	@echo "Running tests..."
	@-go run cmd/tester/main.go; \
	result=$$?; \
	kill `cat api.pid` || true; \
	rm api.pid; \
	if [ $$result -ne 0 ]; then \
		echo "API Logs:"; \
		cat api.log; \
	fi; \
	rm api.log; \
	exit $$result

clean:
	rm -rf bin
