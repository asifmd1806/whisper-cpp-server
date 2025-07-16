#!/bin/bash

# Simple build script for whisper-server

set -e

echo "Building whisper-server..."

# Build whisper.cpp with Go bindings
echo "Building whisper.cpp with Go bindings..."
cd whisper.cpp/bindings/go
make whisper
cd ../../..

# Get Go dependencies
echo "Getting Go dependencies..."
go mod tidy

# Build the server
echo "Building Go server..."
export CGO_ENABLED=1
export C_INCLUDE_PATH="./whisper.cpp/include:./whisper.cpp/ggml/include"
export LIBRARY_PATH="./whisper.cpp/build_go/src:./whisper.cpp/build_go/ggml/src:./whisper.cpp/build_go/ggml/src/ggml-metal:./whisper.cpp/build_go/ggml/src/ggml-cpu:./whisper.cpp/build_go/ggml/src/ggml-blas"
export GGML_METAL_PATH_RESOURCES="./whisper.cpp/"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"
export CGO_LDFLAGS="-framework Foundation -framework Metal -framework MetalKit -framework Accelerate"

go build -ldflags="-s -w" -o whisper-server .

echo "Build completed! Binary: whisper-server"
echo ""
echo "To run the server:"
echo "  ./whisper-server"
echo ""
echo "Environment variables:"
echo "  WHISPER_MODEL=base.en     # Model to use"
echo "  SERVER_PORT=8080          # Server port"
echo "  MAX_FILE_SIZE=25000000    # Max file size in bytes"