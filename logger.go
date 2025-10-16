// Package harelog provides a structured, level-based logging solution.
// It is designed to be flexible, thread-safe, and particularly well-suited for
// use with Google Cloud Logging by supporting its special JSON fields.
package harelog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel defines the severity level of a log entry.
type LogLevel string

const (
	LogLevelOff      LogLevel = "OFF"
	LogLevelCritical LogLevel = "CRITICAL"
	LogLevelError    LogLevel = "ERROR"
	LogLevelWarn     LogLevel = "WARN"
	LogLevelInfo     LogLevel = "INFO"
	LogLevelDebug    LogLevel = "DEBUG"
	LogLevelAll      LogLevel = "ALL"
)

type logLevelValue int

const (
	logLevelValueOff logLevelValue = iota
	logLevelValueCritical
	logLevelValueError
	logLevelValueWarn
	logLevelValueInfo
	logLevelValueDebug
)

const (
	logLevelValueAll logLevelValue = math.MaxInt32
)

// sourceLocationMode defines the behavior for automatic source code location capturing.
type sourceLocationMode int

const (
	// SourceLocationModeNever disables automatic source location capturing.
	// This provides the best performance. This is the default behavior.
	SourceLocationModeNever sourceLocationMode = iota

	// SourceLocationModeAlways enables automatic source location capturing for all log levels.
	// This is very useful for development and debugging, but has a performance cost.
	SourceLocationModeAlways

	// SourceLocationModeErrorOrAbove enables automatic source location capturing only for
	// logs of ERROR severity or higher. This is a balanced mode for capturing
	// critical debug information in production with minimal performance impact.
	SourceLocationModeErrorOrAbove
)

var (
	std      = New()
	stdMutex = &sync.RWMutex{}

	// harelogPackage is the import path of this package, determined at runtime.
	harelogPackage string

	osExit = os.Exit
)

var levelMap = map[LogLevel]logLevelValue{
	LogLevelOff:      logLevelValueOff,
	LogLevelCritical: logLevelValueCritical,
	LogLevelError:    logLevelValueError,
	LogLevelWarn:     logLevelValueWarn,
	LogLevelInfo:     logLevelValueInfo,
	LogLevelDebug:    logLevelValueDebug,
	LogLevelAll:      logLevelValueAll,
}

var logEntryPool = sync.Pool{
	New: func() any {
		return &LogEntry{
			Labels:  make(map[string]string),
			Payload: make(map[string]interface{}),
		}
	},
}

func init() {
	// Determine the package path of this library at startup.
	harelogPackage = reflect.TypeOf(Logger{}).PkgPath()

	// Fail Fast: If the package path could not be determined, it's a catastrophic
	// failure. The findCaller function would not work correctly, so we should
	// panic immediately to alert the developer.
	if harelogPackage == "" {
		panic("harelog: could not determine package path for source location feature")
	}

	setupLogLevelFromEnv()
}

// setupLogLevelFromEnv reads the HARELOG_LEVEL environment variable and
// configures the default logger's log level accordingly.
func setupLogLevelFromEnv() {
	levelStr := os.Getenv("HARELOG_LEVEL")

	if levelStr == "" {
		return
	}

	level, err := ParseLogLevel(levelStr)
	if err != nil {
		log.Printf("harelog: invalid HARELOG_LEVEL value %q, using default level", levelStr)

		return
	}

	SetDefaultLogLevel(level)
}

// ParseLogLevel parses a string into a LogLevel.
// It is case-insensitive. It returns an error if the input string is not a valid log level.
func ParseLogLevel(levelStr string) (LogLevel, error) {
	level := LogLevel(strings.ToUpper(levelStr))
	if _, ok := levelMap[level]; ok {
		return level, nil
	}

	return "", errors.New("invalid log level: " + levelStr)
}

// --- GCP-specific structured data ---

// HTTPRequest bundles information about an HTTP request for structured logging.
// When included in a log entry, Cloud Logging can interpret it to display request details.
type HTTPRequest struct {
	RequestMethod string `json:"requestMethod,omitempty"`
	RequestURL    string `json:"requestUrl,omitempty"`
	Status        int    `json:"status,omitempty"`
	UserAgent     string `json:"userAgent,omitempty"`
	RemoteIP      string `json:"remoteIp,omitempty"`
	Latency       string `json:"latency,omitempty"`
}

// SourceLocation represents the location in the source code where a log entry was generated.
type SourceLocation struct {
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Function string `json:"function,omitempty"`
}

// --- Log Entry Structure ---

// LogEntry is the internal data container for a single log entry.
type LogEntry struct {
	Message        string          `json:"message"`
	Severity       LogLevel        `json:"severity,omitempty"`
	Trace          string          `json:"logging.googleapis.com/trace,omitempty"`
	SpanID         string          `json:"logging.googleapis.com/spanId,omitempty"`
	TraceSampled   *bool           `json:"logging.googleapis.com/trace_sampled,omitempty"`
	HTTPRequest    *HTTPRequest    `json:"httpRequest,omitempty"`
	SourceLocation *SourceLocation `json:"logging.googleapis.com/sourceLocation,omitempty"`

	Time   time.Time         `json:"timestamp,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`

	CorrelationID string `json:"correlationId,omitempty"`

	// Any fields you want to output as `jsonPayload` are stored in this map.
	Payload map[string]interface{} `json:"-"`
}

