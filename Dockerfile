FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y libvips-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -ldflags="-s -w" -o bootstrap main.go

# Collect libvips and all its shared library dependencies
RUN mkdir -p /opt/lib && \
    cp /usr/lib/x86_64-linux-gnu/libvips.so* /opt/lib/ && \
    ldd /app/bootstrap | grep "=> /" | awk '{print $3}' | xargs -I{} cp -L {} /opt/lib/ 2>/dev/null || true

FROM public.ecr.aws/lambda/provided:al2023

# Copy shared libraries from builder
COPY --from=builder /opt/lib/ /usr/lib64/
RUN ldconfig

COPY --from=builder /app/bootstrap ${LAMBDA_RUNTIME_DIR}/bootstrap

CMD ["bootstrap"]
