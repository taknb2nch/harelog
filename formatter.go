package harelog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	isatty "github.com/mattn/go-isatty"
)

// levelColorMap maps log levels to their corresponding color functions.
// This is a private implementation detail of the textFormatter.
var levelColorMap = map[string]*color.Color{
	string(LogLevelError):    color.New(color.FgRed),
	string(LogLevelCritical): color.New(color.FgHiRed, color.Bold),
	string(LogLevelWarn):     color.New(color.FgYellow),
	string(LogLevelInfo):     color.New(color.FgGreen),
	string(LogLevelDebug):    color.New(color.FgCyan),
}

// Formatter is an interface for converting a logEntry into a byte slice.
type Formatter interface {
	Format(entry *logEntry) ([]byte, error)
}

// jsonFormatter formats log entries as JSON.
type jsonFormatter struct{}

// NewJSONFormatter creates a new JSONFormatter.
func NewJSONFormatter() *jsonFormatter {
	return &jsonFormatter{}
}

// Format converts a logEntry to JSON format.
func (f *jsonFormatter) Format(e *logEntry) ([]byte, error) {
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

	return json.Marshal(m)
}

// textFormatter formats log entries as human-readable text.
type textFormatter struct {
	enableColor      bool
	isEnableColorSet bool
}

// TextFormatterOption configures a textFormatter.
type TextFormatterOption func(*textFormatter)

// NewTextFormatter creates a new TextFormatter.
func NewTextFormatter(opts ...TextFormatterOption) Formatter {
	formatter := &textFormatter{
		enableColor:      false,
		isEnableColorSet: false,
	}

	for _, opt := range opts {
		opt(formatter)
	}

	return formatter
}

// WithColor is an option to enable or disable color output for the TextFormatter.
func WithColor(enabled bool) TextFormatterOption {
	return func(f *textFormatter) {
		f.enableColor = enabled
		f.isEnableColorSet = true
	}
}

// Format converts a logEntry to a single-line text format.
func (f *textFormatter) Format(e *logEntry) ([]byte, error) {
	var b bytes.Buffer

	useColor := f.enableColor

	if !f.isEnableColorSet {
		// If user hasn't specified, auto-detect based on TTY.
		// Note: This check assumes a standard output file descriptor.
		useColor = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stderr.Fd())
	}

	// Timestamp
	b.WriteString(e.Time.Format(time.RFC3339))
	b.WriteString(" ")

	levelString := fmt.Sprintf("[%s]", e.Severity)

	if c, ok := levelColorMap[e.Severity]; ok {
		// Explicitly enable or disable color on the object for this call.
		if useColor {
			c.EnableColor()
		} else {
			c.DisableColor()
		}
		b.WriteString(c.Sprint(levelString))
	} else {
		b.WriteString(levelString)
	}

	b.WriteString(" ")

	// Message
	b.WriteString(strings.TrimRight(e.Message, "\n"))

	// Aggregate all structured data into a single map
	fields := make(map[string]interface{})

	// Copy payload fields first
	for k, v := range e.Payload {
		fields[k] = v
	}

	// Add special fields if they exist and are not already in the payload
	if e.SourceLocation != nil {
		if _, ok := fields["sourceLocation"]; !ok {
			// Format source location for readability
			fields["source"] = fmt.Sprintf("%s:%d", e.SourceLocation.File, e.SourceLocation.Line)
		}
	}

	if e.Trace != "" {
		fields["trace"] = e.Trace
	}

	if e.SpanID != "" {
		fields["spanId"] = e.SpanID
	}

	if e.CorrelationID != "" {
		fields["correlationId"] = e.CorrelationID
	}

	for k, v := range e.Labels {
		fields[fmt.Sprintf("label.%s", k)] = v // Prefix to avoid key collisions
	}

	if e.HTTPRequest != nil {
		// Extract the most useful parts of the HTTP request
		if e.HTTPRequest.RequestMethod != "" {
			fields["http.method"] = e.HTTPRequest.RequestMethod
		}
		if e.HTTPRequest.Status != 0 {
			fields["http.status"] = e.HTTPRequest.Status
		}
		if e.HTTPRequest.RequestURL != "" {
			fields["http.url"] = e.HTTPRequest.RequestURL
		}
	}

	// // Add payload only if it exists.
	if len(fields) > 0 {
		b.WriteString(" {")

		keys := make([]string, 0, len(fields))

		for k := range fields {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}

			b.WriteString(k)
			b.WriteString("=")

			// Handle strings and other types differently for quoting.
			val := fields[k]

			if s, ok := val.(string); ok {
				b.WriteString(fmt.Sprintf("%q", s))
			} else {
				b.WriteString(fmt.Sprint(val))
			}
		}

		b.WriteString("}")
	}

	return b.Bytes(), nil
}
