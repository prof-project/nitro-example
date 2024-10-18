package main

import (
    "context"
    "log"
    "encoding/base64"
	"errors"

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
}

func (s *server) Echo(ctx context.Context, in *pb.EchoRequest) (*pb.EchoResponse, error) {
    log.Printf("Received: %v", in.GetMessage())
    return &pb.EchoResponse{Message: "Echo: " + in.GetMessage()}, nil
}

func main() {

    // User data can be used as part of a challenge-response authentication mechanism. 
    // For example, when a remote service requests attestation from an enclave, 
    // the service might want to ensure that the attestation request is fresh and hasn’t
    // been reused. By including custom user data (e.g., a unique identifier, request 
    // parameters, etc.), the service can tie the attestation to a specific session 
    // or interaction.

    att, err :=
		attest(
			[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			[]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		)

	log.Printf("attestation %v %v\n", base64.StdEncoding.EncodeToString(att), err)

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

// Uses AWS nsm to obtain an attestation document
func attest(nonce, userData, publicKey []byte) ([]byte, error) {
	sess, err := nsm.OpenDefaultSession()
	defer sess.Close()

	if nil != err {
		return nil, err
	}

	res, err := sess.Send(&request.Attestation{
		Nonce:     nonce,
		UserData:  userData,
		PublicKey: publicKey,
	})
	if nil != err {
		return nil, err
	}

	if "" != res.Error {
		return nil, errors.New(string(res.Error))
	}

	if nil == res.Attestation || nil == res.Attestation.Document {
		return nil, errors.New("NSM device did not return an attestation")
	}

	return res.Attestation.Document, nil
}
