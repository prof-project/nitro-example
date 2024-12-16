package main

import (
    "archive/zip"
    "context"
    "crypto/sha256"
    "crypto/x509"
    "encoding/pem"
    "errors"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "time"

    "github.com/fxamacker/cbor/v2"
    "github.com/veraison/go-cose"
    "google.golang.org/grpc"
    pb "github.com/prof-project/nitro-example/grpc-nitro-enclave/proto"
)

const (
    address        = "localhost:50051"
    defaultMessage = "Hello from client!"
)

// AttestationDocument represents the structure of the attestation payload.
type AttestationDocument struct {
	ModuleID    string         `cbor:"module_id"`
	Timestamp   uint64         `cbor:"timestamp"`
	Digest      string         `cbor:"digest"`
	PCRs        map[int][]byte `cbor:"pcrs"`
	Certificate []byte         `cbor:"certificate"`
	CABundle    [][]byte       `cbor:"cabundle"`
	PublicKey   []byte         `cbor:"public_key,omitempty"`
	UserData    []byte         `cbor:"user_data,omitempty"`
	Nonce       []byte         `cbor:"nonce,omitempty"`
}

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

    // Verify the attestation document received from the server
    attestationDoc := r.GetAttestationDocument()
    if len(attestationDoc) == 0 {
        log.Fatalf("No attestation document received from server")
    }

    // Download and verify the AWS Nitro Enclaves root certificate
    rootCertPEM, err := downloadAndVerifyRootCert(
        "https://aws-nitro-enclaves.amazonaws.com/AWS_NitroEnclaves_Root-G1.zip",
        "8cf60e2b2efca96c6a9e71e851d00c1b6991cc09eadbe64a6a1d1b1eb9faff7c",
    )
    if err != nil {
        log.Fatalf("Failed to obtain root certificate: %v", err)
    }

    // Call the verification function
    err = verifyAttestationDocument(attestationDoc, rootCertPEM)
    if err != nil {
        log.Fatalf("Attestation document verification failed: %v", err)
    }

    log.Println("Attestation document verified successfully")

    // Log the response and the elapsed time.
    log.Printf("Server response: %s", r.GetMessage())
    log.Printf("Round-trip time: %v", elapsed)
}

// Function to download and verify the root certificate
func downloadAndVerifyRootCert(url, expectedHash string) ([]byte, error) {
    // Download the zip file
    resp, err := http.Get(url)
    if err != nil {
        return nil, fmt.Errorf("failed to download root certificate: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("failed to download root certificate: HTTP %d", resp.StatusCode)
    }

    // Read the response body
    zipData, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read zip file: %v", err)
    }

    // Compute SHA256 hash
    hash := sha256.Sum256(zipData)
    hashString := fmt.Sprintf("%x", hash)

    if hashString != expectedHash {
        return nil, fmt.Errorf("root certificate hash mismatch: expected %s, got %s", expectedHash, hashString)
    }

    // Get the current working directory
    cwd, err := os.Getwd()
    if err != nil {
        return nil, fmt.Errorf("failed to get current working directory: %v", err)
    }

    // Construct the file path
    zipFilePath := fmt.Sprintf("%s/AWS_NitroEnclaves_Root-G1.zip", cwd)

    // Save zip file temporarily
    err = os.WriteFile(zipFilePath, zipData, 0644)
    if err != nil {
        return nil, fmt.Errorf("failed to save zip file: %v", err)
    }
    defer os.Remove(zipFilePath)

    // Unzip the file and extract the PEM file
    pemData, err := extractPEMFromZip(zipFilePath)
    if err != nil {
        return nil, fmt.Errorf("failed to extract PEM file: %v", err)
    }

    return pemData, nil
}
func extractPEMFromZip(zipFilePath string) ([]byte, error) {
    // Open the zip file
    zipReader, err := zip.OpenReader(zipFilePath)
    if err != nil {
        return nil, fmt.Errorf("failed to open zip file: %v", err)
    }
    defer zipReader.Close()

    // Look for the .pem file
    for _, file := range zipReader.File {
        if file.Name == "root.pem" {
            pemFile, err := file.Open()
            if err != nil {
                return nil, fmt.Errorf("failed to open PEM file inside zip: %v", err)
            }
            defer pemFile.Close()

            pemData, err := io.ReadAll(pemFile)
            if err != nil {
                return nil, fmt.Errorf("failed to read PEM file: %v", err)
            }
            return pemData, nil
        }
    }

    return nil, errors.New("PEM file not found in zip archive")
}
// Function to verify the attestation document
func verifyAttestationDocument(attestationDoc []byte, rootCertPEM []byte) error {
    
    // Parse the COSE message
	attestationMap := cose.UntaggedSign1Message{}
	err := attestationMap.UnmarshalCBOR(attestationDoc)
	if err != nil {
		log.Fatalf("failed to unmarshal COSE message: %v", err)
	}

    // Unmarshal the Payload into AttestationDocument
    if len(attestationMap.Payload) == 0 {
        return errors.New("Payload is empty in the attestation document")
    }

    var attestationDocStruct AttestationDocument
    err = cbor.Unmarshal(attestationMap.Payload, &attestationDocStruct)
    if err != nil {
        return fmt.Errorf("FAILED to unmarshal Payload as AttestationDocument: %v", err)
    }

    // Syntactic Validation
    err = validateAttestationDocumentFields(attestationDocStruct)
    if err != nil {
        return fmt.Errorf("Syntactic validation failed: %v", err)
    }

    // Parse Root Certificate
    rootCert, err := parseCertificate(rootCertPEM)
    if err != nil {
        return fmt.Errorf("Failed to parse root certificate: %v", err)
    }

    // Parse and Validate Certificate Chain
    _, err = buildCertificateChain(attestationDocStruct.Certificate, attestationDocStruct.CABundle, rootCert)
    if err != nil {
        return fmt.Errorf("Certificate chain validation failed: %v", err)
    }

    // Step 7: Verify COSE Signature 
    // Parse the attestation certificate to get the public key
    attestationCert, err := parseCertificate(attestationDocStruct.Certificate)
    if err != nil {
        return fmt.Errorf("Failed to parse attestation certificate: %v", err)
    }

    publicKey := attestationCert.PublicKey

    // Assume the algorithm is always ECDSA with SHA-384
    alg := cose.AlgorithmES384

    // Create a verifier using the algorithm and public key
    verifier, err := cose.NewVerifier(alg, publicKey)
    if err != nil {
        return fmt.Errorf("Failed to create COSE verifier: %v", err)
    }

    // Verify the COSE_Sign1 signature
    err = attestationMap.Verify(nil, verifier)
    if err != nil {
        return fmt.Errorf("COSE signature verification failed: %v", err)
    }

    log.Println("COSE signature verified successfully!")

    return nil
}

