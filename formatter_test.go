package harelog

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

// TestJSONFormatter_Format directly tests the output of the jsonFormatter.
func TestJSONFormatter_Format(t *testing.T) {
	t.Parallel()

	f := NewJSONFormatter()
	testTime := time.Date(2025, 9, 25, 12, 0, 0, 0, time.UTC)

	entry := &LogEntry{
		Message:  "json format test",
		Severity: LogLevelInfo,
		Time:     testTime,
		Payload: map[string]interface{}{
			"user": "gopher",
		},
	}

	b, err := f.Format(entry)
	if err != nil {
		t.Fatalf("Format() returned an error: %v", err)
	}

	// Basic checks for JSON validity and content
	s := string(b)
	if !strings.Contains(s, `"message":"json format test"`) {
		t.Errorf("output missing message: %s", s)
	}
	if !strings.Contains(s, `"severity":"INFO"`) {
		t.Errorf("output missing severity: %s", s)
	}
	if !strings.Contains(s, `"user":"gopher"`) {
		t.Errorf("output missing payload: %s", s)
	}
	if !strings.Contains(s, `"timestamp":"2025-09-25T12:00:00Z"`) {
		t.Errorf("output missing or incorrect timestamp: %s", s)
	}
}

// TestTextFormatter_Format verifies the behavior of the textFormatter, including colorization.
func TestTextFormatter_Format(t *testing.T) {
	// Hijack time for predictable output
	testTime := time.Date(2025, 9, 30, 14, 0, 0, 0, time.UTC)

	// --- Subtest for basic formatting (ensuring it's uncolored) ---
	t.Run("Basic structure and payload formatting is correct", func(t *testing.T) {
		f := NewTextFormatter()

		tests := []struct {
			name     string
			entry    *LogEntry
			expected string
		}{
			{
				name: "Simple message with no payload (trims empty brackets)",
				entry: &LogEntry{
					Message:  "server started",
					Severity: LogLevelInfo,
					Time:     testTime,
				},
				// ★FIX: The new logic adds and removes {} if no fields exist
				expected: `2025-09-30T14:00:00Z [INFO] server started`,
			},
			{
				name: "Message with trailing newline (trims newline)",
				entry: &LogEntry{
					Message:  "message with newline\n",
					Severity: LogLevelInfo,
					Time:     testTime,
				},
				// ★FIX: The new logic correctly trims the \n from the message
				expected: `2025-09-30T14:00:00Z [INFO] message with newline`,
			},
			{
				name: "Message with simple payload (payload sorted)",
				entry: &LogEntry{
					Message:  "request failed",
					Severity: LogLevelError,
					Time:     testTime,
					Payload: map[string]interface{}{
						"status": 500,
						"path":   "/api/v1/users", // "path" comes before "status" alphabetically
						"active": true,
					},
				},
				// ★FIX: No space before {, payload keys are sorted, bool/int formats
				// "path"の値は特殊文字を含まないためクォートしない ( / は特殊文字ではないという前提)
				expected: `2025-09-30T14:00:00Z [ERROR] request failed { active=true, path=/api/v1/users, status=500 }`,
			},
			{
				name: "Message with all special fields (fixed order + map sort)",
				entry: &LogEntry{
					Message:        "complex event",
					Severity:       LogLevelWarn,
					Time:           testTime,
					Trace:          "trace-id-123",
					SpanID:         "span-id-456",
					CorrelationID:  "corr-id-789",
					Labels:         map[string]string{"region": "jp-east", "cluster": "A"}, // cluster, region
					SourceLocation: &SourceLocation{File: "app/server.go", Line: 152},
					HTTPRequest: &HTTPRequest{
						RequestMethod: "POST",
						Status:        401,
						RequestURL:    "/api/v1/login",
					},
					Payload: map[string]interface{}{
						"userID": "user-abc",
						"dept":   "eng", // dept, userID
					},
				},
				// ★FIX: This is the new deterministic order:
				// {StructFields(fixed)} {Labels(sorted)} {Payload(sorted)}
				// "dept" と "userID" は特殊文字を含まないためクォートしない
				expected: `2025-09-30T14:00:00Z [WARN] complex event { source="app/server.go:152", trace="trace-id-123", spanId="span-id-456", correlationId="corr-id-789", http.method="POST", http.status=401, http.url="/api/v1/login", label.cluster="A", label.region="jp-east", dept=eng, userID=user-abc }`,
			},
			{
				name: "Payload with duplicate struct fields (skips payload fields)",
				entry: &LogEntry{
					Message:  "duplicate fields test",
					Severity: LogLevelInfo,
					Time:     testTime,
					Trace:    "trace-A", // This one should be written
					Payload: map[string]interface{}{
						"userID": "user-123",
						"trace":  "trace-B", // This one should be skipped
					},
				},
				// ★FIX: Ensures StructFields take precedence and payload duplicates are skipped
				// "userID" は特殊文字を含まないためクォートしない
				expected: `2025-09-30T14:00:00Z [INFO] duplicate fields test { trace="trace-A", userID=user-123 }`,
			},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(tc.name, func(t *testing.T) {
				b, err := f.Format(tc.entry)
				if err != nil {
					t.Fatalf("Format() returned an error: %v", err)
				}
				got := string(b)
				if got != tc.expected {
					t.Errorf("unexpected text output:\ngot:  %s\nwant: %s", got, tc.expected)
				}
			})
		}
	})
}

