.PHONY: all build test lint docker-build clean

all: build

build:
	go build -o audiobookshelf .

test:
	go test -v ./...

lint:
	go fmt ./...
	go vet ./...

docker-build:
	docker build -t audiobookshelf-go .

clean:
	rm -f audiobookshelf
