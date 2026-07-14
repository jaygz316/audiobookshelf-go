.PHONY: all build test vet fmt fmt-check docker-build clean

all: build

build:
	go build -o audiobookshelf-go .

test:
	go test -v ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

fmt-check:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Go files are not formatted. Please run 'make fmt'."; \
		gofmt -l .; \
		exit 1; \
	fi

docker-build:
	docker build -t jaygz/audiobookshelf-go:latest .

clean:
	rm -f audiobookshelf-go