func (e *LogEntry) Clear() {
	e.Message = ""
	e.Severity = ""
	e.Trace = ""
	e.SpanID = ""
	e.TraceSampled = nil
	e.HTTPRequest = nil
	e.SourceLocation = nil
	e.Time = time.Time{}
	e.CorrelationID = ""

	if e.Labels != nil {
		clear(e.Labels)
	}

	if e.Payload != nil {
		clear(e.Payload)
	}
}

// applyKVs applies key-value pairs to a log entry, handling special keys.
func (e *LogEntry) applyKVs(kvs ...interface{}) {
	n := len(kvs)
	if n%2 != 0 {
		// confirm whether last key is string or not
		if key, ok := kvs[n-1].(string); ok {
			e.Payload[key] = "KEY_WITHOUT_VALUE"
		}

		e.Payload["logging_error"] = "odd number of arguments received"

		n--
	}

	for i := 0; i < n; i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			// For simplicity in this helper, we skip non-string keys.
			// The With method will panic on them, ensuring safety.
			continue
		}

		switch key {
		case "error":
			if err, ok := kvs[i+1].(error); ok {
				e.Payload[key] = err.Error()
			} else {
				e.Payload[key] = kvs[i+1]
			}
		case "httpRequest":
			if req, ok := kvs[i+1].(*HTTPRequest); ok {
				e.HTTPRequest = req
			} else {
				e.Payload[key] = kvs[i+1]
			}
		case "sourceLocation":
			if sl, ok := kvs[i+1].(*SourceLocation); ok {
				e.SourceLocation = sl
			} else {
				e.Payload[key] = kvs[i+1]
			}
		default:
			e.Payload[key] = kvs[i+1]
		}
	}
}

// --- Logger ---

// Logger is a structured logger that provides leveled logging.
// Instances of Logger are safe for concurrent use.
type Logger struct {
	out                io.Writer
	trace              string
	spanId             string
	traceSampled       *bool
	labels             map[string]string
	logLevel           logLevelValue
	prefix             string
	correlationID      string
	projectID          string
	sourceLocationMode sourceLocationMode

	payload map[string]interface{}

	traceContextKey interface{}

	formatter Formatter

	// for hooks
	hookBufferSize int
	hooks          []Hook
	hooksByLevel   map[LogLevel][]Hook
	hookChan       chan *LogEntry
	hookWg         sync.WaitGroup

	outMutex sync.Mutex
}

// New creates a new Logger with default settings.
// The default log level is LevelInfo and the default output is os.Stderr.
func New(opts ...Option) *Logger {
	logger := &Logger{
		out:                os.Stderr,
		trace:              "",
		spanId:             "",
		traceSampled:       nil,
		logLevel:           logLevelValueInfo,
		prefix:             "",
		correlationID:      "",
		projectID:          "",
		labels:             make(map[string]string),
		payload:            make(map[string]interface{}),
		traceContextKey:    nil,
		sourceLocationMode: SourceLocationModeNever,
		formatter:          NewJSONFormatter(),
		hookBufferSize:     100,
	}

	for _, opt := range opts {
		opt(logger)
	}

	if len(logger.hooks) > 0 {
		logger.hooksByLevel = make(map[LogLevel][]Hook)

		for _, hook := range logger.hooks {
			levels := hook.Levels()

			if len(levels) == 0 {
				// If hook.Levels() is empty, it should fire for all levels.
				for level := range levelMap {
					if level == LogLevelOff {
						continue
					}

					logger.hooksByLevel[level] = append(logger.hooksByLevel[level], hook)
				}
			} else {
				for _, level := range levels {
					if level == LogLevelOff {
						continue
					}

					logger.hooksByLevel[level] = append(logger.hooksByLevel[level], hook)
				}
			}
		}

		logger.hookChan = make(chan *LogEntry, logger.hookBufferSize)
		logger.hookWg.Add(1)

		go logger.runHookWorker()
	}

	return logger
}

// Close gracefully shuts down the logger's background processes, such as the hook worker.
// It ensures that all buffered log entries for hooks are processed before returning.
// It's recommended to call this via defer when the application is shutting down.
func (l *Logger) Close() error {
	// If the hook worker is running, close the channel and wait for it to finish.
	if l.hookChan != nil {
		close(l.hookChan)

		l.hookWg.Wait()
	}

	return nil
}

// runHookWorker is the background goroutine that processes log entries for hooks.
func (l *Logger) runHookWorker() {
	defer l.hookWg.Done()

	for entry := range l.hookChan {
		if entry != nil {
			l.fireHooks(entry)
		}
	}
}

// fireHooks iterates over registered hooks and calls their Fire method if the level matches.
func (l *Logger) fireHooks(entry *LogEntry) {
	hooksForLevel, ok := l.hooksByLevel[LogLevel(entry.Severity)]
	if !ok {
		return
	}

	for _, hook := range hooksForLevel {
		entryCopy := l.defensiveCopy(entry)

		func() {
			defer func() {
				if r := recover(); r != nil {
					e := &LogEntry{
						Severity: LogLevelError,
						Time:     time.Now(),
						Message:  "A hook panicked",
						Payload:  map[string]any{"panic": r},
					}

					if e.SourceLocation == nil && (l.sourceLocationMode == SourceLocationModeAlways ||
						(l.sourceLocationMode == SourceLocationModeErrorOrAbove && l.logLevel <= logLevelValueError)) {
						e.SourceLocation = l.findCaller()
					}

					l.print(e)
				}
			}()

			_ = hook.Fire(entryCopy)
		}()
	}
}

