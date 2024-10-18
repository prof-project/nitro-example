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

    log.Printf("attestationDoc %v\n", attestationDoc)

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

    log.Printf("zipFilePath %v\n", zipFilePath)

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

    // Log the contents of the zip file for debugging
    log.Println("Contents of the zip file:")
    for _, file := range zipReader.File {
        log.Println(" -", file.Name)
    }

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

    log.Printf("Raw Attestation Document (hex): %x", attestationDoc)
    
    // 1. Decode the attestation document as a COSE_Sign1 message.
    var msg cose.Sign1Message
    err := msg.UnmarshalCBOR(attestationDoc)
    if err != nil {
        return fmt.Errorf("failed to unmarshal COSE_Sign1 message: %v", err)
    }

    // 2. Extract the payload (attestation document)
    payload := msg.Payload

    // 3. Extract the protected header and verify the algorithm
    alg, _ := msg.Headers.Protected.Algorithm()
    if alg != cose.AlgorithmES384 {
        return fmt.Errorf("unexpected algorithm: %v", alg)
    }

    // 4. Extract the certificate and CA bundle from the payload
    var attestationData map[string]interface{}
    if err := cbor.Unmarshal(payload, &attestationData); err != nil {
        return fmt.Errorf("failed to unmarshal attestation document payload: %v", err)
    }

    // Extract certificate
    certBytes, ok := attestationData["certificate"].([]byte)
    if !ok || len(certBytes) == 0 {
        return errors.New("certificate not found in attestation document")
    }

    // Extract CA bundle
    caBundleInterface, ok := attestationData["cabundle"].([]interface{})
    if !ok || len(caBundleInterface) == 0 {
        return errors.New("cabundle not found or invalid in attestation document")
    }

    // Build the certificate chain
    certChain := make([]*x509.Certificate, 0)

    // Parse the leaf certificate
    cert, err := x509.ParseCertificate(certBytes)
    if err != nil {
        return fmt.Errorf("failed to parse certificate: %v", err)
    }
    certChain = append(certChain, cert)

    // Parse intermediate certificates
    for _, caCertRaw := range caBundleInterface {
        caCertBytes, ok := caCertRaw.([]byte)
        if !ok {
            return errors.New("invalid certificate in cabundle")
        }
        caCert, err := x509.ParseCertificate(caCertBytes)
        if err != nil {
            return fmt.Errorf("failed to parse CA certificate: %v", err)
        }
        certChain = append(certChain, caCert)
    }

    // Decode the AWS Nitro Enclaves root certificate
    rootCertBlock, _ := pem.Decode(rootCertPEM)
    if rootCertBlock == nil || rootCertBlock.Type != "CERTIFICATE" {
        return errors.New("failed to decode root certificate PEM")
    }
    rootCert, err := x509.ParseCertificate(rootCertBlock.Bytes)
    if err != nil {
        return fmt.Errorf("failed to parse root certificate: %v", err)
    }

    // 5. Verify the certificate chain
    intermediates := x509.NewCertPool()
    for i := 1; i < len(certChain); i++ {
        intermediates.AddCert(certChain[i])
    }

    roots := x509.NewCertPool()
    roots.AddCert(rootCert)

    opts := x509.VerifyOptions{
        Intermediates: intermediates,
        Roots:         roots,
    }

    if _, err := certChain[0].Verify(opts); err != nil {
        return fmt.Errorf("failed to verify certificate chain: %v", err)
    }

    // 6. Create a Verifier using the public key from the certificate
    publicKey := certChain[0].PublicKey
    verifier, err := cose.NewVerifier(alg, publicKey)
    if err != nil {
        return fmt.Errorf("failed to create verifier: %v", err)
    }

    // 7. Verify the signature using the verifier
    err = msg.Verify(nil, verifier)
    if err != nil {
        return fmt.Errorf("signature verification failed: %v", err)
    }

    // 8. Validate the contents of the attestation document according to the specified rules

    // Check for required fields
    requiredFields := []string{"module_id", "digest", "timestamp", "pcrs", "certificate", "cabundle"}
    for _, field := range requiredFields {
        if value, exists := attestationData[field]; !exists || value == nil {
            return fmt.Errorf("required field %q is missing or null", field)
        }
    }

    // Additional syntactical validations
    if moduleID, ok := attestationData["module_id"].(string); !ok || len(moduleID) == 0 {
        return errors.New("module_id must be a non-empty string")
    }

    if digest, ok := attestationData["digest"].(string); !ok || digest != "SHA384" {
        return errors.New("digest must be 'SHA384'")
    }

    if timestamp, ok := attestationData["timestamp"].(uint64); !ok || timestamp == 0 {
        return errors.New("timestamp must be a positive integer")
    }

    // Validate PCRs
    pcrs, ok := attestationData["pcrs"].(map[interface{}]interface{})
    if !ok || len(pcrs) == 0 || len(pcrs) > 32 {
        return errors.New("pcrs must be a map with 1 to 32 entries")
    }

    for key, value := range pcrs {
        // Key must be an integer in [0,31]
        index, ok := key.(uint64)
        if !ok || index > 31 {
            return errors.New("pcr index must be an integer in [0,31]")
        }
        // Value must be a byte string of length 32, 48, or 64
        hashValue, ok := value.([]byte)
        if !ok || (len(hashValue) != 32 && len(hashValue) != 48 && len(hashValue) != 64) {
            return errors.New("pcr value must be a byte string of length 32, 48, or 64")
        }
    }

    // Additional semantic validations can be added here

    return nil
}
