FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y libvips-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -ldflags="-s -w" -o bootstrap main.go

FROM debian:bookworm-slim

# Install libvips runtime and CA certificates
RUN apt-get update && \
    apt-get install -y --no-install-recommends libvips42 ca-certificates && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/bootstrap /var/runtime/bootstrap

ENTRYPOINT ["/var/runtime/bootstrap"]