// defensiveCopy creates a safe copy of a log entry for use in hooks.
func (l *Logger) defensiveCopy(entry *LogEntry) *LogEntry {
	entryCopy := *entry

	if entry.Payload != nil {
		payload := make(map[string]interface{}, len(entry.Payload))

		for k, v := range entry.Payload {
			payload[k] = v
		}

		entryCopy.Payload = payload
	}

	return &entryCopy
}

// Clone creates a new copy of the logger.
func (l *Logger) Clone() *Logger {
	newLogger := &Logger{
		out:                l.out,
		trace:              l.trace,
		spanId:             l.spanId,
		traceSampled:       l.traceSampled,
		logLevel:           l.logLevel,
		prefix:             l.prefix,
		correlationID:      l.correlationID,
		projectID:          l.projectID,
		labels:             make(map[string]string),
		payload:            make(map[string]interface{}),
		traceContextKey:    l.traceContextKey,
		sourceLocationMode: l.sourceLocationMode,
		formatter:          l.formatter,
		hooks:              l.hooks,
		hooksByLevel:       make(map[LogLevel][]Hook),
		hookChan:           l.hookChan,
	}

	for k, v := range l.labels {
		newLogger.labels[k] = v
	}

	for k, v := range l.payload {
		newLogger.payload[k] = v
	}

	for k, v := range l.hooksByLevel {
		newLogger.hooksByLevel[k] = v
	}

	return newLogger
}

// DebugfCtx logs a formatted message at the Debug level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) DebugfCtx(ctx context.Context, format string, v ...interface{}) {
	if !l.IsDebugEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelDebug, fmt.Sprintf(format, v...))
}

// InfofCtx logs a formatted message at the Info level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) InfofCtx(ctx context.Context, format string, v ...interface{}) {
	if !l.IsInfoEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelInfo, fmt.Sprintf(format, v...))
}

// WarnfCtx logs a formatted message at the Warn level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) WarnfCtx(ctx context.Context, format string, v ...interface{}) {
	if !l.IsWarnEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelWarn, fmt.Sprintf(format, v...))
}

// ErrorfCtx logs a formatted message at the Error level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) ErrorfCtx(ctx context.Context, format string, v ...interface{}) {
	if !l.IsErrorEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelError, fmt.Sprintf(format, v...))
}

// CriticalfCtx logs a formatted message at the Critical level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) CriticalfCtx(ctx context.Context, format string, v ...interface{}) {
	if !l.IsCriticalEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelCritical, fmt.Sprintf(format, v...))
}

// PrintfCtx logs a formatted message at the Info level, like log.Printf.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) PrintfCtx(ctx context.Context, format string, v ...interface{}) {
	l.InfofCtx(ctx, format, v...)
}

// PrintCtx logs its arguments at the Info level, like log.Print.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) PrintCtx(ctx context.Context, v ...interface{}) {
	if !l.IsInfoEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelInfo, sprintMessage(v...))
}

// PrintlnCtx logs its arguments at the Info level, like log.Println.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) PrintlnCtx(ctx context.Context, v ...interface{}) {
	if !l.IsInfoEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelInfo, sprintlnMessage(v...))
}

// FatalCtxf logs a formatted message at the Critical level and then calls os.Exit(1).
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) FatalfCtx(ctx context.Context, format string, v ...interface{}) {
	if l.IsCriticalEnabled() {
		l.dispatch(ctx, LogLevelCritical, fmt.Sprintf(format, v...))
	}

	// FatalfCtx functions always call os.Exit.
	osExit(1)
}

// FatalCtx logs its arguments at the Critical level and then calls os.Exit(1).
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) FatalCtx(ctx context.Context, v ...interface{}) {
	if l.IsCriticalEnabled() {
		l.dispatch(ctx, LogLevelCritical, sprintMessage(v...))
	}

	// FatalCtx functions always call os.Exit.
	osExit(1)
}

// FatallnCtx logs its arguments at the Critical level and then calls os.Exit(1).
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) FatallnCtx(ctx context.Context, v ...interface{}) {
	if l.IsCriticalEnabled() {
		l.dispatch(ctx, LogLevelCritical, sprintlnMessage(v...))
	}

	// FatallnCtx functions always call os.Exit.
	osExit(1)
}

// DebugwCtx logs a formatted message at the Debug level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) DebugwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	if !l.IsDebugEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelDebug, msg, kvs...)
}

// InfowCtx logs a formatted message at the Info level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) InfowCtx(ctx context.Context, msg string, kvs ...interface{}) {
	if !l.IsInfoEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelInfo, msg, kvs...)
}

// WarnwCtx logs a formatted message at the Warn level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) WarnwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	if !l.IsWarnEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelWarn, msg, kvs...)
}

// ErrorwCtx logs a formatted message at the Error level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) ErrorwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	if !l.IsErrorEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelError, msg, kvs...)
}

