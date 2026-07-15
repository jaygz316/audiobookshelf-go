.PHONY: all build test vet fmt fmt-check docker-build docker-push clean

all: build

build:
	go run run.go build

test:
	go run run.go test

vet:
	go run run.go vet

fmt:
	go run run.go fmt

fmt-check:
	go run run.go fmt-check

docker-build:
	go run run.go docker-build

docker-push:
	go run run.go docker-push

clean:
	go run run.go clean