// parseCertificate parses a PEM or DER encoded certificate.
func parseCertificate(certBytes []byte) (*x509.Certificate, error) {
    // Attempt to parse as PEM
    block, _ := pem.Decode(certBytes)
    if block != nil && block.Type == "CERTIFICATE" {
        return x509.ParseCertificate(block.Bytes)
    }
    // Attempt to parse as DER
    return x509.ParseCertificate(certBytes)
}

// buildCertificateChain builds and validates the certificate chain.
func buildCertificateChain(targetCertBytes []byte, caBundleBytes [][]byte, rootCert *x509.Certificate) ([]*x509.Certificate, error) {
    // Parse target certificate
    targetCert, err := parseCertificate(targetCertBytes)
    if err != nil {
        return nil, fmt.Errorf("failed to parse target certificate: %v", err)
    }

    // Parse CA bundle certificates
    var intermediateCerts []*x509.Certificate
    for i, caCertBytes := range caBundleBytes {
        cert, err := parseCertificate(caCertBytes)
        if err != nil {
            return nil, fmt.Errorf("failed to parse cabundle[%d]: %v", i, err)
        }
        intermediateCerts = append(intermediateCerts, cert)
    }

    // Create certificate pool for intermediates
    intermediatesPool := x509.NewCertPool()
    for _, cert := range intermediateCerts {
        intermediatesPool.AddCert(cert)
    }

    // Create certificate pool for root
    roots := x509.NewCertPool()
    roots.AddCert(rootCert)

    // Set up verification options
    opts := x509.VerifyOptions{
        Roots:         roots,
        Intermediates: intermediatesPool,
        CurrentTime:   time.Now(),
        KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
    }

    // Verify the certificate chain
    chains, err := targetCert.Verify(opts)
    if err != nil {
        return nil, fmt.Errorf("certificate verification failed: %v", err)
    }

    // For further semantic validation, you might need to traverse the chain
    // Here, we simply return the first valid chain
    if len(chains) == 0 {
        return nil, errors.New("no valid certificate chains found")
    }

    return chains[0], nil
}

// validateAttestationDocumentFields performs syntactic validation of the attestation document.
func validateAttestationDocumentFields(doc AttestationDocument) error {
    // Check mandatory fields are non-empty
    if doc.ModuleID == "" {
        return errors.New("module_id is missing or empty")
    }
    if doc.Digest == "" {
        return errors.New("digest is missing or empty")
    }
    if doc.Timestamp == 0 {
        return errors.New("timestamp is missing or zero")
    }
    if len(doc.PCRs) == 0 {
        return errors.New("pcrs is missing or empty")
    }
    if len(doc.Certificate) == 0 {
        return errors.New("certificate is missing or empty")
    }
    if len(doc.CABundle) == 0 {
        return errors.New("cabundle is missing or empty")
    }

    // Validate 'digest' field
    if doc.Digest != "SHA384" {
        return fmt.Errorf("invalid digest value: %s", doc.Digest)
    }

    // Validate 'pcrs' field
    if len(doc.PCRs) < 1 || len(doc.PCRs) > 32 {
        return fmt.Errorf("pcrs size out of bounds: %d", len(doc.PCRs))
    }
    for idx, pcr := range doc.PCRs {
        if idx < 0 || idx >= 32 {
            return fmt.Errorf("invalid PCR index: %d", idx)
        }
        if len(pcr) != 32 && len(pcr) != 48 && len(pcr) != 64 {
            return fmt.Errorf("invalid PCR length for index %d: %d", idx, len(pcr))
        }
    }

    // Validate 'cabundle' field
    for i, cert := range doc.CABundle {
        if len(cert) < 1 || len(cert) > 1024 {
            return fmt.Errorf("invalid cabundle[%d] length: %d", i, len(cert))
        }
    }

    // Validate optional fields
    if len(doc.PublicKey) > 0 && len(doc.PublicKey) > 1024 {
        return fmt.Errorf("public_key length exceeds limit: %d", len(doc.PublicKey))
    }
    if len(doc.UserData) > 512 {
        return fmt.Errorf("user_data length exceeds limit: %d", len(doc.UserData))
    }
    if len(doc.Nonce) > 512 {
        return fmt.Errorf("nonce length exceeds limit: %d", len(doc.Nonce))
    }

    log.Println("Attestation document fields validated")

    return nil
}

