// Package harelog provides a structured, level-based logging solution.
// It is designed to be flexible, thread-safe, and particularly well-suited for
// use with Google Cloud Logging by supporting its special JSON fields.
package harelog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel defines the severity level of a log entry.
type logLevel string

const (
	LogLevelOff      logLevel = "OFF"
	LogLevelCritical logLevel = "CRITICAL"
	LogLevelError    logLevel = "ERROR"
	LogLevelWarn     logLevel = "WARN"
	LogLevelInfo     logLevel = "INFO"
	LogLevelDebug    logLevel = "DEBUG"
	LogLevelAll      logLevel = "ALL"
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

var (
	std      = New()
	stdMutex = &sync.RWMutex{}

	osExit = os.Exit
)

var levelMap = map[logLevel]logLevelValue{
	LogLevelOff:      logLevelValueOff,
	LogLevelCritical: logLevelValueCritical,
	LogLevelError:    logLevelValueError,
	LogLevelWarn:     logLevelValueWarn,
	LogLevelInfo:     logLevelValueInfo,
	LogLevelDebug:    logLevelValueDebug,
	LogLevelAll:      logLevelValueAll,
}

// ParseLogLevel parses a string into a LogLevel.
// It is case-insensitive. It returns an error if the input string is not a valid log level.
func ParseLogLevel(levelStr string) (logLevel, error) {
	level := logLevel(strings.ToUpper(levelStr))
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

type jsonTime struct {
	time.Time
}

func (t jsonTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.In(time.UTC).Format(time.RFC3339Nano) + `"`), nil
}

// --- Log Entry Structure ---

// logEntry is the internal data container for a single log entry.
type logEntry struct {
	Message        string          `json:"message"`
	Severity       string          `json:"severity,omitempty"`
	Trace          string          `json:"logging.googleapis.com/trace,omitempty"`
	SpanID         string          `json:"logging.googleapis.com/spanId,omitempty"`
	TraceSampled   *bool           `json:"logging.googleapis.com/trace_sampled,omitempty"`
	HTTPRequest    *HTTPRequest    `json:"httpRequest,omitempty"`
	SourceLocation *SourceLocation `json:"logging.googleapis.com/sourceLocation,omitempty"`

	Time   jsonTime          `json:"timestamp,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`

	CorrelationID string `json:"correlationId,omitempty"`

	// Any fields you want to output as `jsonPayload` are stored in this map.
	Payload map[string]interface{} `json:"-"`
}

// --- Logger ---

// Logger is a structured logger that provides leveled logging.
// Instances of Logger are safe for concurrent use.
type Logger struct {
	out           io.Writer
	trace         string
	spanId        string
	traceSampled  *bool
	labels        map[string]string
	logLevel      logLevelValue
	prefix        string
	correlationID string
}

// New creates a new Logger with default settings.
// The default log level is LevelInfo and the default output is os.Stderr.
func New() *Logger {
	return &Logger{
		out:           os.Stderr,
		trace:         "",
		spanId:        "",
		traceSampled:  nil,
		logLevel:      logLevelValueInfo,
		prefix:        "",
		correlationID: "",
		labels:        make(map[string]string),
	}
}

// Clone creates a new copy of the default logger.
func Clone() *Logger {
	return std.Clone()
}

// Clone creates a new copy of the logger.
func (l *Logger) Clone() *Logger {
	newLogger := &Logger{
		out:           l.out,
		trace:         l.trace,
		spanId:        l.spanId,
		traceSampled:  l.traceSampled,
		logLevel:      l.logLevel,
		prefix:        l.prefix,
		correlationID: l.correlationID,
		labels:        make(map[string]string),
	}

	for k, v := range l.labels {
		newLogger.labels[k] = v
	}

	return newLogger
}

// Debugf logs a formatted message at the Debug level.
func (l *Logger) Debugf(format string, v ...interface{}) {
	if !l.IsDebugEnabled() {
		return
	}

	l.print(l.createEntryf(LogLevelDebug, format, v...))
}

// Infof logs a formatted message at the Info level.
func (l *Logger) Infof(format string, v ...interface{}) {
	if !l.IsInfoEnabled() {
		return
	}

	l.print(l.createEntryf(LogLevelInfo, format, v...))
}

// Warnf logs a formatted message at the Warn level.
func (l *Logger) Warnf(format string, v ...interface{}) {
	if !l.IsWarnEnabled() {
		return
	}

	l.print(l.createEntryf(LogLevelWarn, format, v...))
}

// Errorf logs a formatted message at the Error level.
func (l *Logger) Errorf(format string, v ...interface{}) {
	if !l.IsErrorEnabled() {
		return
	}

	l.print(l.createEntryf(LogLevelError, format, v...))
}

// Criticalf logs a formatted message at the Critical level.
func (l *Logger) Criticalf(format string, v ...interface{}) {
	if !l.IsCriticalEnabled() {
		return
	}

	l.print(l.createEntryf(LogLevelCritical, format, v...))
}

// Printf logs a formatted message at the Info level, like log.Printf.
func (l *Logger) Printf(format string, v ...interface{}) {
	l.Infof(format, v...)
}

// Print logs its arguments at the Info level, like log.Print.
func (l *Logger) Print(v ...interface{}) {
	if !l.IsInfoEnabled() {
		return
	}

	l.print(l.createEntry(LogLevelInfo, sprintMessage(v...)))
}

// Println logs its arguments at the Info level, like log.Println.
func (l *Logger) Println(v ...interface{}) {
	if !l.IsInfoEnabled() {
		return
	}

	l.print(l.createEntry(LogLevelInfo, sprintlnMessage(v...)))
}

// Fatalf logs a formatted message at the Critical level and then calls os.Exit(1).
func (l *Logger) Fatalf(format string, v ...interface{}) {
	if l.IsCriticalEnabled() {
		l.print(l.createEntryf(LogLevelCritical, format, v...))
	}

	osExit(1)
}

// Fatal logs its arguments at the Critical level and then calls os.Exit(1).
func (l *Logger) Fatal(v ...interface{}) {
	if l.IsCriticalEnabled() {
		l.print(l.createEntry(LogLevelCritical, sprintMessage(v...)))
	}

	osExit(1)
}

// Fatalln logs its arguments at the Critical level and then calls os.Exit(1).
func (l *Logger) Fatalln(v ...interface{}) {
	if l.IsCriticalEnabled() {
		l.print(l.createEntry(LogLevelCritical, sprintlnMessage(v...)))
	}

	osExit(1)
}

// Debugw logs a message at the Debug level with structured key-value pairs.
func (l *Logger) Debugw(msg string, kvs ...interface{}) {
	if !l.IsDebugEnabled() {
		return
	}

	l.print(l.createEntryw(LogLevelDebug, msg, kvs...))
}

// Infow logs a message at the Info level with structured key-value pairs.
func (l *Logger) Infow(msg string, kvs ...interface{}) {
	if !l.IsInfoEnabled() {
		return
	}

	l.print(l.createEntryw(LogLevelInfo, msg, kvs...))
}

// Warnw logs a message at the Warn level with structured key-value pairs.
func (l *Logger) Warnw(msg string, kvs ...interface{}) {
	if !l.IsWarnEnabled() {
		return
	}

	l.print(l.createEntryw(LogLevelWarn, msg, kvs...))
}

// Errorw logs a message at the Error level with structured key-value pairs.
func (l *Logger) Errorw(msg string, kvs ...interface{}) {
	if !l.IsErrorEnabled() {
		return
	}

	l.print(l.createEntryw(LogLevelError, msg, kvs...))
}

// Criticalw logs a message at the Critical level with structured key-value pairs.
func (l *Logger) Criticalw(msg string, kvs ...interface{}) {
	if !l.IsCriticalEnabled() {
		return
	}

	l.print(l.createEntryw(LogLevelCritical, msg, kvs...))
}

// createEntry creates a logEntry with a pre-formatted message.
func (l *Logger) createEntry(level logLevel, msg string) *logEntry {
	return &logEntry{
		Severity:      string(level),
		Message:       l.prefix + msg,
		Trace:         l.trace,
		SpanID:        l.spanId,
		TraceSampled:  l.traceSampled,
		CorrelationID: l.correlationID,
		Labels:        l.labels,
		Time:          jsonTime{time.Now()},
	}
}

// createEntryf creates a logEntry by formatting a message.
func (l *Logger) createEntryf(level logLevel, format string, v ...interface{}) *logEntry {
	return l.createEntry(level, fmt.Sprintf(format, v...))
}

// createEntryw creates a logEntry from the logger's context and the provided arguments.
func (l *Logger) createEntryw(severity logLevel, msg string, kvs ...interface{}) *logEntry {
	payload := make(map[string]interface{})

	logEntry := &logEntry{
		Severity:      string(severity),
		Message:       l.prefix + msg,
		Trace:         l.trace,
		SpanID:        l.spanId,
		TraceSampled:  l.traceSampled,
		CorrelationID: l.correlationID,
		Labels:        l.labels,
		Time:          jsonTime{time.Now()},
	}

	n := len(kvs)

	if n%2 != 0 {
		// confirm whether last key is string or not
		if key, ok := kvs[n-1].(string); ok {
			payload[key] = "KEY_WITHOUT_VALUE"
		}

		// add error information to log(playload)
		payload["logging_error"] = "odd number of arguments received"

		n--
	}

	// loop through a range guaranteed to be even
	for i := 0; i < n; i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			// skip if key is not string
			continue
		}

		// error
		if key == "error" {
			if err, ok := kvs[i+1].(error); ok {
				payload[key] = err.Error()
			}
			continue
		}

		// httpRequest
		if key == "httpRequest" {
			if req, ok := kvs[i+1].(*HTTPRequest); ok {
				logEntry.HTTPRequest = req
			}
			continue
		}

		// sourceLocation
		if key == "sourceLocation" {
			if sl, ok := kvs[i+1].(*SourceLocation); ok {
				logEntry.SourceLocation = sl
			}
			continue
		}

		payload[key] = kvs[i+1]
	}

	logEntry.Payload = payload

	return logEntry
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

