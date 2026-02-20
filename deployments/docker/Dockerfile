# Stage 1: Build Go binary
# go-nvml requires CGO_ENABLED=1 (cgo bindings to libnvidia-ml.so).
# At build time it only needs glibc headers (bundled in go-nvml module).
# At runtime, libnvidia-ml.so is loaded via dlopen from the host mount.
FROM golang:1.21-bookworm AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o gpu-idle-exporter ./cmd

# Stage 2: Runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /root/
COPY --from=builder /build/gpu-idle-exporter .

EXPOSE 9835

CMD ["./gpu-idle-exporter"]