// CriticalwCtx logs a formatted message at the Critical level.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) CriticalwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	if !l.IsCriticalEnabled() {
		return
	}

	l.dispatch(ctx, LogLevelCritical, msg, kvs...)
}

// FatalwCtx logs a message with structured key-value pairs at the Critical level
// and then calls os.Exit(1).
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func (l *Logger) FatalwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	if l.IsCriticalEnabled() {
		l.dispatch(ctx, LogLevelCritical, msg, kvs...)
	}

	// FatalwCtx functions always call os.Exit.
	osExit(1)
}

// Debugf logs a formatted message at the Debug level.
func (l *Logger) Debugf(format string, v ...interface{}) {
	l.DebugfCtx(context.Background(), format, v...)
}

// Infof logs a formatted message at the Info level.
func (l *Logger) Infof(format string, v ...interface{}) {
	l.InfofCtx(context.Background(), format, v...)
}

// Warnf logs a formatted message at the Warn level.
func (l *Logger) Warnf(format string, v ...interface{}) {
	l.WarnfCtx(context.Background(), format, v...)
}

// Errorf logs a formatted message at the Error level.
func (l *Logger) Errorf(format string, v ...interface{}) {
	l.ErrorfCtx(context.Background(), format, v...)
}

// Criticalf logs a formatted message at the Critical level.
func (l *Logger) Criticalf(format string, v ...interface{}) {
	l.CriticalfCtx(context.Background(), format, v...)
}

// Printf logs a formatted message at the Info level, like log.Printf.
func (l *Logger) Printf(format string, v ...interface{}) {
	l.PrintfCtx(context.Background(), format, v...)
}

// Print logs its arguments at the Info level, like log.Print.
func (l *Logger) Print(v ...interface{}) {
	l.PrintCtx(context.Background(), v...)
}

// Println logs its arguments at the Info level, like log.Println.
func (l *Logger) Println(v ...interface{}) {
	l.PrintlnCtx(context.Background(), v...)
}

// Fatalf logs a formatted message at the Critical level and then calls os.Exit(1).
func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.FatalfCtx(context.Background(), format, v...)
}

// Fatal logs its arguments at the Critical level and then calls os.Exit(1).
func (l *Logger) Fatal(v ...interface{}) {
	l.FatalCtx(context.Background(), v...)
}

// Fatalln logs its arguments at the Critical level and then calls os.Exit(1).
func (l *Logger) Fatalln(v ...interface{}) {
	l.FatallnCtx(context.Background(), v...)
}

// Debugw logs a message at the Debug level with structured key-value pairs.
func (l *Logger) Debugw(msg string, kvs ...interface{}) {
	l.DebugwCtx(context.Background(), msg, kvs...)
}

// Infow logs a message at the Info level with structured key-value pairs.
func (l *Logger) Infow(msg string, kvs ...interface{}) {
	l.InfowCtx(context.Background(), msg, kvs...)
}

// Warnw logs a message at the Warn level with structured key-value pairs.
func (l *Logger) Warnw(msg string, kvs ...interface{}) {
	l.WarnwCtx(context.Background(), msg, kvs...)
}

// Errorw logs a message at the Error level with structured key-value pairs.
func (l *Logger) Errorw(msg string, kvs ...interface{}) {
	l.ErrorwCtx(context.Background(), msg, kvs...)
}

// Criticalw logs a message at the Critical level with structured key-value pairs.
func (l *Logger) Criticalw(msg string, kvs ...interface{}) {
	l.CriticalwCtx(context.Background(), msg, kvs...)
}

// Fatalw logs a message with structured key-value pairs at the Critical level
// and then calls os.Exit(1).
func (l *Logger) Fatalw(msg string, kvs ...interface{}) {
	l.FatalwCtx(context.Background(), msg, kvs...)
}

// dispatch is the single, central method that handles all log entry creation and printing.
// It is called *after* a level check has been performed by a public method.
func (l *Logger) dispatch(ctx context.Context, level LogLevel, msg string, kvs ...interface{}) {
	e := l.createEntry(ctx, level, msg, kvs...)

	defer func() {
		e.Clear()

		logEntryPool.Put(e)
	}()

	if e.SourceLocation == nil && (l.sourceLocationMode == SourceLocationModeAlways ||
		(l.sourceLocationMode == SourceLocationModeErrorOrAbove && levelMap[level] <= logLevelValueError)) {
		e.SourceLocation = l.findCaller()
	}

	if l.hookChan != nil {
		// Use a non-blocking send to prevent the application from stalling
		// if the hook channel buffer is full.
		hookEntry := l.defensiveCopy(e)

		select {
		case l.hookChan <- hookEntry:
		default:
			// The entry is dropped if the channel is full.
			// This is a trade-off to prioritize application performance over hook reliability under extreme load.
		}
	}

	l.print(e)
}

