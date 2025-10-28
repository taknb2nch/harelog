package harelog

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/fatih/color"
	json "github.com/goccy/go-json"
	isatty "github.com/mattn/go-isatty"
)

// levelColorMap maps log levels to their corresponding color functions.
// This is a private implementation detail of the textFormatter.
var levelColorMap = map[LogLevel]*color.Color{
	LogLevelError:    color.New(color.FgRed),
	LogLevelCritical: color.New(color.FgHiRed, color.Bold),
	LogLevelWarn:     color.New(color.FgYellow),
	LogLevelInfo:     color.New(color.FgGreen),
	LogLevelDebug:    color.New(color.FgCyan),
}

var jsonEntryPool = sync.Pool{
	New: func() any {
		return &jsonEntry{}
	},
}

type jsonEntry struct {
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
}

// Clear resets the jsonEntry fields to their zero values for safe reuse in the pool.
func (e *jsonEntry) Clear() {
	e.Message = ""
	e.Severity = ""
	e.Trace = ""
	e.SpanID = ""
	e.TraceSampled = nil
	e.HTTPRequest = nil
	e.SourceLocation = nil
	e.Time = time.Time{}
	// e.Labels = nil // Set to nil, as it's a reference
	e.CorrelationID = ""

	clearOrResetMap(&e.Labels, 16)
}

// Formatter is an interface for converting a logEntry into a byte slice.
type Formatter interface {
	Format(entry *LogEntry) ([]byte, error)
	FormatMessageOnly(entry *LogEntry) ([]byte, error)
}

// jsonFormatter formats log entries as JSON.
type jsonFormatter struct{}

// NewJSONFormatter creates a new JSONFormatter.
func NewJSONFormatter() *jsonFormatter {
	return &jsonFormatter{}
}

// Format converts a logEntry to JSON format.
func (f *jsonFormatter) Format(e *LogEntry) ([]byte, error) {
	head := jsonEntryPool.Get().(*jsonEntry)

	defer func() {
		head.Clear()
		jsonEntryPool.Put(head)
	}()

	head.Message = e.Message
	head.Severity = e.Severity
	head.Trace = e.Trace
	head.SpanID = e.SpanID
	head.TraceSampled = e.TraceSampled
	head.HTTPRequest = e.HTTPRequest
	head.SourceLocation = e.SourceLocation
	head.Time = e.Time
	head.Labels = e.Labels
	head.CorrelationID = e.CorrelationID

	headerBytes, err := json.Marshal(head)
	if err != nil {
		return nil, err
	}

	if len(e.Payload) == 0 {
		return headerBytes, nil
	}

	payloadBytes, err := json.Marshal(e.Payload)
	if err != nil {
		return nil, err
	}

	if len(headerBytes) <= 2 {
		return payloadBytes, nil
	}

	out := headerBytes[:len(headerBytes)-1]
	out = append(out, ',')
	out = append(out, payloadBytes[1:]...)

	return out, nil
}

func (f *jsonFormatter) FormatMessageOnly(e *LogEntry) ([]byte, error) {
	var b bytes.Buffer

	// Timestamp
	b.Grow(32)
	b.Write(e.Time.AppendFormat(nil, time.RFC3339))
	b.WriteByte(' ')

	// Log Level
	b.WriteByte('[')
	b.WriteString(string(e.Severity))
	b.WriteByte(']')
	b.WriteByte(' ')

	// Message
	b.WriteString(e.Message)

	return b.Bytes(), nil
}

// textFormatter formats log entries as human-readable text.
type textFormatter struct{}

// NewTextFormatter creates a new TextFormatter.
func NewTextFormatter() *textFormatter {
	return &textFormatter{}
}

