# Whisper.cpp Go Server

Simple HTTP server for audio transcription using whisper.cpp with Go bindings.

## Build & Run

```bash
# Build
./build.sh

# Run
./whisper-server
```

## Docker

```bash
# Build image
docker build -t whisper-server .

# Run container
docker run -p 8080:8080 whisper-server
```

## Environment Variables

- `WHISPER_MODEL` - Model to use (default: base.en)
- `SERVER_PORT` - Server port (default: 8080)
- `MAX_FILE_SIZE` - Max file size in bytes (default: 25MB)

## API Endpoints

- `GET /` - Server info
- `GET /health` - Health check
- `POST /transcribe` - Transcribe audio file (WAV format)

## Usage

```bash
# Upload audio file
curl -X POST http://localhost:8080/transcribe \
  -F "file=@audio.wav" \
  -F "language=auto"
```