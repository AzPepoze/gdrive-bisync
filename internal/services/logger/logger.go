package logger

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	Log     *slog.Logger
	logFile *os.File
	once    sync.Once

	outputMu           sync.Mutex
	lastProgressLen    int
	currentProgressMsg string
	progressVisible    bool

	consoleLevelVar = &slog.LevelVar{}
	categoryColors  sync.Map
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

	if lastProgressLen > 0 {
		if _, err := fmt.Fprint(h.w, "\r\033[2K"); err != nil {
			return err
		}
		lastProgressLen = 0
	}

	level := r.Level.String()
	t := r.Time.Format("2006-01-02 15:04:05")

	var levelStr string
	switch r.Level {
	case slog.LevelDebug:
		levelStr = "\033[36mDEBUG\033[0m"
	case slog.LevelInfo:
		levelStr = "\033[32mINFO \033[0m"
	case slog.LevelWarn:
		levelStr = "\033[33mWARN \033[0m"
	case slog.LevelError:
		levelStr = "\033[31mERROR\033[0m"
	default:
		levelStr = level
	}

	msg := r.Message
	attrs := ""
	category := "APP"
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "category" {
			category = strings.ToUpper(a.Value.String())
			return true
		}
		attrs += fmt.Sprintf(" %s=%v", a.Key, a.Value.Any())
		return true
	})

	categoryLabel := fmt.Sprintf("\033[38;5;%dm%-10s\033[0m", categoryColor(category), category)
	_, err := fmt.Fprintf(h.w, "%s [%s] [%s] %s%s\n", t, levelStr, categoryLabel, msg, attrs)

	if currentProgressMsg != "" {
		if _, progressErr := fmt.Fprintf(h.w, "\r\033[36m[SCAN]\033[0m %s", currentProgressMsg); err == nil {
			err = progressErr
		}
		lastProgressLen = len(currentProgressMsg) + 7
	}

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

func Init(showLogs bool) {
	once.Do(func() {
		consoleHandler := NewConsoleHandler(os.Stdout, &slog.HandlerOptions{
			Level: consoleLevelVar,
		})

		var handlers []slog.Handler
		handlers = append(handlers, consoleHandler)

		if showLogs {
			logDir := GetLogDir()
			if err := os.MkdirAll(logDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
			}
			filename := filepath.Join(logDir, fmt.Sprintf("sync-%s.log", time.Now().Format("2006-01-02")))
			f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
			} else {
				logFile = f
				fileHandler := slog.NewTextHandler(f, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				})
				handlers = append(handlers, fileHandler)
			}
		}

		Log = slog.New(&MultiHandler{handlers: handlers})
		slog.SetDefault(Log)
	})

	if showLogs {
		consoleLevelVar.Set(slog.LevelInfo)
	} else {
		consoleLevelVar.Set(slog.LevelError + 100)
	}
	outputMu.Lock()
	if info, err := os.Stdout.Stat(); err == nil {
		progressVisible = showLogs && info.Mode()&os.ModeCharDevice != 0
	}
	outputMu.Unlock()
}

func Close() {
	if logFile != nil {
		_ = logFile.Close()
	}
}

func UpdateStatus(msg string) {
	outputMu.Lock()
	defer outputMu.Unlock()
	if !progressVisible {
		currentProgressMsg = ""
		lastProgressLen = 0
		return
	}

	if lastProgressLen > 0 {
		fmt.Print("\r\033[2K")
	}

	if msg == "" {
		currentProgressMsg = ""
		lastProgressLen = 0
		return
	}

	const maxLen = 70
	if len(msg) > maxLen {
		msg = "..." + msg[len(msg)-(maxLen-3):]
	}

	currentProgressMsg = msg
	fmt.Printf("\r\033[36m[SCAN]\033[0m %s", msg)
	lastProgressLen = len(msg) + 7
}

func Info(msg string, args ...any) {
	logWithCategory(slog.LevelInfo, callerCategory(), msg, args...)
}

func Error(msg string, args ...any) {
	logWithCategory(slog.LevelError, callerCategory(), msg, args...)
}

func Warn(msg string, args ...any) {
	logWithCategory(slog.LevelWarn, callerCategory(), msg, args...)
}

func Debug(msg string, args ...any) {
	logWithCategory(slog.LevelDebug, callerCategory(), msg, args...)
}

func InfoCategory(category string, msg string, args ...any) {
	logWithCategory(slog.LevelInfo, category, msg, args...)
}

func WarnCategory(category string, msg string, args ...any) {
	logWithCategory(slog.LevelWarn, category, msg, args...)
}

func ErrorCategory(category string, msg string, args ...any) {
	logWithCategory(slog.LevelError, category, msg, args...)
}

func logWithCategory(level slog.Level, category string, msg string, args ...any) {
	if Log == nil {
		Init(false)
	}
	attributes := make([]any, 0, len(args)+2)
	attributes = append(attributes, "category", strings.ToUpper(category))
	attributes = append(attributes, args...)
	Log.Log(context.Background(), level, msg, attributes...)
}

func callerCategory() string {
	_, file, _, ok := runtime.Caller(2)
	if !ok {
		return "APP"
	}
	slashPath := filepath.ToSlash(file)
	if marker := "/internal/"; strings.Contains(slashPath, marker) {
		remainder := strings.SplitN(slashPath, marker, 2)[1]
		return strings.ToUpper(strings.SplitN(remainder, "/", 2)[0])
	}
	if marker := "/cmd/"; strings.Contains(slashPath, marker) {
		return "CMD"
	}
	return strings.ToUpper(filepath.Base(filepath.Dir(file)))
}

func categoryColor(category string) uint32 {
	category = strings.ToUpper(category)
	if cached, ok := categoryColors.Load(category); ok {
		return cached.(uint32)
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(category))
	// Use the 6×6×6 terminal color cube, avoiding low-contrast system colors.
	color := uint32(16 + hash.Sum32()%216)
	categoryColors.Store(category, color)
	return color
}
