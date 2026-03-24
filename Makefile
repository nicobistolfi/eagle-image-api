.PHONY: build clean test dev

build:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -ldflags="-s -w" -o bootstrap main.go

clean:
	rm -f bootstrap

test:
	go test ./...

dev:
	go run main.go

deploy-dev: build
	serverless deploy --stage dev

deploy-prod: build
	serverless deploy --stage prod
