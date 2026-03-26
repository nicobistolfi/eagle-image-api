.PHONY: build clean test dev docker-build

build:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -ldflags="-s -w" -o bootstrap main.go

clean:
	rm -f bootstrap

test:
	go test ./...

dev:
	go run main.go

docker-build:
	docker build -t image-api:latest .