// createEntry is the single, central helper for creating log entries.
// It accepts a context (which can be nil) and correctly applies values with the
// precedence: method args > logger context > context.Context.
func (l *Logger) createEntry(ctx context.Context, level LogLevel, msg string, kvs ...interface{}) *LogEntry {
	// 1. Create the base entry.
	e := logEntryPool.Get().(*LogEntry)

	e.Severity = level
	e.Message = l.prefix + msg
	e.Trace = l.trace
	e.SpanID = l.spanId
	e.TraceSampled = l.traceSampled
	e.CorrelationID = l.correlationID
	e.Labels = l.labels
	e.Time = time.Now()

	// 2. Apply values from context.Context (lowest precedence).
	if ctx != nil && l.projectID != "" && l.traceContextKey != nil {
		if traceHeader, ok := ctx.Value(l.traceContextKey).(string); ok {
			parts := strings.Split(traceHeader, "/")

			if len(parts) > 0 && e.Trace == "" {
				e.Trace = fmt.Sprintf("projects/%s/traces/%s", l.projectID, parts[0])
			}

			if len(parts) > 1 && e.SpanID == "" {
				spanParts := strings.Split(parts[1], ";")
				e.SpanID = spanParts[0]
			}
		}
	}

	// 3. Apply contextual fields from the logger (With method).
	if len(l.payload) > 0 {
		contextKVs := make([]interface{}, 0, len(l.payload)*2)

		for k, v := range l.payload {
			contextKVs = append(contextKVs, k, v)
		}

		e.applyKVs(contextKVs...)
	}

	// 4. Apply key-value pairs from the specific log call (highest precedence).
	if len(kvs) > 0 {
		e.applyKVs(kvs...)
	}

	return e
}

// print writes the log entry to the logger's output.
func (l *Logger) print(e *LogEntry) {
	l.outMutex.Lock()
	defer l.outMutex.Unlock()

	out, err := l.formatter.Format(e)
	if err != nil {
		log.Printf("failed to format log entry: %v", err)

		return
	}

	fmt.Fprintln(l.out, string(out))
}

func (l *Logger) findCaller() *SourceLocation {
	pcs := make([]uintptr, 16)

	// 0: Callers, 1: findCaller. Start search from the caller of findCaller.
	n := runtime.Callers(2, pcs)

	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()

		// Skip frames that are inside the harelog package.
		// if !strings.Contains(frame.File, "harelog") {
		if !strings.HasPrefix(frame.Function, harelogPackage) {
			return &SourceLocation{
				File:     frame.File,
				Line:     frame.Line,
				Function: frame.Function,
			}
		}

		if !more {
			break
		}
	}

	return nil
}

// IsDebugEnabled checks if the Debug level is enabled for the logger.
func (l *Logger) IsDebugEnabled() bool {
	return isDebugEnabled(l.logLevel)
}

// IsInfoEnabled checks if the Info level is enabled for the logger.
func (l *Logger) IsInfoEnabled() bool {
	return isInfoEnabled(l.logLevel)
}

// IsWarnEnabled checks if the Warn level is enabled for the logger.
func (l *Logger) IsWarnEnabled() bool {
	return isWarnEnabled(l.logLevel)
}

// IsErrorEnabled checks if the Error level is enabled for the logger.
func (l *Logger) IsErrorEnabled() bool {
	return isErrorEnabled(l.logLevel)
}

// IsCriticalEnabled checks if the Critical level is enabled for the logger.
func (l *Logger) IsCriticalEnabled() bool {
	return isCriticalEnabled(l.logLevel)
}

// WithLogLevel returns a new logger instance with the specified log level.
func (l *Logger) WithLogLevel(level LogLevel) *Logger {
	if _, ok := levelMap[level]; !ok {
		panic(fmt.Sprintf("harelog: invalid log level provided to (*Logger).WithLogLevel: %q", level))
	}

	newLogger := l.Clone()
	newLogger.logLevel = levelMap[level]

	return newLogger
}

// WithOutput returns a new logger instance that writes to the provided io.Writer.
func (l *Logger) WithOutput(w io.Writer) *Logger {
	newLogger := l.Clone()

	if w != nil {
		newLogger.out = w
	}

	return newLogger
}

// WithFormatter returns a new logger instance with the specified formatter.
func (l *Logger) WithFormatter(f Formatter) *Logger {
	newLogger := l.Clone()

	if f != nil {
		newLogger.formatter = f
	}

	return newLogger
}

// WithAutoSource returns a new logger with a different source location mode.
func (l *Logger) WithAutoSource(mode sourceLocationMode) *Logger {
	if mode < SourceLocationModeNever || mode > SourceLocationModeErrorOrAbove {
		panic(fmt.Sprintf("harelog: invalid SourceLocationMode provided: %d", mode))
	}

	newLogger := l.Clone()

	newLogger.sourceLocationMode = mode

	return newLogger
}

// WithProjectID returns a new logger with a different Project ID.
func (l *Logger) WithProjectID(projectID string) *Logger {
	newLogger := l.Clone()
	newLogger.projectID = projectID

	return newLogger
}

// WithTraceContextKey returns a new logger with a different trace context key.
func (l *Logger) WithTraceContextKey(key interface{}) *Logger {
	if key == nil {
		panic("harelog: nil key provided to WithTraceContextKey; context keys must be non-nil")
	}

	newLogger := l.Clone()
	newLogger.traceContextKey = key

	return newLogger
}

// WithPrefix returns a new logger instance with the specified message prefix.
func (l *Logger) WithPrefix(prefix string) *Logger {
	newLogger := l.Clone()
	newLogger.prefix = prefix

	return newLogger
}

