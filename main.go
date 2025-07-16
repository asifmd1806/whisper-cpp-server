package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/fx"
	"go.uber.org/zap"

	whisper "github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// Config holds application configuration
type Config struct {
	ModelName   string
	Port        string
	MaxFileSize int64
	ModelPath   string
}

// WhisperService handles whisper model operations
type WhisperService struct {
	model     whisper.Model
	modelName string
	logger    *zap.Logger
}

// Server handles HTTP requests
type Server struct {
	whisperService *WhisperService
	config         *Config
	logger         *zap.Logger
}

// Response types
type TranscriptionResponse struct {
	Success       bool              `json:"success"`
	Transcription string            `json:"transcription"`
	Segments      []SegmentResponse `json:"segments"`
	Language      string            `json:"language"`
	Model         string            `json:"model"`
	Duration      float64           `json:"duration"`
	Error         string            `json:"error,omitempty"`
}

type SegmentResponse struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type ServerInfo struct {
	Service   string            `json:"service"`
	Version   string            `json:"version"`
	Model     string            `json:"model"`
	Languages []string          `json:"languages"`
	Endpoints map[string]string `json:"endpoints"`
}

type HealthResponse struct {
	Status string `json:"status"`
	Model  string `json:"model"`
}

// NewConfig creates application configuration
func NewConfig() *Config {
	modelName := getEnv("WHISPER_MODEL", "base.en")
	return &Config{
		ModelName:   modelName,
		Port:        getEnv("SERVER_PORT", "8080"),
		MaxFileSize: getEnvInt("MAX_FILE_SIZE", 25*1024*1024),
		ModelPath:   filepath.Join("whisper.cpp", "models", fmt.Sprintf("ggml-%s.bin", modelName)),
	}
}

// NewLogger creates a structured logger
func NewLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}

// NewWhisperService creates whisper service
func NewWhisperService(config *Config, logger *zap.Logger) (*WhisperService, error) {
	// Check if model exists
	if _, err := os.Stat(config.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("model file not found: %s", config.ModelPath)
	}

	// Load the model
	logger.Info("Loading whisper model", zap.String("path", config.ModelPath))
	model, err := whisper.New(config.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load model: %w", err)
	}

	logger.Info("Model loaded successfully",
		zap.String("model", config.ModelName),
		zap.Bool("multilingual", model.IsMultilingual()),
		zap.Strings("languages", model.Languages()),
	)

	return &WhisperService{
		model:     model,
		modelName: config.ModelName,
		logger:    logger,
	}, nil
}

// NewServer creates HTTP server
func NewServer(whisperService *WhisperService, config *Config, logger *zap.Logger) *Server {
	return &Server{
		whisperService: whisperService,
		config:         config,
		logger:         logger,
	}
}

// SetupRoutes configures HTTP routes
func (s *Server) SetupRoutes() *mux.Router {
	r := mux.NewRouter()
	
	r.Use(s.loggingMiddleware)
	r.Use(s.corsMiddleware)

	r.HandleFunc("/", s.handleRoot).Methods("GET")
	r.HandleFunc("/health", s.handleHealth).Methods("GET")
	r.HandleFunc("/transcribe", s.handleTranscribe).Methods("POST")

	return r
}

