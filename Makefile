APP := govoting
BIN := bin/govoting
TEST_DB_DSN ?= osivote:osivote_dev@tcp(127.0.0.1:3307)/go_voting_test?parseTime=true&loc=UTC&time_zone=%27%2B00%3A00%27

.PHONY: run build test test-integration test-all vet fmt tidy docker-up docker-down docker-logs clean

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/server

test:
	go test ./...

test-integration:
	TEST_DB_DSN='$(TEST_DB_DSN)' go test ./tests -v -count=1

test-all: test test-integration

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

tidy:
	go mod tidy

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

clean:
	rm -rf bin coverage.out