// WithLabels returns a new logger instance with the provided labels added.
func (l *Logger) WithLabels(labels map[string]string) *Logger {
	newLogger := l.Clone()

	for k, v := range labels {
		newLogger.labels[k] = v
	}

	return newLogger
}

// WithoutLabels returns a new logger instance with the provided labels removed.
func (l *Logger) WithoutLabels(keys ...string) *Logger {
	newLogger := l.Clone()

	for _, key := range keys {
		delete(newLogger.labels, key)
	}

	return newLogger
}

// With returns a new logger instance with the provided key-value pairs added to its context.
// It panics if the number of arguments is odd or if a key is not a string.
func (l *Logger) With(kvs ...interface{}) *Logger {
	n := len(kvs)

	if n%2 != 0 {
		panic("log.With: odd number of arguments received")
	}

	newLogger := l.Clone()

	for i := 0; i < n; i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			panic(fmt.Sprintf("log.With: non-string key at argument position %d", i))
		}

		newLogger.payload[key] = kvs[i+1]
	}

	return newLogger
}

// WithTrace returns a new logger instance with the specified GCP trace identifier.
func (l *Logger) WithTrace(trace string) *Logger {
	newLogger := l.Clone()
	newLogger.trace = trace

	return newLogger
}

// WithSpanId returns a new logger instance with the specified GCP spanId identifier.
func (l *Logger) WithSpanId(spanId string) *Logger {
	newLogger := l.Clone()
	newLogger.spanId = spanId

	return newLogger
}

// WithTraceSampled returns a new logger instance with the specified GCP traceSampled identifier.
func (l *Logger) WithTraceSampled(traceSampled *bool) *Logger {
	newLogger := l.Clone()
	newLogger.traceSampled = traceSampled

	return newLogger
}

// WithCorrelationID returns a new logger instance with the specified correlation ID.
func (l *Logger) WithCorrelationID(correlationID string) *Logger {
	newLogger := l.Clone()
	newLogger.correlationID = correlationID

	return newLogger
}

// Clone creates a new copy of the default logger.
func Clone() *Logger {
	return std.Clone()
}

// SetDefaultLogLevel sets the log level for the default logger.
// The provided level should be validated with ParseLogLevel first.
func SetDefaultLogLevel(level LogLevel) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	if _, ok := levelMap[level]; !ok {
		panic(fmt.Sprintf("harelog: invalid log level provided to SetDefaultLogLevel: %q", level))
	}

	std = std.WithLogLevel(level)
}

// SetDefaultOutput sets the output destination for the default logger.
func SetDefaultOutput(w io.Writer) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithOutput(w)
}

// SetDefaultFormatter sets the formatter for the default logger.
func SetDefaultFormatter(f Formatter) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithFormatter(f)
}

// SetDefaultAutoSource sets the automatic source location capturing mode.
func SetDefaultAutoSource(mode sourceLocationMode) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithAutoSource(mode)
}

// SetDefaultHooks sets hooks for the default logger.
// This function is safe for concurrent use.
// It replaces the existing default logger with a new one containing the specified hooks.
func SetDefaultHooks(hooks ...Hook) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	// Gracefully close the old logger's worker if it exists.
	if std.hookChan != nil {
		_ = std.Close()
	}

	// --- Preserve existing settings ---
	// Find the current LogLevel string from the internal logLevelValue.
	var currentLevel LogLevel = LogLevelInfo // Default fallback
	for l, v := range levelMap {
		if v == std.logLevel {
			currentLevel = l
			break
		}
	}

	// Convert payload map to a slice for WithFields.
	payloadKVs := make([]interface{}, 0, len(std.payload)*2)

	for k, v := range std.payload {
		payloadKVs = append(payloadKVs, k, v)
	}

	opts := []Option{
		WithOutput(std.out),
		WithLogLevel(currentLevel),
		WithFormatter(std.formatter),
		WithAutoSource(std.sourceLocationMode),
		WithProjectID(std.projectID),
		WithPrefix(std.prefix),
		WithLabels(std.labels),
		WithFields(payloadKVs...),
		WithHookBufferSize(std.hookBufferSize),
		WithHooks(hooks...),
	}

	// WithTraceContextKey panics on nil, so only add it if it exists.
	if std.traceContextKey != nil {
		opts = append(opts, WithTraceContextKey(std.traceContextKey))
	}
	// --- End of preserving settings ---

	// Create a new logger with the new hooks, preserving all other settings.
	std = New(opts...)
}

// WithProjectID sets the initial Google Cloud Project ID.
func SetDefaultProjectID(projectID string) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithProjectID(projectID)
}

// WithTraceContextKey sets the initial context key for tracing.
func SetDefaultTraceContextKey(key interface{}) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithTraceContextKey(key)
}

// SetDefaultPrefix sets the message prefix for the default logger.
func SetDefaultPrefix(prefix string) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithPrefix(prefix)
}

// SetDefaultLabels sets labels for the default logger.
// These labels will be included in all logs from the default logger.
func SetDefaultLabels(labels map[string]string) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithLabels(labels)
}

// RemoveDefaultLabels removes labels from the default logger.
func RemoveDefaultLabels(keys ...string) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithoutLabels(keys...)
}

