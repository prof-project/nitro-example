# Nitro Enclaves Samples

This repository hosts:
- A vsock server running inside an enclave, implemented in Rust and Python, which is used for communication between a parent instance and a Nitro Enclave or between two enclaves
- A gRPC server, implemented in Golang, which is running inside the Nitro Enclave. Traffic from clients is proxied through a vsock interface.
    - Inspiration comes from [this article](https://dev.to/bendecoste/running-an-http-server-with-aws-nitro-enclaves-elo) 

**NOTE:** Ensure that Enclave Support is enabled when the EC2 instance is launched! Otherwise, you may get errors [E19], [E39], and '/dev/nitro_enclaves' is not created.

## How to run on AWS

Important Articles:
- [Install nitro-enclaves-cli](https://docs.aws.amazon.com/enclaves/latest/user/nitro-enclave-cli-install.html)
- [Follow the instructions for developing applications on Linux](https://docs.aws.amazon.com/enclaves/latest/user/developing-applications-linux.html)

### Installation:

1. Install rust & rustup:
   ```bash
   curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
   source $HOME/.cargo/env
   ```

2. Install git for cloning the repository:
   ```bash
   sudo yum install git -y
   ```

3. Clone the repository:
   ```bash
   git clone https://github.com/prof-project/nitro-example.git
   ```

4. Build the project by following [this article](https://docs.aws.amazon.com/enclaves/latest/user/developing-applications-linux.html)
   
   In Step 9, run the client side by executing:
   ```bash
   ./vsock_sample/rs/target/x86_64-unknown-linux-musl/release/vsock-sample client --cid 6 --port 5005
   ```
   The server should display `Hello, World!` in the enclave terminal.

## Proxied gRPC server inside a Nitro Enclave with a plain gRPC client

Before running the gRPC server, ensure that all previously launched enclaves are terminated:

```
nitro-cli describe-enclaves
sudo nitro-cli terminate-enclave --enclave-id <enclave-id>
```

Further, ensure that enough memory is allocated. This machine will require 1772 MB of memory.
```
sudo cat /etc/nitro_enclaves/allocator.yaml
```

Start by installing relevant tools for protobuf and gRPC.
```
# Install protoc (if not installed)
sudo yum install -y protobuf-compiler

# Install Go plugins for gRPC and protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
```

- Generate the proto files
```
protoc --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. proto/echo.proto
```

- Build the docker image
```
cd grpc-nitro-enclave/
sudo docker build -t grpc-nitro-enclave .
```

- Build the enclave image
```
sudo nitro-cli build-enclave --docker-uri grpc-nitro-enclave --output-file grpc-nitro-enclave.eif
```

- Run the enclave
```
sudo nitro-cli run-enclave --eif-path grpc-nitro-enclave.eif --memory 2000 --cpu-count 2 --enclave-cid 16 --debug-mode
```

- Check the enclave terminal
```
sudo nitro-cli console --enclave-id <enclave-id>
```

Now, one needs to run the client. For this, we need to run Socat to Forward TCP to VSOCK

```
sudo socat TCP-LISTEN:50051,reuseaddr,fork VSOCK-CONNECT:16:50051
```

- Build the client
```
go build -o client client.go
```

- Run the client
```
./client "Hello from outside the enclave!"
```

- Expected Output in the enclave terminal: `2024/10/11 10:37:29 Received: Hello from outside the enclave!`
- Expected Output in the client terminal: 
```
2024/10/11 10:41:08 Server response: Echo: Hello from outside the enclave!
2024/10/11 10:41:08 Round-trip time: 3.991322ms
```

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.