// Format converts a logEntry to a single-line text format.
func (f *textFormatter) Format(e *LogEntry) ([]byte, error) {
	var b bytes.Buffer
	var scratch [64]byte
	var buf []byte

	// Timestamp
	b.Grow(32)
	b.Write(e.Time.AppendFormat(nil, time.RFC3339))
	b.WriteByte(' ')

	b.WriteByte('[')
	b.WriteString(string(e.Severity))
	b.WriteByte(']')
	b.WriteByte(' ')

	// Message
	b.WriteString(e.Message)

	buf = b.Bytes()

	if len(buf) > 0 && buf[len(buf)-1] == '\n' {
		b.Truncate(len(buf) - 1)
	}

	isSource := false
	isTrace := false
	isSpanID := false
	isCorrelationId := false
	isHttpRequest := false
	isLabel := false
	isPayload := false

	b.WriteByte(' ')
	b.WriteByte('{')
	b.WriteByte(' ')

	// Add special fields if they exist and are not already in the payload
	if e.SourceLocation != nil {
		if _, ok := e.Payload["sourceLocation"]; !ok {
			// Format source location for readability
			b.WriteString("source")
			b.WriteByte('=')
			b.WriteByte('"')
			b.WriteString(e.SourceLocation.File)
			b.WriteByte(':')
			b.Write(strconv.AppendInt(scratch[:0], int64(e.SourceLocation.Line), 10))
			b.WriteByte('"')
			b.WriteByte(',')
			b.WriteByte(' ')

			isSource = true
		}
	}

	if e.Trace != "" {
		b.WriteString("trace")
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(e.Trace)
		b.WriteByte('"')
		b.WriteByte(',')
		b.WriteByte(' ')

		isTrace = true
	}

	if e.SpanID != "" {
		b.WriteString("spanId")
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(e.SpanID)
		b.WriteByte('"')
		b.WriteByte(',')
		b.WriteByte(' ')

		isSpanID = true
	}

	if e.CorrelationID != "" {
		b.WriteString("correlationId")
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(e.CorrelationID)
		b.WriteByte('"')
		b.WriteByte(',')
		b.WriteByte(' ')

		isCorrelationId = true
	}

	if e.HTTPRequest != nil {
		// Extract the most useful parts of the HTTP request
		if e.HTTPRequest.RequestMethod != "" {
			b.WriteString("http.method")
			b.WriteByte('=')
			b.WriteByte('"')
			b.WriteString(e.HTTPRequest.RequestMethod)
			b.WriteByte('"')
			b.WriteByte(',')
			b.WriteByte(' ')

			isHttpRequest = true
		}
		if e.HTTPRequest.Status != 0 {
			b.WriteString("http.status")
			b.WriteByte('=')
			b.Write(strconv.AppendInt(scratch[:0], int64(e.HTTPRequest.Status), 10))
			b.WriteString(",")
			b.WriteByte(' ')

			isHttpRequest = true
		}
		if e.HTTPRequest.RequestURL != "" {
			b.WriteString("http.url")
			b.WriteByte('=')
			b.WriteByte('"')
			b.WriteString(e.HTTPRequest.RequestURL)
			b.WriteByte('"')
			b.WriteByte(',')
			b.WriteByte(' ')

			isHttpRequest = true
		}
	}

	keys := make([]string, 0, len(e.Labels))

	for k := range e.Labels {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, key := range keys {
		b.WriteString("label")
		b.WriteByte('.')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(e.Labels[key]))
		b.WriteByte(',')
		b.WriteByte(' ')

		isLabel = true
	}

	keys = make([]string, 0, len(e.Payload))

	for k := range e.Payload {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, key := range keys {
		if isTrace && key == "trace" {
			continue
		}

		if isSpanID && key == "spanId" {
			continue
		}

		if isCorrelationId && key == "correlationId" {
			continue
		}

		if isHttpRequest && key == "httpRequest" {
			continue
		}

		b.WriteString(key)
		b.WriteString("=")

		switch val := e.Payload[key].(type) {
		case string:
			appendStringValue(&b, val)
		case bool:
			scratch := [64]byte{}

			b.Write(strconv.AppendBool(scratch[:0], val))
		case int:
			scratch := [64]byte{}

			b.Write(strconv.AppendInt(scratch[:0], int64(val), 10))
		case int32:
			scratch := [64]byte{}

			b.Write(strconv.AppendInt(scratch[:0], int64(val), 10))
		case int64:
			scratch := [64]byte{}

			b.Write(strconv.AppendInt(scratch[:0], val, 10))
		case float32:
			scratch := [64]byte{}

			b.Write(strconv.AppendFloat(scratch[:0], float64(val), 'f', -1, 64))
		case float64:
			scratch := [64]byte{}

			b.Write(strconv.AppendFloat(scratch[:0], val, 'f', -1, 64))
		case fmt.Stringer:
			appendStringValue(&b, val.String())
		default:
			appendStringValue(&b, fmt.Sprint(val))
		}

		b.WriteByte(',')
		b.WriteByte(' ')

		isPayload = true
	}

	buf = b.Bytes()

	if isSource || isTrace || isSpanID || isCorrelationId || isHttpRequest || isLabel || isPayload {
		b.Truncate(len(buf) - 2)
		b.WriteByte(' ')
		b.WriteByte('}')
	} else {
		// space }
		b.Truncate(len(buf) - 3)
	}

	return b.Bytes(), nil
}

func (f *textFormatter) FormatMessageOnly(e *LogEntry) ([]byte, error) {
	var b bytes.Buffer

	// Timestamp
	b.Grow(32)
	b.Write(e.Time.AppendFormat(nil, time.RFC3339))
	b.WriteByte(' ')

	// Log Level
	b.WriteByte('[')
	b.WriteString(string(e.Severity))
	b.WriteByte(']')
	b.WriteByte(' ')

	// Message
	b.WriteString(e.Message)

	return b.Bytes(), nil
}

// ColorAttribute defines a text attribute like color or style for the ConsoleFormatter.
type ColorAttribute int

// Public constants for text attributes.
// These are used with WithKeyHighlight to configure the ConsoleFormatter.

// Constants for foreground text colors.
const (
	FgBlack ColorAttribute = iota + 1
	FgRed
	FgGreen
	FgYellow
	FgBlue
	FgMagenta
	FgCyan
	FgWhite
)

// Constants for text style attributes.
const (
	AttrBold ColorAttribute = iota + 20
	AttrUnderline
)

// consoleFormatter provides a rich, developer-focused text format.
// It supports highlighting specific key-value pairs to improve readability.
type consoleFormatter struct {
	enableColor      bool
	isEnableColorSet bool
	highlightColors  map[string]*color.Color
}

// ConsoleFormatterOption is a functional option for configuring a ConsoleFormatter.
type ConsoleFormatterOption func(*consoleFormatter)

// NewConsoleFormatter creates a new ConsoleFormatter.
func NewConsoleFormatter(opts ...ConsoleFormatterOption) *consoleFormatter {
	formatter := &consoleFormatter{
		enableColor:      false,
		isEnableColorSet: false,
		highlightColors:  make(map[string]*color.Color),
	}

	for _, opt := range opts {
		opt(formatter)
	}

	return formatter
}

// WithLogLevelColor is an option to enable or disable log level color output for the ConsoleFormatter.
func WithLogLevelColor(enabled bool) ConsoleFormatterOption {
	return func(f *consoleFormatter) {
		f.enableColor = enabled
		f.isEnableColorSet = true
	}
}

// WithKeyHighlight is a functional option for the ConsoleFormatter that configures
// highlighting for a specific key. This option can be passed multiple times.
// - Color attributes (Fg...): The last one specified wins.
// - Style attributes (Attr...): All specified styles are applied.
func WithKeyHighlight(key string, attrs ...ColorAttribute) ConsoleFormatterOption {
	return func(f *consoleFormatter) {
		var colorAttr color.Attribute
		isColorSet := false

		styleAttrs := make(map[color.Attribute]struct{})

		for _, attr := range attrs {
			cAttr := toFatihAttribute(attr)

			if cAttr >= color.FgBlack && cAttr <= color.FgWhite {
				colorAttr = cAttr
				isColorSet = true
			} else {
				styleAttrs[cAttr] = struct{}{}
			}
		}

		finalAttrs := make([]color.Attribute, 0, len(styleAttrs)+1)

		if isColorSet {
			finalAttrs = append(finalAttrs, colorAttr)
		}

		for attr := range styleAttrs {
			finalAttrs = append(finalAttrs, attr)
		}

		f.highlightColors[key] = color.New(finalAttrs...)
	}
}

// Format overrides the default TextFormatter's field formatting to add highlighting.
func (f *consoleFormatter) Format(e *LogEntry) ([]byte, error) {
	var b bytes.Buffer
	var scratch [64]byte
	var buf []byte
	var b2 bytes.Buffer

	isUseColor := f.shouldUseColor()

	// Timestamp
	b.Grow(32)
	b.Write(e.Time.AppendFormat(nil, time.RFC3339))
	b.WriteByte(' ')

	enableLogLevelColor := f.isEnableColorSet && f.enableColor

	if c, ok := levelColorMap[e.Severity]; ok && enableLogLevelColor {
		// Explicitly enable or disable color on the object for this call.
		if isUseColor {
			c.EnableColor()
		} else {
			c.DisableColor()
		}

		b.WriteString(c.Sprintf("[%s]", e.Severity))
	} else {
		b.WriteByte('[')
		b.WriteString(string(e.Severity))
		b.WriteByte(']')
	}

	b.WriteByte(' ')

	// Message
	b.WriteString(e.Message)

	buf = b.Bytes()

	if len(buf) > 0 && buf[len(buf)-1] == '\n' {
		b.Truncate(len(buf) - 1)
	}

	isSource := false
	isTrace := false
	isSpanID := false
	isCorrelationId := false
	isHttpRequest := false
	isLabel := false
	isPayload := false

	b.WriteByte(' ')
	b.WriteByte('{')
	b.WriteByte(' ')

	// Add special fields if they exist and are not already in the payload
	if e.SourceLocation != nil {
		if _, ok := e.Payload["sourceLocation"]; !ok {
			// Format source location for readability
			b.WriteString("source")
			b.WriteByte('=')
			b.WriteByte('"')
			b.WriteString(e.SourceLocation.File)
			b.WriteByte(':')
			b.Write(strconv.AppendInt(scratch[:0], int64(e.SourceLocation.Line), 10))
			b.WriteByte('"')
			b.WriteByte(',')
			b.WriteByte(' ')

			isSource = true
		}
	}

	if e.Trace != "" {
		b.WriteString("trace")
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(e.Trace)
		b.WriteByte('"')
		b.WriteByte(',')
		b.WriteByte(' ')

		isTrace = true
	}

	if e.SpanID != "" {
		b.WriteString("spanId")
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(e.SpanID)
		b.WriteByte('"')
		b.WriteByte(',')
		b.WriteByte(' ')

		isSpanID = true
	}

	if e.CorrelationID != "" {
		b.WriteString("correlationId")
		b.WriteByte('=')
		b.WriteByte('"')
		b.WriteString(e.CorrelationID)
		b.WriteByte('"')
		b.WriteByte(',')
		b.WriteByte(' ')

		isCorrelationId = true
	}

	if e.HTTPRequest != nil {
		// Extract the most useful parts of the HTTP request
		if e.HTTPRequest.RequestMethod != "" {
			b.WriteString("http.method")
			b.WriteByte('=')
			b.WriteByte('"')
			b.WriteString(e.HTTPRequest.RequestMethod)
			b.WriteByte('"')
			b.WriteByte(',')
			b.WriteByte(' ')

			isHttpRequest = true
		}
		if e.HTTPRequest.Status != 0 {
			b.WriteString("http.status")
			b.WriteByte('=')
			b.Write(strconv.AppendInt(scratch[:0], int64(e.HTTPRequest.Status), 10))
			b.WriteString(",")
			b.WriteByte(' ')

			isHttpRequest = true
		}
		if e.HTTPRequest.RequestURL != "" {
			b.WriteString("http.url")
			b.WriteByte('=')
			b.WriteByte('"')
			b.WriteString(e.HTTPRequest.RequestURL)
			b.WriteByte('"')
			b.WriteByte(',')
			b.WriteByte(' ')

			isHttpRequest = true
		}
	}

	keys := make([]string, 0, len(e.Labels))

	for k := range e.Labels {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, key := range keys {
		b.WriteString("label")
		b.WriteByte('.')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(e.Labels[key]))
		b.WriteByte(',')
		b.WriteByte(' ')

		isLabel = true
	}

	keys = make([]string, 0, len(e.Payload))

	for k := range e.Payload {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, key := range keys {
		if isTrace && key == "trace" {
			continue
		}

		if isSpanID && key == "spanId" {
			continue
		}

		if isCorrelationId && key == "correlationId" {
			continue
		}

		if isHttpRequest && key == "httpRequest" {
			continue
		}

		b2.Reset()

		switch val := e.Payload[key].(type) {
		case string:
			b2.WriteString(strconv.Quote(val))
		case bool:
			scratch := [64]byte{}

			b2.Write(strconv.AppendBool(scratch[:0], val))
		case int:
			scratch := [64]byte{}

			b2.Write(strconv.AppendInt(scratch[:0], int64(val), 10))
		case int32:
			scratch := [64]byte{}

			b2.Write(strconv.AppendInt(scratch[:0], int64(val), 10))
		case int64:
			scratch := [64]byte{}

			b2.Write(strconv.AppendInt(scratch[:0], val, 10))
		case float32:
			scratch := [64]byte{}

			b2.Write(strconv.AppendFloat(scratch[:0], float64(val), 'f', -1, 64))
		case float64:
			scratch := [64]byte{}

			b2.Write(strconv.AppendFloat(scratch[:0], val, 'f', -1, 64))
		case fmt.Stringer:
			b2.WriteString(val.String())
		default:
			b2.WriteString(fmt.Sprint(val))
		}

		//-----
		if c, ok := f.highlightColors[key]; ok && isUseColor {
			c.EnableColor()

			b.WriteString(c.Sprintf("%s=%s", key, b2.String()))
		} else {
			b.WriteString(key)
			b.WriteByte('=')
			b.Write(b2.Bytes())
		}
		//-----

		b.WriteByte(',')
		b.WriteByte(' ')

		isPayload = true
	}

	buf = b.Bytes()

	if isSource || isTrace || isSpanID || isCorrelationId || isHttpRequest || isLabel || isPayload {
		b.Truncate(len(buf) - 2)
		b.WriteByte(' ')
		b.WriteByte('}')
	} else {
		// space }
		b.Truncate(len(buf) - 3)
	}

	return b.Bytes(), nil
}

func (f *consoleFormatter) FormatMessageOnly(e *LogEntry) ([]byte, error) {
	var b bytes.Buffer

	// Timestamp
	b.Grow(32)
	b.Write(e.Time.AppendFormat(nil, time.RFC3339))
	b.WriteByte(' ')

	// Log Level
	b.WriteByte('[')
	b.WriteString(string(e.Severity))
	b.WriteByte(']')
	b.WriteByte(' ')

	// Message
	b.WriteString(e.Message)

	return b.Bytes(), nil
}

// should UseColor determines if color should be used for the output.
func (f *consoleFormatter) shouldUseColor() bool {
	if os.Getenv("HARELOG_NO_COLOR") != "" || os.Getenv("NO_COLOR") != "" {
		return false
	}

	if os.Getenv("HARELOG_FORCE_COLOR") != "" {
		return true
	}

	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stderr.Fd())
}

// toFatihAttribute converts our public ColorAttribute to an internal fatih/color.Attribute.
func toFatihAttribute(attr ColorAttribute) color.Attribute {
	switch attr {
	case FgBlack:
		return color.FgBlack
	case FgRed:
		return color.FgRed
	case FgGreen:
		return color.FgGreen
	case FgYellow:
		return color.FgYellow
	case FgBlue:
		return color.FgBlue
	case FgMagenta:
		return color.FgMagenta
	case FgCyan:
		return color.FgCyan
	case FgWhite:
		return color.FgWhite
	case AttrBold:
		return color.Bold
	case AttrUnderline:
		return color.Underline
	default:
		panic(fmt.Sprintf("harelog: invalid ColorAttribute provided: %d", attr))
	}
}

// appendStringValue use Quote for safety if needed
func appendStringValue(b *bytes.Buffer, value string) {
	if needsQuoting(value) {
		b.WriteString(strconv.Quote(value))
	} else {
		b.WriteString(value)
	}
}
