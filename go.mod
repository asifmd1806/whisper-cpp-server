module whisper-server

go 1.21

require (
	github.com/ggerganov/whisper.cpp/bindings/go v0.0.0-20241216130308-95ecf93acf1b
	github.com/gorilla/mux v1.8.1
	go.uber.org/fx v1.20.0
	go.uber.org/zap v1.26.0
)

require (
	go.uber.org/dig v1.17.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
)

replace github.com/ggerganov/whisper.cpp/bindings/go => ./whisper.cpp/bindings/go