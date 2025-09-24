package harelog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

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
type textFormatter struct{}

// NewTextFormatter creates a new TextFormatter.
func NewTextFormatter() *textFormatter {
	return &textFormatter{}
}

// Format converts a logEntry to a single-line text format.
func (f *textFormatter) Format(e *logEntry) ([]byte, error) {
	var b bytes.Buffer

	// Timestamp
	b.WriteString(e.Time.Format(time.RFC3339))
	b.WriteString(" ")

	// Severity
	b.WriteString("[")
	b.WriteString(e.Severity)
	b.WriteString("] ")

	// Message
	b.WriteString(strings.TrimRight(e.Message, "\n"))

	// Add payload only if it exists.
	if len(e.Payload) > 0 {
		b.WriteString(" {")

		keys := make([]string, 0, len(e.Payload))

		for k := range e.Payload {
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
			val := e.Payload[k]

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
