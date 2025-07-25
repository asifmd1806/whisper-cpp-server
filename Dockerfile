FROM golang:1.23-bullseye AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    cmake \
    git \
    make \
    wget \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy go module files
COPY go.mod go.sum ./

# Copy whisper.cpp source
COPY whisper.cpp/ ./whisper.cpp/

# Build whisper.cpp with Go bindings
RUN cd whisper.cpp && rm -rf build_go && cd bindings/go && make whisper

# Copy source code
COPY main.go ./

# Download Go dependencies
RUN go mod tidy && go mod download

# Platform-specific build stage
FROM builder AS builder-amd64
RUN CGO_ENABLED=1 \
    C_INCLUDE_PATH="/workspace/whisper.cpp/include:/workspace/whisper.cpp/ggml/include" \
    LIBRARY_PATH="/workspace/whisper.cpp/build_go/src:/workspace/whisper.cpp/build_go/ggml/src:/workspace/whisper.cpp/build_go/ggml/src/ggml-metal:/workspace/whisper.cpp/build_go/ggml/src/ggml-cpu:/workspace/whisper.cpp/build_go/ggml/src/ggml-blas" \
    go build -ldflags="-s -w -linkmode external -extldflags '-static'" -a -installsuffix cgo -o whisper-server .

FROM builder AS builder-arm64
RUN CGO_ENABLED=1 \
    C_INCLUDE_PATH="/workspace/whisper.cpp/include:/workspace/whisper.cpp/ggml/include" \
    LIBRARY_PATH="/workspace/whisper.cpp/build_go/src:/workspace/whisper.cpp/build_go/ggml/src:/workspace/whisper.cpp/build_go/ggml/src/ggml-metal:/workspace/whisper.cpp/build_go/ggml/src/ggml-cpu:/workspace/whisper.cpp/build_go/ggml/src/ggml-blas" \
    go build -ldflags="-s -w -linkmode external -extldflags '-static'" -a -installsuffix cgo -o whisper-server .

FROM builder-${TARGETARCH} AS final-builder


# Runtime stage with platform-specific dependencies
FROM alpine:3.18 AS runtime-amd64
RUN apk add --no-cache curl ca-certificates tzdata

FROM alpine:3.18 AS runtime-arm64
RUN apk add --no-cache curl ca-certificates tzdata libc6-compat libstdc++

FROM runtime-${TARGETARCH} AS runtime

WORKDIR /app

# Create non-root user
RUN adduser -D -u 1000 whisper

# Copy server binary
COPY --from=final-builder /workspace/whisper-server /app/whisper-server

# Copy download script for runtime model downloading
COPY --from=final-builder /workspace/whisper.cpp/models/download-ggml-model.sh /app/whisper.cpp/models/download-ggml-model.sh
COPY --from=final-builder /workspace/whisper.cpp/samples/ /app/whisper.cpp/samples/

# Copy entrypoint script
COPY entrypoint.sh /app/entrypoint.sh

# Create models directory and set permissions
RUN mkdir -p /app/whisper.cpp/models && \
    chown -R whisper:whisper /app && \
    chmod +x /app/whisper-server && \
    chmod +x /app/whisper.cpp/models/download-ggml-model.sh && \
    chmod +x /app/entrypoint.sh

# Switch to non-root user
USER whisper

# Environment variables
ENV WHISPER_MODEL=base.en \
    SERVER_PORT=8080 \
    MAX_FILE_SIZE=25000000

# Expose port
EXPOSE 8080

# Health check - allow more time for model download during startup
HEALTHCHECK --interval=30s --timeout=10s --start-period=300s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run server via entrypoint script
CMD ["./entrypoint.sh"]