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

# Build server with absolute paths
RUN CGO_ENABLED=1 \
    C_INCLUDE_PATH="/workspace/whisper.cpp/include:/workspace/whisper.cpp/ggml/include" \
    LIBRARY_PATH="/workspace/whisper.cpp/build_go/src:/workspace/whisper.cpp/build_go/ggml/src:/workspace/whisper.cpp/build_go/ggml/src/ggml-metal:/workspace/whisper.cpp/build_go/ggml/src/ggml-cpu:/workspace/whisper.cpp/build_go/ggml/src/ggml-blas" \
    go build -ldflags="-s -w -linkmode external -extldflags '-static'" -a -installsuffix cgo -o whisper-server .

# Download models
RUN cd whisper.cpp && \
    ./models/download-ggml-model.sh base.en && \
    ./models/download-ggml-model.sh base

# Runtime stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache curl ca-certificates tzdata

WORKDIR /app

# Create non-root user
RUN adduser -D -u 1000 whisper

# Copy server binary
COPY --from=builder /workspace/whisper-server /app/whisper-server

# Copy models and samples
COPY --from=builder /workspace/whisper.cpp/models/ /app/whisper.cpp/models/
COPY --from=builder /workspace/whisper.cpp/samples/ /app/whisper.cpp/samples/

# Set permissions
RUN chown -R whisper:whisper /app && \
    chmod +x /app/whisper-server

# Switch to non-root user
USER whisper

# Environment variables
ENV WHISPER_MODEL=base.en \
    SERVER_PORT=8080 \
    MAX_FILE_SIZE=25000000

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run server
CMD ["./whisper-server"]