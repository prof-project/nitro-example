package main

import (
    "context"
    "log"
    "fmt"
    "encoding/base64"

    "github.com/mdlayher/vsock"
    "google.golang.org/grpc"
    pb "github.com/prof-project/nitro-example/grpc-nitro-enclave/proto"

    "github.com/hf/nsm"
    "github.com/hf/nsm/request"
)

const (
    port = 50051 // vsock port number for the gRPC server
)

type server struct {
    pb.UnimplementedEchoServiceServer
    attestationDocument []byte
}

func (s *server) Echo(ctx context.Context, in *pb.EchoRequest) (*pb.EchoResponse, error) {
    log.Printf("Received: %v", in.GetMessage())
    // Include the attestation document in the response
    return &pb.EchoResponse{
        Message:             "Echo: " + in.GetMessage(),
        AttestationDocument: s.attestationDocument,
    }, nil
}

func main() {
    // Obtain the attestation document
    attestationDoc, err := attest(nil, nil, nil)
    if err != nil {
        log.Fatalf("Failed to obtain attestation document: %v", err)
    }

    log.Printf("Attestation Document (base64): %v\n", base64.StdEncoding.EncodeToString(attestationDoc))

    // Create a vsock listener
    listener, err := vsock.Listen(uint32(port), &vsock.Config{})
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    s := grpc.NewServer()
    // Pass the attestation document to the server implementation
    pb.RegisterEchoServiceServer(s, &server{attestationDocument: attestationDoc})
    log.Printf("Server listening on vsock port %d", port)
    if err := s.Serve(listener); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}

// Uses AWS NSM to obtain an attestation document
func attest(nonce, userData, publicKey []byte) ([]byte, error) {
    sess, err := nsm.OpenDefaultSession()
    if err != nil {
        return nil, err
    }
    defer sess.Close()

    res, err := sess.Send(&request.Attestation{
        Nonce:     nonce,
        UserData:  userData,
        PublicKey: publicKey,
    })
    if err != nil {
        return nil, err
    }

    if res.Error != "" {
        return nil, fmt.Errorf("NSM error: %s", res.Error)
    }

    if res.Attestation == nil || res.Attestation.Document == nil {
        return nil, fmt.Errorf("NSM device did not return an attestation")
    }

    return res.Attestation.Document, nil
}
