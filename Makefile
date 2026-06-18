.PHONY: build build-linux build-linux-amd64 build-linux-arm64 test

build:
	go build -o paqetpremium ./cmd/paqetpremium

# Build on Linux VPS (requires libpcap-dev and CGO).
build-linux: build-linux-amd64

build-linux-amd64:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o paqetpremium-linux-amd64 ./cmd/paqetpremium

build-linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -o paqetpremium-linux-arm64 ./cmd/paqetpremium

test:
	go test ./...
	go run ./cmd/paqetpremium test -c example/client.yaml
