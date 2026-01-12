package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	Log     *slog.Logger
	logFile *os.File
	once    sync.Once
	
	outputMu        sync.Mutex
	lastProgressLen int
)

func GetLogDir() string {
	tmpDir := "/tmp/gdrive-bisync-logs"
	if err := os.MkdirAll(tmpDir, 0755); err == nil {
		return tmpDir
	}
	return "logs"
}

type ConsoleHandler struct {
	w    io.Writer
	opts *slog.HandlerOptions
}

func NewConsoleHandler(w io.Writer, opts *slog.HandlerOptions) *ConsoleHandler {
	return &ConsoleHandler{w: w, opts: opts}
}

func (h *ConsoleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

func (h *ConsoleHandler) Handle(ctx context.Context, r slog.Record) error {
	outputMu.Lock()
	defer outputMu.Unlock()

	// Clear any active progress line
	if lastProgressLen > 0 {
		fmt.Fprint(h.w, "\r\033[2K") 
		lastProgressLen = 0
	}

	level := r.Level.String()
	t := r.Time.Format("2006-01-02 15:04:05")
	
	// Colorize level
	var levelStr string
	switch r.Level {
	case slog.LevelDebug:
		levelStr = "\033[36mDEBUG\033[0m" // Cyan
	case slog.LevelInfo:
		levelStr = "\033[32mINFO \033[0m" // Green
	case slog.LevelWarn:
		levelStr = "\033[33mWARN \033[0m" // Yellow
	case slog.LevelError:
		levelStr = "\033[31mERROR\033[0m" // Red
	default:
		levelStr = level
	}

	msg := r.Message
	
	// Format attributes
	attrs := ""
	r.Attrs(func(a slog.Attr) bool {
		attrs += fmt.Sprintf(" %s=%v", a.Key, a.Value.Any())
		return true
	})

	_, err := fmt.Fprintf(h.w, "%s [%s] %s%s\n", t, levelStr, msg, attrs)
	return err
}

func (h *ConsoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *ConsoleHandler) WithGroup(name string) slog.Handler {
	return h
}

type MultiHandler struct {
	handlers []slog.Handler
}

func (m *MultiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

func (m *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}

func Init() {
	once.Do(func() {
		logDir := GetLogDir()
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		}

		filename := filepath.Join(logDir, fmt.Sprintf("sync-%s.log", time.Now().Format("2006-01-02")))
		f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(fmt.Sprintf("Failed to open log file: %v", err))
		}
		logFile = f

		// File Handler: JSON or Text, Level DEBUG (stores everything)
		fileHandler := slog.NewTextHandler(f, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})

		// Console Handler: Pretty text, Level INFO (user friendly)
		consoleHandler := NewConsoleHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})

		// Combine them
		multiHandler := &MultiHandler{
			handlers: []slog.Handler{consoleHandler, fileHandler},
		}

		Log = slog.New(multiHandler)
		slog.SetDefault(Log)
	})
}

func Close() {
	if logFile != nil {
		logFile.Close()
	}
}

func UpdateStatus(msg string) {
	outputMu.Lock()
	defer outputMu.Unlock()
	
	if lastProgressLen > 0 {
		fmt.Print("\r\033[2K")
	}

	if msg == "" {
		lastProgressLen = 0
		return
	}

	const maxLen = 70
	if len(msg) > maxLen {
		msg = "..." + msg[len(msg)-(maxLen-3):]
	}
	
	fmt.Printf("\r\033[36m[SCAN]\033[0m %s", msg)
	lastProgressLen = len(msg) + 7
}

func Info(msg string, args ...any) {
	if Log == nil { Init() }
	Log.Info(msg, args...)
}

func Error(msg string, args ...any) {
	if Log == nil { Init() }
	Log.Error(msg, args...)
}

func Warn(msg string, args ...any) {
	if Log == nil { Init() }
	Log.Warn(msg, args...)
}

func Debug(msg string, args ...any) {
	if Log == nil { Init() }
	Log.Debug(msg, args...)
}