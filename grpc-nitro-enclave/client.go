package main

import (
    "context"
    "log"
    "os"
    "time"

    "google.golang.org/grpc"
    pb "github.com/prof-project/nitro-example/grpc-nitro-enclave/proto"
)

const (
    address        = "localhost:50051"
    defaultMessage = "Hello from client!"
)

func main() {
    // Set up a connection to the server.
    conn, err := grpc.Dial(address, grpc.WithInsecure())
    if err != nil {
        log.Fatalf("did not connect: %v", err)
    }
    defer conn.Close()
    c := pb.NewEchoServiceClient(conn)

    // Prepare the message.
    message := defaultMessage
    if len(os.Args) > 1 {
        message = os.Args[1]
    }

    // Record the start time.
    startTime := time.Now()

    // Create a context with timeout.
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // Make the gRPC call.
    r, err := c.Echo(ctx, &pb.EchoRequest{Message: message})
    if err != nil {
        log.Fatalf("could not echo: %v", err)
    }

    // Record the end time.
    endTime := time.Now()

    // Calculate the elapsed time.
    elapsed := endTime.Sub(startTime)

    // Log the response and the elapsed time.
    log.Printf("Server response: %s", r.GetMessage())
    log.Printf("Round-trip time: %v", elapsed)
}
