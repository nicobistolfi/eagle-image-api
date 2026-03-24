FROM golang:1.22 AS builder

RUN apt-get update && apt-get install -y libvips-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -ldflags="-s -w" -o bootstrap main.go

FROM public.ecr.aws/lambda/provided:al2023

RUN dnf install -y libvips && dnf clean all

COPY --from=builder /app/bootstrap ${LAMBDA_RUNTIME_DIR}/bootstrap

CMD ["bootstrap"]
