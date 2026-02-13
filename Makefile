.PHONY: build test test-integration test-integration-docker clean

build:
	go build -o bin/isolarium ./cmd/isolarium

test:
	go test ./...

test-integration:
	go test -tags=integration ./internal/lima/...

test-integration-docker:
	go test -tags=integration ./internal/docker/...

clean:
	rm -rf bin/
