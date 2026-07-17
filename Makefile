.PHONY: all build test vet fmt fmt-check docker-build docker-push clean

all: build

build:
	go run run.go run_commands.go build

test:
	go run run.go run_commands.go test

vet:
	go run run.go run_commands.go vet

fmt:
	go run run.go run_commands.go fmt

fmt-check:
	go run run.go run_commands.go fmt-check

docker-build:
	go run run.go run_commands.go docker-build

docker-push:
	go run run.go run_commands.go docker-push

clean:
	go run run.go run_commands.go clean