// IsDebugEnabled checks if the Debug level is enabled for the default logger.
func IsDebugEnabled() bool {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	return std.IsDebugEnabled()
}

// IsInfoEnabled checks if the Info level is enabled for the default logger.
func IsInfoEnabled() bool {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	return std.IsInfoEnabled()
}

// IsWarnEnabled checks if the Warn level is enabled for the default logger.
func IsWarnEnabled() bool {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	return std.IsWarnEnabled()
}

// IsErrorEnabled checks if the Error level is enabled for the default logger.
func IsErrorEnabled() bool {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	return std.IsErrorEnabled()
}

// IsCriticalEnabled checks if the Critical level is enabled for the default logger.
func IsCriticalEnabled() bool {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	return std.IsCriticalEnabled()
}

// DebugfCtx logs a formatted message at the Debug level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func DebugfCtx(ctx context.Context, format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.DebugfCtx(ctx, format, v...)
}

// InfofCtx logs a formatted message at the Info level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func InfofCtx(ctx context.Context, format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.InfofCtx(ctx, format, v...)
}

// WarnfCtx logs a formatted message at the Warn level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func WarnfCtx(ctx context.Context, format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.WarnfCtx(ctx, format, v...)
}

// ErrorfCtx logs a formatted message at the Error level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func ErrorfCtx(ctx context.Context, format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.ErrorfCtx(ctx, format, v...)
}

// CriticalfCtx logs a formatted message at the Critical level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func CriticalfCtx(ctx context.Context, format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.CriticalfCtx(ctx, format, v...)
}

// PrintfCtx logs a formatted message at the Info level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func PrintfCtx(ctx context.Context, format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.PrintfCtx(ctx, format, v...)
}

// PrintCtx logs its arguments at the Info level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func PrintCtx(ctx context.Context, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.PrintCtx(ctx, v...)
}

// PrintlnCtx logs its arguments at the Info level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func PrintlnCtx(ctx context.Context, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.PrintlnCtx(ctx, v...)
}

// FatalfCtx logs a formatted message at the Critical level and then calls os.Exit(1).
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func FatalfCtx(ctx context.Context, format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.FatalfCtx(ctx, format, v...)
}

// FatalCtx logs its arguments at the Critical level and then calls os.Exit(1).
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func FatalCtx(ctx context.Context, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.FatalCtx(ctx, v...)
}

// FatallnCtx logs its arguments at the Critical level and then calls os.Exit(1).
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func FatallnCtx(ctx context.Context, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.FatallnCtx(ctx, v...)
}

// DebugwCtx logs a message at the Debug level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func DebugwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.DebugwCtx(ctx, msg, kvs...)
}

// InfowCtx logs a message at the Info level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func InfowCtx(ctx context.Context, msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.InfowCtx(ctx, msg, kvs...)
}

// WarnwCtx logs a message at the Warn level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func WarnwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.WarnwCtx(ctx, msg, kvs...)
}

// ErrorwCtx logs a message at the Error level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func ErrorwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.ErrorwCtx(ctx, msg, kvs...)
}

// CriticalwCtx logs a message at the Critical level using the default logger.
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func CriticalwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.CriticalwCtx(ctx, msg, kvs...)
}

// FatalwCtx logs a message with structured key-value pairs at the Critical level
// using the default logger and then calls os.Exit(1).
// It extracts values from the provided context, such as Google Cloud Trace identifiers,
// and includes them in the log entry.
func FatalwCtx(ctx context.Context, msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.FatalwCtx(ctx, msg, kvs...)
}

// Debugf logs a formatted message at the Debug level using the default logger.
func Debugf(format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Debugf(format, v...)
}

// Infof logs a formatted message at the Info level using the default logger.
func Infof(format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Infof(format, v...)
}

// Warnf logs a formatted message at the Warn level using the default logger.
func Warnf(format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Warnf(format, v...)
}

// Errorf logs a formatted message at the Error level using the default logger.
func Errorf(format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Errorf(format, v...)
}

// Criticalf logs a formatted message at the Critical level using the default logger.
func Criticalf(format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Criticalf(format, v...)
}

// Printf logs a formatted message at the Info level using the default logger.
func Printf(format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Printf(format, v...)
}

// Print logs its arguments at the Info level using the default logger.
func Print(v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Print(v...)
}

// Println logs its arguments at the Info level using the default logger.
func Println(v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Println(v...)
}

// Fatalf logs a formatted message at the Critical level and then calls os.Exit(1).
func Fatalf(format string, v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Fatalf(format, v...)
}

// Fatal logs its arguments at the Critical level and then calls os.Exit(1).
func Fatal(v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Fatal(v...)
}

// Fatalln logs its arguments at the Critical level and then calls os.Exit(1).
func Fatalln(v ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Fatalln(v...)
}

// Debugw logs a message at the Debug level using the default logger.
func Debugw(msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Debugw(msg, kvs...)
}

// Infow logs a message at the Info level using the default logger.
func Infow(msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Infow(msg, kvs...)
}

// Warnw logs a message at the Warn level using the default logger.
func Warnw(msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Warnw(msg, kvs...)
}

// Errorw logs a message at the Error level using the default logger.
func Errorw(msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Errorw(msg, kvs...)
}

