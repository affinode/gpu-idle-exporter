.PHONY: build test vet docker deploy clean

build:
	go build -o gpu-idle-exporter ./cmd/

test:
	go test ./...

vet:
	go vet ./...

docker:
	docker build -t ghcr.io/affinode/gpu-idle-exporter:latest -f deployments/docker/Dockerfile .

deploy:
	kubectl apply -f deployments/k8s/daemonset.yaml

clean:
	rm -f gpu-idle-exporter
