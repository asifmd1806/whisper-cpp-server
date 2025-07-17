#!/bin/sh

# Whisper CPP Server Entrypoint
# This script downloads the model if it doesn't exist and starts the server

set -e

# Default values
WHISPER_MODEL=${WHISPER_MODEL:-base.en}
MODELS_DIR="/app/whisper.cpp/models"
MODEL_FILE="$MODELS_DIR/ggml-$WHISPER_MODEL.bin"

echo "Starting Whisper CPP Server..."
echo "Model: $WHISPER_MODEL"
echo "Models directory: $MODELS_DIR"

# Check if model file exists
if [ ! -f "$MODEL_FILE" ]; then
    echo "Model file not found: $MODEL_FILE"
    echo "Downloading model: $WHISPER_MODEL"
    
    # Change to models directory
    cd "$MODELS_DIR"
    
    # Download the model using curl (Alpine's wget doesn't support required options)
    MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-${WHISPER_MODEL}.bin"
    MODEL_FILE="ggml-${WHISPER_MODEL}.bin"
    
    echo "Downloading from: $MODEL_URL"
    if ! curl -L --fail --progress-bar -o "$MODEL_FILE" "$MODEL_URL"; then
        echo "ERROR: Failed to download model: $WHISPER_MODEL"
        echo "Please check if the model name is valid."
        echo "Available models: tiny, tiny.en, base, base.en, small, small.en, medium, medium.en, large-v1, large-v2, large-v3, etc."
        exit 1
    fi
    
    echo "Model downloaded successfully: $MODEL_FILE"
else
    echo "Model file exists: $MODEL_FILE"
fi

# Change back to app directory
cd /app

# Start the server
echo "Starting server on port ${SERVER_PORT:-8080}..."
exec ./whisper-server