// Criticalw logs a message at the Critical level using the default logger.
func Criticalw(msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Criticalw(msg, kvs...)
}

// Fatalw logs a message with structured key-value pairs at the Critical level
// using the default logger and then calls os.Exit(1).
func Fatalw(msg string, kvs ...interface{}) {
	stdMutex.RLock()
	defer stdMutex.RUnlock()

	std.Fatalw(msg, kvs...)
}

// Close gracefully shuts down the default logger's background processes,
// such as the hook worker. It's recommended to call this via defer in the main function
// to ensure all buffered logs for hooks are processed before the application exits.
func Close() error {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	return std.Close()
}

// isDebugEnabled returns
func isDebugEnabled(level logLevelValue) bool {
	return level >= logLevelValueDebug
}

// isInfoEnabled returns
func isInfoEnabled(level logLevelValue) bool {
	return level >= logLevelValueInfo
}

// isWarnEnabled returns
func isWarnEnabled(level logLevelValue) bool {
	return level >= logLevelValueWarn
}

// isErrorEnabled returns
func isErrorEnabled(level logLevelValue) bool {
	return level >= logLevelValueError
}

// isCriticalEnabled returns
func isCriticalEnabled(level logLevelValue) bool {
	return level >= logLevelValueCritical
}

// sprintMessage builds a string from a slice of interfaces, separated by spaces.
func sprintMessage(v ...interface{}) string {
	var b strings.Builder

	for i, arg := range v {
		if i > 0 {
			b.WriteString(" ")
		}
		fmt.Fprint(&b, arg)
	}

	return b.String()
}

// sprintlnMessage builds a string from a slice of interfaces, separated by spaces, with a final newline.
func sprintlnMessage(v ...interface{}) string {
	return sprintMessage(v...) + "\n"
}

// Option configures a Logger.
type Option func(*Logger)

// WithLogLevel is a functional option that sets the initial log level for the logger.
func WithLogLevel(level LogLevel) Option {
	return func(l *Logger) {
		lv, ok := levelMap[level]
		if !ok {
			panic(fmt.Sprintf("harelog: invalid log level provided to WithLogLevel: %q", level))
		}

		l.logLevel = lv
	}
}

// WithOutput sets the writer for the logger.
func WithOutput(w io.Writer) Option {
	return func(l *Logger) {
		if w != nil {
			l.out = w
		}
	}
}

// WithFormatter sets the formatter for the logger.
func WithFormatter(f Formatter) Option {
	return func(l *Logger) {
		if f != nil {
			l.formatter = f
		}
	}
}

// WithAutoSource is a functional option that configures the logger's behavior for
// automatically capturing the source code location (file, line, function name).
// Note: Enabling this feature, especially with SourceLocationModeAlways, has a
// non-trivial performance cost due to the use of runtime.Callers.
func WithAutoSource(mode sourceLocationMode) Option {
	// This is the "Fail Fast" check.
	if mode < SourceLocationModeNever || mode > SourceLocationModeErrorOrAbove {
		panic(fmt.Sprintf("harelog: invalid SourceLocationMode provided: %d", mode))
	}

	return func(l *Logger) {
		l.sourceLocationMode = mode
	}
}

// WithProjectID sets the Google Cloud Project ID to be used for formatting trace identifiers.
func WithProjectID(id string) Option {
	return func(l *Logger) {
		l.projectID = id
	}
}

// WithTraceContextKey sets the key used to extract Google Cloud Trace data from a context.Context.
func WithTraceContextKey(key interface{}) Option {
	if key == nil {
		panic("harelog: nil key provided to WithTraceContextKey; context keys must be non-nil")
	}

	return func(l *Logger) {
		l.traceContextKey = key
	}
}

// WithPrefix sets the initial message prefix.
func WithPrefix(prefix string) Option {
	return func(l *Logger) {
		l.prefix = prefix
	}
}

// WithLabels sets the initial set of labels.
func WithLabels(labels map[string]string) Option {
	return func(l *Logger) {
		for k, v := range labels {
			l.labels[k] = v
		}
	}
}

// WithFields sets the initial set of contextual key-value fields (payload).
func WithFields(kvs ...interface{}) Option {
	n := len(kvs)

	if n%2 != 0 {
		panic("log.With: odd number of arguments received")
	}

	return func(l *Logger) {
		for i := 0; i < n; i += 2 {
			key, ok := kvs[i].(string)
			if !ok {
				panic(fmt.Sprintf("log.With: non-string key at argument position %d", i))
			}

			l.payload[key] = kvs[i+1]
		}

	}
}

// WithHookBufferSize sets the buffer size for the hook channel.
// The default is 100. A larger buffer can handle higher log volumes without
// dropping hook events, but consumes more memory.
func WithHookBufferSize(size int) Option {
	return func(l *Logger) {
		if size > 0 {
			l.hookBufferSize = size
		}
	}
}

// WithHooks is a functional option that registers hooks with the logger.
// Hooks are triggered asynchronously when a log entry is created at a level
// specified in the hook's Levels() method.
func WithHooks(hooks ...Hook) Option {
	return func(l *Logger) {
		l.hooks = make([]Hook, 0, len(hooks))

		l.hooks = append(l.hooks, hooks...)
	}
}
