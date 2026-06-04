# syntax=docker/dockerfile:1

# ---- builder: compile binaries and bake references.bin ----
FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# AVX2 distance kernel is gated behind GOEXPERIMENT=simd; the binary still runs
# on non-AVX2 CPUs (it falls back to the scalar kernel at init). sonic gives a
# faster JSON bind/encode than encoding/json. Static, stripped binary.
ENV CGO_ENABLED=0 GOOS=linux GOEXPERIMENT=simd
RUN go build -tags=sonic -ldflags="-s -w" -o /out/api ./cmd/api
RUN go build -ldflags="-s -w" -o /out/preprocess ./cmd/preprocess

# Download the immutable 3M-reference dataset and build the int8 IVF index once,
# at image-build time. The request path never parses JSON or builds an index.
# target 0.998 selects nprobe=4 — the recall plateau (~99.86% decision agreement
# vs exact float kNN; the int8-quantization ceiling is ~99.88%) at minimal scan.
RUN /out/preprocess -output /out/references.bin -clusters 2048 -iters 15 \
    -sample 200000 -target 0.998 -validate 2000

# ---- runtime: tiny image with just the binary + index ----
FROM alpine:3.20 AS runtime

WORKDIR /app
COPY --from=builder /out/api .
COPY --from=builder /out/references.bin .

EXPOSE 9999
CMD ["./api"]