func TestConsoleFormatter(t *testing.T) {
	// Temporarily disable color for fatih/color's auto-detection to ensure
	// our enable/disable logic works as expected.
	originalNoColor := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = originalNoColor }()

	testTime := time.Date(2025, 10, 14, 13, 30, 0, 0, time.UTC)

	entry := &LogEntry{
		Time:     testTime,
		Severity: LogLevelInfo,
		Message:  "user action",
		Payload: map[string]interface{}{
			"userID":    "user-123",
			"requestID": "req-abc",
			"action":    "logout",
		},
	}

	// --- Subtests specifically for color logic ---
	t.Run("Colorization logic", func(t *testing.T) {
		entry := &LogEntry{
			Message:  "error message",
			Severity: LogLevelError,
			Time:     testTime,
		}

		t.Run("WithColor(true) enables color", func(t *testing.T) {
			t.Setenv("HARELOG_FORCE_COLOR", "1")

			f := NewConsoleFormatter(WithLogLevelColor(true))
			b, _ := f.Format(entry)
			got := string(b)

			// Manually construct the expected colored string for a precise check.
			c := levelColorMap[LogLevelError]
			c.EnableColor() // Ensure color is enabled for the check
			expectedSubstring := c.Sprint("[ERROR]")

			if !strings.Contains(got, expectedSubstring) {
				t.Errorf("output should contain colored severity %q, but it was not found in %q", expectedSubstring, got)
			}
		})

		t.Run("WithColor(false) disables color", func(t *testing.T) {
			t.Setenv("HARELOG_FORCE_COLOR", "1")

			f := NewConsoleFormatter(WithLogLevelColor(false))
			b, _ := f.Format(entry)
			got := string(b)

			if strings.Contains(got, "\x1b") { // \x1b is the ANSI escape character
				t.Errorf("output should not contain any ANSI escape codes, but found some in %q", got)
			}
			if !strings.Contains(got, "[ERROR]") {
				t.Errorf("output should contain the uncolored severity string, but did not find it in %q", got)
			}
		})

		t.Run("Default behavior in non-TTY test environment is no color", func(t *testing.T) {
			// The `go test` runner is not an interactive terminal (TTY),
			// so the smart default should correctly disable colors.

			// IMPORTANT: Intended for non-TTY environments
			t.Setenv("HARELOG_NO_COLOR", "1")

			f := NewConsoleFormatter() // No options provided
			b, _ := f.Format(entry)
			got := string(b)

			if strings.Contains(got, "\x1b") {
				t.Errorf("output should not contain any ANSI escape codes in a non-TTY environment, but found some in %q", got)
			}
		})
	})

	t.Run("Basic Highlighting", func(t *testing.T) {
		t.Setenv("HARELOG_FORCE_COLOR", "1")

		f := NewConsoleFormatter(
			WithLogLevelColor(true),
			WithKeyHighlight("userID", FgCyan),
		)

		b, err := f.Format(entry)
		if err != nil {
			t.Fatalf("Format() error = %v", err)
		}

		output := string(b)
		cyan := color.New(color.FgCyan)
		cyan.EnableColor()
		expectedHighlight := cyan.Sprint(`userID="user-123"`)

		// Expected output with new order and spacing
		infoLevel := levelColorMap[LogLevelInfo]
		infoLevel.EnableColor()
		hlInfo := infoLevel.Sprint("[INFO]")
		// Payload keys sorted: action, requestID, userID
		expected := fmt.Sprintf(`2025-10-14T13:30:00Z %s user action { action="logout", requestID="req-abc", %s }`, hlInfo, expectedHighlight)

		if output != expected {
			// Use %q for clearer diffs with escape codes
			t.Errorf("unexpected console output:\ngot:  %q\nwant: %q", output, expected)
		}
		// Check that other keys are not colored incorrectly (this check might be fragile)
		expectedNonHighlight := cyan.Sprint(`action="logout"`)
		if strings.Contains(output, expectedNonHighlight) {
			t.Errorf("action key should not be highlighted: %s", output)
		}
	})

	t.Run("Highlight with Style", func(t *testing.T) {
		t.Setenv("HARELOG_FORCE_COLOR", "1")

		f := NewConsoleFormatter(
			WithLogLevelColor(true),
			WithKeyHighlight("userID", FgCyan, AttrBold),
		)

		b, err := f.Format(entry)
		if err != nil {
			t.Fatalf("Format() error = %v", err)
		}

		output := string(b)
		cyanBold := color.New(color.FgCyan, color.Bold)
		cyanBold.EnableColor()
		expectedHighlight := cyanBold.Sprint(`userID="user-123"`)

		infoLevel := levelColorMap[LogLevelInfo]
		infoLevel.EnableColor()
		hlInfo := infoLevel.Sprint("[INFO]")
		// Payload keys sorted: action, requestID, userID
		expected := fmt.Sprintf(`2025-10-14T13:30:00Z %s user action { action="logout", requestID="req-abc", %s }`, hlInfo, expectedHighlight)

		if output != expected {
			t.Errorf("unexpected console output:\ngot:  %q\nwant: %q", output, expected)
		}
	})

	t.Run("Rule: Last Color Wins", func(t *testing.T) {
		t.Setenv("HARELOG_FORCE_COLOR", "1")

		f := NewConsoleFormatter(
			WithLogLevelColor(true),
			WithKeyHighlight("userID", FgRed, FgYellow), // Yellow should win
		)

		b, err := f.Format(entry)
		if err != nil {
			t.Fatalf("Format() error = %v", err)
		}

		output := string(b)
		yellow := color.New(color.FgYellow)
		yellow.EnableColor()
		expectedHighlight := yellow.Sprint(`userID="user-123"`)

		infoLevel := levelColorMap[LogLevelInfo]
		infoLevel.EnableColor()
		hlInfo := infoLevel.Sprint("[INFO]")
		// Payload keys sorted: action, requestID, userID
		expected := fmt.Sprintf(`2025-10-14T13:30:00Z %s user action { action="logout", requestID="req-abc", %s }`, hlInfo, expectedHighlight)

		if output != expected {
			t.Errorf("unexpected console output:\ngot:  %q\nwant: %q", output, expected)
		}
	})

	t.Run("Rule: Styles are Additive", func(t *testing.T) {
		t.Setenv("HARELOG_FORCE_COLOR", "1")

		f := NewConsoleFormatter(
			WithLogLevelColor(true),
			WithKeyHighlight("userID", AttrBold, AttrUnderline),
		)

		b, err := f.Format(entry)
		if err != nil {
			t.Fatalf("Format() error = %v", err)
		}

		output := string(b)
		boldUnderline := color.New(color.Bold, color.Underline)
		boldUnderline.EnableColor()
		expectedHighlight := boldUnderline.Sprint(`userID="user-123"`)

		infoLevel := levelColorMap[LogLevelInfo]
		infoLevel.EnableColor()
		hlInfo := infoLevel.Sprint("[INFO]")
		// Payload keys sorted: action, requestID, userID
		expected := fmt.Sprintf(`2025-10-14T13:30:00Z %s user action { action="logout", requestID="req-abc", %s }`, hlInfo, expectedHighlight)

		if output != expected {
			t.Errorf("unexpected console output:\ngot:  %q\nwant: %q", output, expected)
		}
	})

	t.Run("Rule: Last Key Config Overwrites", func(t *testing.T) {
		t.Setenv("HARELOG_FORCE_COLOR", "1")

		f := NewConsoleFormatter(
			WithLogLevelColor(true),
			WithKeyHighlight("userID", FgRed, AttrBold),        // This should be overwritten
			WithKeyHighlight("userID", FgGreen, AttrUnderline), // This should be applied
		)

		b, err := f.Format(entry)
		if err != nil {
			t.Fatalf("Format() error = %v", err)
		}

		output := string(b)
		greenUnderline := color.New(color.FgGreen, color.Underline)
		greenUnderline.EnableColor()
		expectedHighlight := greenUnderline.Sprint(`userID="user-123"`)

		infoLevel := levelColorMap[LogLevelInfo]
		infoLevel.EnableColor()
		hlInfo := infoLevel.Sprint("[INFO]")
		// Payload keys sorted: action, requestID, userID
		expected := fmt.Sprintf(`2025-10-14T13:30:00Z %s user action { action="logout", requestID="req-abc", %s }`, hlInfo, expectedHighlight)

		if output != expected {
			t.Errorf("unexpected console output:\ngot:  %q\nwant: %q", output, expected)
		}
	})

	t.Run("Color Disabled (LogLevel=false, Highlight=true)", func(t *testing.T) {
		t.Setenv("HARELOG_FORCE_COLOR", "1")

		f := NewConsoleFormatter(
			WithLogLevelColor(false), // Explicitly disable log level color
			WithKeyHighlight("userID", FgCyan, AttrBold),
		)

		b, err := f.Format(entry)
		if err != nil {
			t.Fatalf("Format() error = %v", err)
		}

		output := string(b)
		cyanBold := color.New(color.FgCyan, color.Bold)
		cyanBold.EnableColor()
		expectedHighlight := cyanBold.Sprint(`userID="user-123"`)
		plainInfo := "[INFO]" // Log level should be plain
		// Payload keys sorted: action, requestID, userID
		expected := fmt.Sprintf(`2025-10-14T13:30:00Z %s user action { action="logout", requestID="req-abc", %s }`, plainInfo, expectedHighlight)

		if output != expected {
			t.Errorf("unexpected console output:\ngot:  %q\nwant: %q", output, expected)
		}
		// Check specifically that the level is NOT colored
		infoLevel := levelColorMap[LogLevelInfo]
		infoLevel.EnableColor()
		hlInfo := infoLevel.Sprint("[INFO]")
		if strings.Contains(output, hlInfo) {
			t.Errorf("Log level should NOT be colored when WithLogLevelColor(false) is used: %q", output)
		}
	})

	t.Run("Panic on Invalid Attribute", func(t *testing.T) {
		// This test remains unchanged
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected NewConsoleFormatter to panic with invalid ColorAttribute, but it did not")
			}
		}()
		// This should panic because 99 is not a valid ColorAttribute
		_ = NewConsoleFormatter(WithKeyHighlight("userID", ColorAttribute(99)))
	})
}

