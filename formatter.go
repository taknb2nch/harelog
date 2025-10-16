package harelog

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
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
	e.Labels = nil // Set to nil, as it's a reference
	e.CorrelationID = ""
}

// Formatter is an interface for converting a logEntry into a byte slice.
type Formatter interface {
	Format(entry *LogEntry) ([]byte, error)
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

// textFormatter formats log entries as human-readable text.
type textFormatter struct {
	enableColor      bool
	isEnableColorSet bool
}

// TextFormatterOption configures a textFormatter.
type TextFormatterOption func(*textFormatter)

// NewTextFormatter creates a new TextFormatter.
func NewTextFormatter(opts ...TextFormatterOption) *textFormatter {
	formatter := &textFormatter{
		enableColor:      false,
		isEnableColorSet: false,
	}

	for _, opt := range opts {
		opt(formatter)
	}

	return formatter
}

// WithTextLevelColor is an option to enable or disable color output for the TextFormatter.
func WithTextLevelColor(enabled bool) TextFormatterOption {
	return func(f *textFormatter) {
		f.enableColor = enabled
		f.isEnableColorSet = true
	}
}

// Format converts a logEntry to a single-line text format.
func (f *textFormatter) Format(e *LogEntry) ([]byte, error) {
	var b bytes.Buffer

	useColor := f.shouldUseColor()

	f.writeHeader(&b, e, useColor)

	fields := f.aggregateFields(e)

	if len(fields) > 0 {
		b.WriteString(" {")

		f.writeFields(&b, fields)

		b.WriteString("}")
	}

	return b.Bytes(), nil
}

// should UseColor determines if color should be used for the output.
func (f *textFormatter) shouldUseColor() bool {
	if os.Getenv("HARELOG_NO_COLOR") != "" || os.Getenv("NO_COLOR") != "" {
		return false
	}

	if os.Getenv("HARELOG_FORCE_COLOR") != "" {
		return true
	}

	if f.isEnableColorSet {
		return f.enableColor
	}

	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stderr.Fd())
}

// writeHeader writes the common part of a text log (timestamp, level, message) to the buffer.
// It returns the determined 'useColor' boolean for use by field formatters.
func (f *textFormatter) writeHeader(b *bytes.Buffer, e *LogEntry, useColor bool) bool {
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

	return useColor
}

// aggregateFields gathers all relevant data from a LogEntry into a single map for formatting.
func (f *textFormatter) aggregateFields(e *LogEntry) map[string]interface{} {
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

	return fields
}

// writeFields formats and appends the key-value pairs to the buffer.
func (f *textFormatter) writeFields(b *bytes.Buffer, fields map[string]interface{}) {
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
	*textFormatter
	highlightColors map[string]*color.Color
}

// ConsoleFormatterOption is a functional option for configuring a ConsoleFormatter.
type ConsoleFormatterOption func(*consoleFormatter)

// NewConsoleFormatter creates a new ConsoleFormatter.
func NewConsoleFormatter(opts ...ConsoleFormatterOption) *consoleFormatter {
	formatter := &consoleFormatter{
		textFormatter:   &textFormatter{},
		highlightColors: make(map[string]*color.Color),
	}

	for _, opt := range opts {
		opt(formatter)
	}

	return formatter
}

// WithConsoleLevelColor is an option to enable or disable log level color output for the ConsoleFormatter.
func WithConsoleLevelColor(enabled bool) ConsoleFormatterOption {
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

	// Since ConsoleFormatter embeds textFormatter, it can call its methods directly.
	useColor := f.shouldUseColor()
	f.writeHeader(&b, e, useColor)

	// Aggregate fields
	fields := f.aggregateFields(e)

	// Custom field formatting with highlighting
	if len(fields) > 0 {
		b.WriteString(" {")

		f.writeHighlightedFields(&b, fields, useColor)

		b.WriteString("}")
	}

	return b.Bytes(), nil
}

// writeHighlightedFields formats and appends key-value pairs with highlighting.
func (f *consoleFormatter) writeHighlightedFields(b *bytes.Buffer, fields map[string]interface{}, useColor bool) {
	keys := make([]string, 0, len(fields))

	for k := range fields {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}

		val := fields[k]
		formattedVal := ""

		if s, ok := val.(string); ok {
			formattedVal = fmt.Sprintf("%q", s)
		} else {
			formattedVal = fmt.Sprint(val)
		}

		if c, ok := f.highlightColors[k]; ok && useColor {
			c.EnableColor()

			b.WriteString(c.Sprint(fmt.Sprintf("%s=%s", k, formattedVal)))
		} else {
			b.WriteString(k)
			b.WriteString("=")
			b.WriteString(formattedVal)
		}
	}
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
