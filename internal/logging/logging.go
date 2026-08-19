// Package logging provides the application's structured logging pipeline.
// Logs are written to stderr for development and to data/logs/mnemo.log when
// the application data directory is available.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const maxLogSize = 10 << 20

var (
	mu         sync.Mutex
	logFile    *os.File
	logPath    string
	dataDir    string
	configured bool
	level      slog.LevelVar
)

func init() {
	level.Set(slog.LevelWarn)
	// Keep an immediate stderr logger so early Wails/dev-mode calls are visible
	// before Startup has resolved the application data directory.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: &level})))
}

// Configure adds the persistent log file while retaining stderr output.
// Calling Configure again replaces the previous file safely.
func Configure(rootDir string) error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	configured = false
	logPath = ""
	dataDir = ""
	if rootDir == "" {
		return fmt.Errorf("empty data directory")
	}
	logDir := filepath.Join(rootDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(logDir, "mnemo.log")
	if info, err := os.Stat(path); err == nil && info.Size() >= maxLogSize {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	logFile = f
	logPath = path
	dataDir = rootDir
	configured = true
	writer := io.MultiWriter(os.Stderr, f)
	// Keep records portable: absolute source paths are noisy and expose the
	// developer's machine layout, so diagnostics use the event and fields only.
	slog.SetDefault(slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: &level})))
	slog.Info("logging initialized", "file", path, "pid", os.Getpid())
	return nil
}

// Close flushes and closes the persistent log file. stderr remains available.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile == nil {
		return
	}
	_ = logFile.Sync()
	_ = logFile.Close()
	logFile = nil
	configured = false
	logPath = ""
	dataDir = ""
}

// Path returns the active persistent log path, or an empty string before setup.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// Configured reports whether a persistent log file is active.
func Configured() bool {
	mu.Lock()
	defer mu.Unlock()
	return configured
}

// Duration returns a stable elapsed duration for structured timing fields.
func Duration(start time.Time) time.Duration { return time.Since(start).Round(time.Millisecond) }

// Debug records a diagnostic event.
func Debug(msg string, args ...any) { slog.Debug(msg, sanitizeArgs(args...)...) }

// Info records a normal lifecycle or operation event.
func Info(msg string, args ...any) { slog.Info(msg, sanitizeArgs(args...)...) }

// Warn records a recoverable problem.
func Warn(msg string, args ...any) { slog.Warn(msg, sanitizeArgs(args...)...) }

// Error records a failed operation.
func Error(msg string, args ...any) { slog.Error(msg, sanitizeArgs(args...)...) }

// SetLevel changes the runtime verbosity. Supported values are debug, info,
// warning and error. Warning is the default to keep normal operation quiet.
func SetLevel(value string) error {
	var next slog.Level
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		next = slog.LevelDebug
	case "info":
		next = slog.LevelInfo
	case "warning", "warn", "":
		next = slog.LevelWarn
	case "error":
		next = slog.LevelError
	default:
		return fmt.Errorf("unsupported log level: %s", value)
	}
	level.Set(next)
	Info("log level changed", "level", LogLevel())
	return nil
}

// LogLevel returns the normalized current level.
func LogLevel() string {
	switch level.Level() {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelError:
		return "error"
	default:
		return "warning"
	}
}

// Clear removes the active log and its rotated backup, then starts a fresh
// file without changing the current verbosity.
func Clear() error {
	mu.Lock()
	root := dataDir
	oldPath := logPath
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	if oldPath != "" {
		_ = os.Remove(oldPath)
		_ = os.Remove(oldPath + ".1")
	}
	mu.Unlock()
	if root == "" {
		return fmt.Errorf("persistent logging is not initialized")
	}
	return Configure(root)
}

var (
	sensitivePattern     = regexp.MustCompile(`(?i)(password|passwd|cookie|authorization|captcha_token|refresh_token|access_token|creditkey|device_id|deviceid|token)([=:])[^\s&|]+`)
	sensitiveJSONPattern = regexp.MustCompile(`(?i)("(?:password|passwd|cookie|authorization|token|captcha_token|refresh_token|access_token|creditkey|device_id|deviceid)"\s*:\s*")[^"]*(")`)
	urlPattern           = regexp.MustCompile(`(?i)https?://[^\s|]+`)
)

func sanitizeArgs(args ...any) []any {
	out := make([]any, len(args))
	for i, value := range args {
		if i > 0 {
			if key, ok := args[i-1].(string); ok && isSensitiveKey(key) {
				out[i] = "[REDACTED]"
				continue
			}
		}
		if text, ok := value.(error); ok {
			out[i] = sanitizeText(text.Error())
		} else if text, ok := value.(string); ok {
			out[i] = sanitizeText(text)
		} else {
			out[i] = value
		}
	}
	return out
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "password") || strings.Contains(key, "cookie") || strings.Contains(key, "token") ||
		strings.Contains(key, "creditkey") || strings.Contains(key, "device_id") || strings.Contains(key, "deviceid") ||
		key == "authorization"
}

func sanitizeText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	value = urlPattern.ReplaceAllStringFunc(value, func(raw string) string {
		trimmed := strings.TrimRight(raw, `.,;)]}`)
		u, err := url.Parse(trimmed)
		if err != nil || u.Hostname() == "" {
			return "[URL_REDACTED]"
		}
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	})
	value = sensitiveJSONPattern.ReplaceAllString(value, `${1}[REDACTED]${2}`)
	value = sensitivePattern.ReplaceAllString(value, `$1$2[REDACTED]`)
	if len(value) > 512 {
		return value[:512] + "..."
	}
	return value
}
