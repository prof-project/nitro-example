package main

import (
    "context"
    "log"

    "github.com/mdlayher/vsock"
    "google.golang.org/grpc"
    pb "github.com/prof-project/nitro-example/grpc-nitro-enclave/proto"
)

const (
    port = 50051 // vsock port number for the gRPC server
)

type server struct {
    pb.UnimplementedEchoServiceServer
}

func (s *server) Echo(ctx context.Context, in *pb.EchoRequest) (*pb.EchoResponse, error) {
    log.Printf("Received: %v", in.GetMessage())
    return &pb.EchoResponse{Message: "Echo: " + in.GetMessage()}, nil
}

func main() {
    // Create a vsock listener
    listener, err := vsock.Listen(uint32(port), &vsock.Config{})
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    s := grpc.NewServer()
    pb.RegisterEchoServiceServer(s, &server{})
    log.Printf("Server listening on vsock port %d", port)
    if err := s.Serve(listener); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}
