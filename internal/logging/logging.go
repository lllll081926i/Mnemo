// Package logging provides the application's structured logging pipeline.
// Logs are written to stderr for development and to data/logs/mnemo.log when
// the application data directory is available.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
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
	writeMu    sync.Mutex
)

func init() {
	level.Set(slog.LevelInfo)
	// Keep an immediate stderr logger so early Wails/dev-mode calls are visible
	// before Startup has resolved the application data directory.
	setDefaultLogger(os.Stderr)
}

type scopedAttr struct {
	groups []string
	attr   slog.Attr
}

// compactHandler keeps log lines readable for humans while retaining every
// structured diagnostic field. A line starts with its level, then the concise
// event message; timestamp/source/attributes follow after a separator.
type compactHandler struct {
	writer io.Writer
	level  slog.Leveler
	attrs  []scopedAttr
	groups []string
}

func setDefaultLogger(writer io.Writer) {
	slog.SetDefault(slog.New(&compactHandler{writer: writer, level: &level}))
}

func (h *compactHandler) Enabled(_ context.Context, recordLevel slog.Level) bool {
	return recordLevel >= h.level.Level()
}

func (h *compactHandler) Handle(_ context.Context, record slog.Record) error {
	message := sanitizeText(record.Message)
	if message == "" {
		message = "event"
	}
	parts := []string{"time=" + record.Time.Local().Format("2006-01-02T15:04:05.000-07:00")}
	if record.Level >= slog.LevelWarn || record.Level <= slog.LevelDebug {
		if source := compactSource(record.PC); source != "" {
			parts = append(parts, "source="+source)
		}
	}
	for _, item := range h.attrs {
		appendCompactAttr(&parts, item.groups, item.attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendCompactAttr(&parts, h.groups, attr)
		return true
	})

	var line strings.Builder
	line.WriteString("[")
	line.WriteString(compactLevel(record.Level))
	line.WriteString("] ")
	line.WriteString(message)
	if len(parts) > 0 {
		line.WriteString(" | ")
		line.WriteString(strings.Join(parts, " "))
	}
	line.WriteByte('\n')

	writeMu.Lock()
	_, err := io.WriteString(h.writer, line.String())
	writeMu.Unlock()
	return err
}

func (h *compactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append([]scopedAttr(nil), h.attrs...)
	for _, attr := range attrs {
		clone.attrs = append(clone.attrs, scopedAttr{groups: append([]string(nil), h.groups...), attr: attr})
	}
	return &clone
}

func (h *compactHandler) WithGroup(name string) slog.Handler {
	if strings.TrimSpace(name) == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

func compactLevel(recordLevel slog.Level) string {
	switch {
	case recordLevel <= slog.LevelDebug:
		return "DEBUG"
	case recordLevel < slog.LevelWarn:
		return "INFO"
	case recordLevel < slog.LevelError:
		return "WARN"
	default:
		return "ERROR"
	}
}

func compactSource(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	file := filepath.ToSlash(frame.File)
	if index := strings.LastIndex(file, "/internal/"); index >= 0 {
		file = file[index+1:]
	} else {
		file = filepath.Base(file)
	}
	return file + ":" + strconv.Itoa(frame.Line)
}

func appendCompactAttr(parts *[]string, groups []string, attr slog.Attr) {
	if attr.Equal(slog.Attr{}) {
		return
	}
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		nextGroups := groups
		if attr.Key != "" {
			nextGroups = append(append([]string(nil), groups...), attr.Key)
		}
		for _, child := range value.Group() {
			appendCompactAttr(parts, nextGroups, child)
		}
		return
	}
	keys := append([]string(nil), groups...)
	if attr.Key != "" {
		keys = append(keys, attr.Key)
	}
	if len(keys) == 0 {
		return
	}
	key := strings.Join(keys, ".")
	if isSensitiveKey(key) && !isSafePresenceValue(key, value.Any()) {
		*parts = append(*parts, key+"=[REDACTED]")
		return
	}
	*parts = append(*parts, key+"="+compactValue(value))
}

func compactValue(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return quoteCompact(sanitizeText(value.String()))
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'g', -1, 64)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return quoteCompact(sanitizeText(err.Error()))
		}
		return quoteCompact(sanitizeText(fmt.Sprint(value.Any())))
	default:
		return quoteCompact(sanitizeText(value.String()))
	}
}

func quoteCompact(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\r\n|=\"") {
		return strconv.Quote(value)
	}
	return value
}

// Configure adds the persistent log file while retaining stderr output.
// Calling Configure again replaces the previous file safely.
func Configure(rootDir string) error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		setDefaultLogger(os.Stderr)
		writeMu.Lock()
		_ = logFile.Close()
		writeMu.Unlock()
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
	setDefaultLogger(writer)
	Info("logging initialized", "file", path, "pid", os.Getpid())
	return nil
}

// Close flushes and closes the persistent log file. stderr remains available.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile == nil {
		return
	}
	setDefaultLogger(os.Stderr)
	writeMu.Lock()
	_ = logFile.Sync()
	_ = logFile.Close()
	writeMu.Unlock()
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

// RedactText returns the same one-line, credential-safe representation used
// by log attributes. It is also used before diagnostic errors are persisted
// or sent to the frontend.
func RedactText(value string) string { return sanitizeText(value) }

// Debug records a diagnostic event.
func Debug(msg string, args ...any) { logRecord(slog.LevelDebug, msg, args...) }

// Info records a normal lifecycle or operation event.
func Info(msg string, args ...any) { logRecord(slog.LevelInfo, msg, args...) }

// Warn records a recoverable problem.
func Warn(msg string, args ...any) { logRecord(slog.LevelWarn, msg, args...) }

// Error records a failed operation.
func Error(msg string, args ...any) { logRecord(slog.LevelError, msg, args...) }

func logRecord(recordLevel slog.Level, message string, args ...any) {
	logger := slog.Default()
	ctx := context.Background()
	if !logger.Enabled(ctx, recordLevel) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	record := slog.NewRecord(time.Now(), recordLevel, message, pcs[0])
	record.Add(sanitizeArgs(args...)...)
	_ = logger.Handler().Handle(ctx, record)
}

// SetLevel changes the runtime verbosity. Supported values are debug, info,
// warning and error. Info is the default so lifecycle and operation context
// remain available without the per-request noise emitted at debug level.
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
		setDefaultLogger(os.Stderr)
		writeMu.Lock()
		_ = logFile.Close()
		writeMu.Unlock()
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
	urlPattern           = regexp.MustCompile(`(?i)https?://[^\s|"'<>]+`)
)

func sanitizeArgs(args ...any) []any {
	out := make([]any, len(args))
	for i, value := range args {
		if i > 0 {
			if key, ok := args[i-1].(string); ok && isSensitiveKey(key) {
				if !isSafePresenceValue(key, value) {
					out[i] = "[REDACTED]"
					continue
				}
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

func isSafePresenceValue(key string, value any) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if !strings.HasPrefix(key, "has_") && !strings.HasSuffix(key, "_present") {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return true
	case string:
		return strings.EqualFold(typed, "true") || strings.EqualFold(typed, "false")
	default:
		return false
	}
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
