.PHONY: build test test-integration test-integration-docker test-ec2 clean

build:
	go build -o bin/isolarium ./cmd/isolarium

test:
	go test ./...

test-integration:
	go test -tags=integration ./internal/lima/...

test-integration-docker:
	go test -tags=integration ./internal/docker/...

test-ec2:
	go test -tags=ec2 -timeout 60m ./internal/ec2/...

clean:
	rm -rf bin/