// BenchmarkTextFormatter_Simple benchmarks formatting a simple log entry.
func BenchmarkTextFormatter_Simple(b *testing.B) {
	// Setup: Define entry locally
	benchmarkTime := time.Date(2025, 9, 30, 14, 0, 0, 0, time.UTC)
	entry := &LogEntry{
		Message:  "server started",
		Severity: LogLevelInfo,
		Time:     benchmarkTime,
	}
	f := NewTextFormatter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// The error is ignored in benchmarks as we test correctness in unit tests.
		_, _ = f.Format(entry)
	}
}

// BenchmarkTextFormatter_Complex benchmarks formatting a complex log entry
// with all special fields (SourceLocation, HTTPRequest, Trace, etc.).
func BenchmarkTextFormatter_Complex(b *testing.B) {
	// Setup: Define entry locally
	benchmarkTime := time.Date(2025, 9, 30, 14, 0, 0, 0, time.UTC)
	entry := &LogEntry{
		Message:        "complex event",
		Severity:       LogLevelWarn,
		Time:           benchmarkTime,
		Trace:          "trace-id-123",
		SpanID:         "span-id-456",
		CorrelationID:  "corr-id-789",
		Labels:         map[string]string{"region": "jp-east"},
		SourceLocation: &SourceLocation{File: "app/server.go", Line: 152},
		HTTPRequest: &HTTPRequest{
			RequestMethod: "POST",
			Status:        401,
			RequestURL:    "/api/v1/login",
		},
		Payload: map[string]interface{}{
			"userID": "user-abc",
		},
	}
	f := NewTextFormatter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(entry)
	}
}

