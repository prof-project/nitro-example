FROM golang:1.22-alpine

WORKDIR /app

COPY grpc-nitro-enclave/go.mod grpc-nitro-enclave/go.sum ./
RUN go mod download

COPY grpc-nitro-enclave/. .

RUN go build -o /enclave-server server.go

CMD ["/enclave-server"]
