ENCLAVE_ID=$(shell nitro-cli describe-enclaves | jq -r '.[0].EnclaveID')

docker-reset:
	sudo docker system prune -a

docker-build:
	sudo docker build -t grpc-nitro-enclave -f grpc-nitro-enclave/Dockerfile .

rebuild-enclave: terminate-enclave build-enclave run-enclave

connect-rebuild-enclave: terminate-enclave build-enclave run-enclave connect-enclave

terminate-enclave:
	sudo nitro-cli terminate-enclave --all

build-enclave:
	@echo "Building enclave image..."
	sudo docker build --no-cache -t grpc-nitro-enclave -f grpc-nitro-enclave/Dockerfile .
	sudo nitro-cli build-enclave --docker-uri grpc-nitro-enclave --output-file grpc-nitro-enclave.eif

run-enclave:
	@echo "Running enclave..."
	sudo nitro-cli run-enclave --eif-path grpc-nitro-enclave.eif --memory 2000 --cpu-count 2 --enclave-cid 16 --debug-mode

connect-enclave:
	@if [ -n "$(ENCLAVE_ID)" ]; then \
		echo "Connecting to enclave console with ID: $(ENCLAVE_ID)"; \
		sudo nitro-cli console --enclave-id $(ENCLAVE_ID); \
	else \
		echo "No running enclave to connect to."; \
	fi

socat-run:
	sudo socat -d -d TCP-LISTEN:50051,reuseaddr,fork VSOCK-CONNECT:16:50051

client-run:
	sudo ./grpc-nitro-enclave/client "Hello from outside the enclave!"