// WithPrefix returns a new logger instance with the specified message prefix.
func (l *Logger) WithPrefix(prefix string) *Logger {
	newLogger := l.Clone()
	newLogger.prefix = prefix

	return newLogger
}

// WithLogLevel returns a new logger instance with the specified log level.
func (l *Logger) WithLogLevel(level logLevel) *Logger {
	newLogger := l.Clone()
	newLogger.logLevel = levelMap[level]

	return newLogger
}

// WithOutput returns a new logger instance that writes to the provided io.Writer.
func (l *Logger) WithOutput(w io.Writer) *Logger {
	newLogger := l.Clone()
	newLogger.out = w

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

// SetDefaultPrefix sets the message prefix for the default logger.
func SetDefaultPrefix(prefix string) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithPrefix(prefix)
}

// SetDefaultOutput sets the output destination for the default logger.
func SetDefaultOutput(w io.Writer) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithOutput(w)
}

// SetDefaultLogLevel sets the log level for the default logger.
// The provided level should be validated with ParseLogLevel first.
func SetDefaultLogLevel(level logLevel) {
	stdMutex.Lock()
	defer stdMutex.Unlock()

	std = std.WithLogLevel(level)
}

// print writes the log entry to the logger's output.
func (l *Logger) print(e *logEntry) {
	m := make(map[string]interface{})

	for k, v := range e.Payload {
		m[k] = v
	}

	m["message"] = e.Message
	m["severity"] = e.Severity

	if e.Trace != "" {
		m["logging.googleapis.com/trace"] = e.Trace
	}

	if e.SpanID != "" {
		m["logging.googleapis.com/spanId"] = e.SpanID
	}

	if e.TraceSampled != nil {
		m["logging.googleapis.com/trace_sampled"] = e.TraceSampled
	}

	if e.HTTPRequest != nil {
		m["httpRequest"] = e.HTTPRequest
	}

	if e.SourceLocation != nil {
		m["logging.googleapis.com/sourceLocation"] = e.SourceLocation
	}

	m["timestamp"] = e.Time

	if len(e.Labels) > 0 {
		m["labels"] = e.Labels
	}

	if e.CorrelationID != "" {
		m["correlationId"] = e.CorrelationID
	}

	out, err := json.Marshal(m)
	if err != nil {
		log.Printf("json.Marshal: %v", err)

		return
	}

	fmt.Fprintln(l.out, string(out))
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
	return sprintMessage(v...) + "\\n"
}