// HTTP handlers
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	endpoints := map[string]string{
		"transcribe": "/transcribe",
		"health":     "/health",
	}

	info := ServerInfo{
		Service:   "Whisper.cpp Server",
		Version:   "1.0.0",
		Model:     s.config.ModelName,
		Languages: s.whisperService.model.Languages(),
		Endpoints: endpoints,
	}

	s.sendJSON(w, info)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check if whisper service is available and model is loaded
	if s.whisperService == nil || s.whisperService.model == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		response := HealthResponse{
			Status: "unhealthy",
			Model:  s.config.ModelName,
		}
		s.sendJSON(w, response)
		return
	}

	// Model is loaded and ready
	response := HealthResponse{
		Status: "healthy",
		Model:  s.config.ModelName,
	}
	s.sendJSON(w, response)
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	err := r.ParseMultipartForm(s.config.MaxFileSize)
	if err != nil {
		s.sendError(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		s.sendError(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Check file size
	if header.Size > s.config.MaxFileSize {
		s.sendError(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Validate file type
	if !s.isValidAudioFile(header.Filename) {
		s.sendError(w, "Invalid file type. Only WAV files are supported", http.StatusBadRequest)
		return
	}

	// Read file content
	fileContent, err := io.ReadAll(file)
	if err != nil {
		s.sendError(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	// Get optional parameters
	language := r.FormValue("language")
	if language == "" {
		language = "auto"
	}

	// Process the audio
	result, err := s.whisperService.ProcessAudio(fileContent, language)
	if err != nil {
		s.logger.Error("Failed to process audio", zap.Error(err))
		s.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.sendJSON(w, result)
}

// WhisperService methods
func (ws *WhisperService) ProcessAudio(audioData []byte, language string) (*TranscriptionResponse, error) {
	// Convert audio to the required format
	audioFloat32, err := convertWAVToFloat32(audioData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert audio: %w", err)
	}

	// Create context for processing
	ctx, err := ws.model.NewContext()
	if err != nil {
		return nil, fmt.Errorf("failed to create context: %w", err)
	}

	// Set parameters
	ctx.SetLanguage(language)
	ctx.SetTranslate(false)
	ctx.SetTokenTimestamps(true)
	ctx.SetMaxSegmentLength(0)
	ctx.SetThreads(4)

	// Process audio
	err = ctx.Process(audioFloat32, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to process audio: %w", err)
	}

	// Collect segments
	var segments []SegmentResponse
	var fullText string
	var duration float64

	for {
		segment, err := ctx.NextSegment()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get segment: %w", err)
		}

		segmentResp := SegmentResponse{
			Start: segment.Start.Seconds(),
			End:   segment.End.Seconds(),
			Text:  strings.TrimSpace(segment.Text),
		}
		segments = append(segments, segmentResp)
		fullText += segmentResp.Text + " "
		
		if segment.End.Seconds() > duration {
			duration = segment.End.Seconds()
		}
	}

	return &TranscriptionResponse{
		Success:       true,
		Transcription: strings.TrimSpace(fullText),
		Segments:      segments,
		Language:      ctx.DetectedLanguage(),
		Model:         ws.modelName,
		Duration:      duration,
	}, nil
}

func (ws *WhisperService) Close() {
	if ws.model != nil {
		ws.model.Close()
	}
}

// Utility functions
func convertWAVToFloat32(data []byte) ([]float32, error) {
	if len(data) < 44 {
		return nil, fmt.Errorf("invalid WAV file: too short")
	}

	// Validate WAV header
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("invalid WAV file: not a valid WAVE file")
	}

	// Find data chunk
	dataChunkPos := -1
	for i := 12; i < len(data)-8; i++ {
		if string(data[i:i+4]) == "data" {
			dataChunkPos = i
			break
		}
	}

	if dataChunkPos == -1 {
		return nil, fmt.Errorf("invalid WAV file: no data chunk found")
	}

	// Get data chunk size
	dataSize := int(data[dataChunkPos+4]) | int(data[dataChunkPos+5])<<8 | 
		int(data[dataChunkPos+6])<<16 | int(data[dataChunkPos+7])<<24

	// Extract audio data
	audioData := data[dataChunkPos+8 : dataChunkPos+8+dataSize]
	
	// Convert 16-bit PCM to float32
	samples := make([]float32, len(audioData)/2)
	for i := 0; i < len(samples); i++ {
		// Read 16-bit little-endian sample
		sample := int16(audioData[i*2]) | int16(audioData[i*2+1])<<8
		// Convert to float32 normalized to [-1, 1]
		samples[i] = float32(sample) / 32768.0
	}

	return samples, nil
}

func (s *Server) isValidAudioFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".wav"
}

func (s *Server) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := TranscriptionResponse{
		Success: false,
		Error:   message,
	}
	
	json.NewEncoder(w).Encode(response)
}

// Middleware
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("Request processed",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Duration("duration", time.Since(start)),
		)
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// HTTP server lifecycle
func NewHTTPServer(lc fx.Lifecycle, server *Server, config *Config, logger *zap.Logger) {
	httpServer := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      server.SetupRoutes(),
		ReadTimeout:  300 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting HTTP server",
				zap.String("port", config.Port),
				zap.String("model", config.ModelName),
				zap.Int64("max_file_size", config.MaxFileSize),
			)

			go func() {
				if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Fatal("Server failed to start", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping HTTP server")
			return httpServer.Shutdown(ctx)
		},
	})
}

// Cleanup whisper service
func RegisterWhisperCleanup(lc fx.Lifecycle, ws *WhisperService) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			ws.Close()
			return nil
		},
	})
}

func main() {
	fx.New(
		fx.Provide(
			NewConfig,
			NewLogger,
			NewWhisperService,
			NewServer,
		),
		fx.Invoke(
			NewHTTPServer,
			RegisterWhisperCleanup,
		),
	).Run()
}