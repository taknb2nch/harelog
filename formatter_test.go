package harelog

import (
	"strings"
	"testing"
	"time"
)

// TestJSONFormatter_Format directly tests the output of the jsonFormatter.
func TestJSONFormatter_Format(t *testing.T) {
	f := NewJSONFormatter()
	testTime := time.Date(2025, 9, 25, 12, 0, 0, 0, time.UTC)

	entry := &LogEntry{
		Message:  "json format test",
		Severity: LogLevelInfo,
		Time:     jsonTime{testTime},
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
		f := NewTextFormatter(WithColor(false)) // Explicitly disable color

		tests := []struct {
			name     string
			entry    *LogEntry
			expected string
		}{
			{
				name: "Simple message with no payload",
				entry: &LogEntry{
					Message:  "server started",
					Severity: LogLevelInfo,
					Time:     jsonTime{testTime},
				},
				expected: `2025-09-30T14:00:00Z [INFO] server started`,
			},
			{
				name: "Message with simple payload",
				entry: &LogEntry{
					Message:  "request failed",
					Severity: LogLevelError,
					Time:     jsonTime{testTime},
					Payload: map[string]interface{}{
						"status": 500,
						"path":   "/api/v1/users",
					},
				},
				expected: `2025-09-30T14:00:00Z [ERROR] request failed {path="/api/v1/users", status=500}`,
			},
			{
				name: "Message with all special fields",
				entry: &LogEntry{
					Message:        "complex event",
					Severity:       LogLevelWarn,
					Time:           jsonTime{testTime},
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
				},
				// Note: keys are sorted alphabetically
				expected: `2025-09-30T14:00:00Z [WARN] complex event {correlationId="corr-id-789", http.method="POST", http.status=401, http.url="/api/v1/login", label.region="jp-east", source="app/server.go:152", spanId="span-id-456", trace="trace-id-123", userID="user-abc"}`,
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

	// --- Subtests specifically for color logic ---
	t.Run("Colorization logic", func(t *testing.T) {
		entry := &LogEntry{
			Message:  "error message",
			Severity: LogLevelError,
			Time:     jsonTime{testTime},
		}

		t.Run("WithColor(true) enables color", func(t *testing.T) {
			f := NewTextFormatter(WithColor(true))
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
			f := NewTextFormatter(WithColor(false))
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
			f := NewTextFormatter() // No options provided
			b, _ := f.Format(entry)
			got := string(b)

			if strings.Contains(got, "\x1b") {
				t.Errorf("output should not contain any ANSI escape codes in a non-TTY environment, but found some in %q", got)
			}
		})
	})
}