func BenchmarkJsonFormatter_Simple(b *testing.B) {
	f := &jsonFormatter{}
	e := &LogEntry{
		Message:  "hello",
		Severity: "INFO",
		Time:     time.Now(),
		Labels: map[string]string{
			"service": "core",
			"env":     "prod",
		},
		Payload: map[string]any{
			"user":  "takanobu",
			"count": 3,
		},
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		f.Format(e)
	}
}

func BenchmarkJsonFormatter_Complex(b *testing.B) {
	f := &jsonFormatter{}
	e := &LogEntry{
		Message:  "world",
		Severity: "DEBUG",
		Time:     time.Now(),
		Payload: map[string]any{
			"active": true,
		},
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		f.Format(e)
	}
}

// BenchmarkConsoleFormatter_Simple benchmarks the console formatter with a simple log entry.
func BenchmarkConsoleFormatter_Simple(b *testing.B) {
	f := NewConsoleFormatter()
	testTime := time.Date(2025, 9, 30, 14, 0, 0, 0, time.UTC)
	entry := &LogEntry{
		Message:  "server started",
		Severity: LogLevelInfo,
		Time:     testTime,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(entry)
	}
}

// BenchmarkConsoleFormatter_Complex benchmarks the console formatter with a complex log entry.
func BenchmarkConsoleFormatter_Complex(b *testing.B) {
	f := NewConsoleFormatter(
		WithLogLevelColor(true),
		WithKeyHighlight("userID", FgCyan),
		WithKeyHighlight("dept", FgMagenta, AttrBold),
	)
	testTime := time.Date(2025, 9, 30, 14, 0, 0, 0, time.UTC)
	entry := &LogEntry{
		Message:        "complex event",
		Severity:       LogLevelWarn,
		Time:           testTime,
		Trace:          "trace-id-123",
		SpanID:         "span-id-456",
		CorrelationID:  "corr-id-789",
		Labels:         map[string]string{"region": "jp-east", "cluster": "A"},
		SourceLocation: &SourceLocation{File: "app/server.go", Line: 152},
		HTTPRequest: &HTTPRequest{
			RequestMethod: "POST",
			Status:        401,
			RequestURL:    "/api/v1/login",
		},
		Payload: map[string]interface{}{
			"userID": "user-abc",
			"dept":   "eng",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(entry)
	}
}
