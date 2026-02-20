.PHONY: build test vet docker deploy deploy-deployment clean

build:
	go build -o gpu-idle-exporter ./cmd/

test:
	go test ./...

vet:
	go vet ./...

docker:
	docker build -t ghcr.io/affinode/gpu-idle-exporter:latest -f deployments/docker/Dockerfile .

deploy:
	kubectl apply -f examples/daemonset/daemonset.yaml

deploy-deployment:
	kubectl apply -f examples/deployment/deployment.yaml

clean:
	rm -f gpu-idle-exporter
