package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type config struct {
	port              string
	apiKey            string
	maxBodyBytes      int64
	commandTimeout    time.Duration
	workerLimit       int
	allowShellEscape  bool
	readTimeout       time.Duration
	readHeaderTimeout time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
}

var renderSem chan struct{}

func renderNativeLatex(cfg config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isPlainText(r.Header.Get("Content-Type")) {
			http.Error(w, "Unsupported Content-Type", http.StatusUnsupportedMediaType)
			return
		}
		if !authorize(cfg.apiKey, r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		select {
		case renderSem <- struct{}{}:
			defer func() { <-renderSem }()
		default:
			http.Error(w, "Service busy", http.StatusTooManyRequests)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, cfg.maxBodyBytes)
		defer r.Body.Close()

		latex, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read LaTeX content", http.StatusBadRequest)
			return
		}
		if len(latex) == 0 {
			http.Error(w, "Empty LaTeX content", http.StatusBadRequest)
			return
		}

		// Create a temporary directory
		dir, err := os.MkdirTemp("", "latex")
		if err != nil {
			http.Error(w, "Failed to create temporary directory", http.StatusInternalServerError)
			return
		}
		defer os.RemoveAll(dir)

		latexFileName := "document.tex"
		latexFilePath := filepath.Join(dir, latexFileName)
		pdfFileName := "document.pdf"
		pdfCroppedFileName := "document-cropped.pdf"
		pngFileNamePrefix := "output"
		pngFileName := fmt.Sprintf("%s-1.png", pngFileNamePrefix)

		// Write the LaTeX content to a .tex file
		err = os.WriteFile(latexFilePath, latex, 0644)
		if err != nil {
			http.Error(w, "Failed to write LaTeX file", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), cfg.commandTimeout)
		defer cancel()

		// Run pdflatex
		pdflatexArgs := []string{"-halt-on-error", "-interaction=nonstopmode"}
		if cfg.allowShellEscape {
			pdflatexArgs = append(pdflatexArgs, "-shell-escape")
		} else {
			pdflatexArgs = append(pdflatexArgs, "-no-shell-escape")
		}
		pdflatexArgs = append(pdflatexArgs, latexFileName)
		if _, err := runCommand(ctx, dir, "pdflatex", pdflatexArgs...); err != nil {
			http.Error(w, "pdflatex error", http.StatusInternalServerError)
			return
		}

		// Run pdfcrop
		if _, err := runCommand(ctx, dir, "pdfcrop", pdfFileName, pdfCroppedFileName); err != nil {
			http.Error(w, "pdfcrop error", http.StatusInternalServerError)
			return
		}

		// Run pdftoppm
		if _, err := runCommand(ctx, dir, "pdftoppm", "-png", "-r", "150", pdfCroppedFileName, pngFileNamePrefix); err != nil {
			http.Error(w, "pdftoppm error", http.StatusInternalServerError)
			return
		}

		// Read the generated PNG file
		pngFilePath := filepath.Join(dir, pngFileName)
		pngData, err := os.ReadFile(pngFilePath)
		if err != nil {
			http.Error(w, "Failed to read PNG file", http.StatusInternalServerError)
			return
		}

		// Write the PNG data to the response
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	}
}

func authorize(apiKey string, r *http.Request) bool {
	if apiKey == "" {
		return true
	}
	if r.Header.Get("X-API-Key") == apiKey {
		return true
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")) == apiKey
	}
	return false
}

func runCommand(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("command failed: %s %v: %v\n%s", name, args, err, string(output))
		return string(output), err
	}
	return string(output), nil
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func loadConfig() config {
	cfg := config{
		port:              getEnv("PORT", "8080"),
		apiKey:            os.Getenv("LATEX_SERVICE_API_KEY"),
		maxBodyBytes:      getEnvInt64("MAX_BODY_BYTES", 1<<20),
		commandTimeout:    getEnvDuration("COMMAND_TIMEOUT", 30*time.Second),
		workerLimit:       getEnvInt("WORKER_LIMIT", runtime.NumCPU()),
		allowShellEscape:  getEnvBool("ALLOW_SHELL_ESCAPE", false),
		readTimeout:       getEnvDuration("READ_TIMEOUT", 30*time.Second),
		readHeaderTimeout: getEnvDuration("READ_HEADER_TIMEOUT", 10*time.Second),
		writeTimeout:      getEnvDuration("WRITE_TIMEOUT", 60*time.Second),
		idleTimeout:       getEnvDuration("IDLE_TIMEOUT", 60*time.Second),
	}
	if cfg.workerLimit < 1 {
		cfg.workerLimit = 1
	}
	if cfg.maxBodyBytes < 1 {
		cfg.maxBodyBytes = 1 << 20
	}
	return cfg
}

func getEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvInt(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func getEnvInt64(key string, defaultValue int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func getEnvBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func isPlainText(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaType == "text/plain"
}

func main() {
	cfg := loadConfig()
	renderSem = make(chan struct{}, cfg.workerLimit)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/render-latex", renderNativeLatex(cfg))

	server := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           mux,
		ReadTimeout:       cfg.readTimeout,
		ReadHeaderTimeout: cfg.readHeaderTimeout,
		WriteTimeout:      cfg.writeTimeout,
		IdleTimeout:       cfg.idleTimeout,
	}

	log.Printf("Server started at port %s", cfg.port)
	if err := server.ListenAndServe(); err != nil {
		log.Printf("Failed to start server: %v", err)
	}
}
