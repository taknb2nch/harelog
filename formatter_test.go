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

	entry := &logEntry{
		Message:  "json format test",
		Severity: string(LogLevelInfo),
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
	testTime := time.Date(2025, 9, 27, 11, 30, 0, 0, time.UTC)

	// --- Subtest for basic formatting (ensuring it's uncolored) ---
	t.Run("Basic structure and payload formatting is correct", func(t *testing.T) {
		f := NewTextFormatter(WithColor(false)) // Explicitly disable color

		entry := &logEntry{
			Message:  "request failed",
			Severity: string(LogLevelError),
			Time:     jsonTime{testTime},
			Payload: map[string]interface{}{
				"status": 500,
				"path":   "/api/v1/users",
			},
		}
		expected := `2025-09-27T11:30:00Z [ERROR] request failed {path="/api/v1/users", status=500}`

		b, err := f.Format(entry)
		if err != nil {
			t.Fatalf("Format() returned an error: %v", err)
		}
		got := string(b)
		if got != expected {
			t.Errorf("unexpected text output:\ngot:  %s\nwant: %s", got, expected)
		}
	})

	// --- Subtests specifically for color logic ---
	t.Run("Colorization logic", func(t *testing.T) {
		entry := &logEntry{
			Message:  "error message",
			Severity: string(LogLevelError),
			Time:     jsonTime{testTime},
		}

		t.Run("WithColor(true) enables color", func(t *testing.T) {
			f := NewTextFormatter(WithColor(true))
			b, _ := f.Format(entry)
			got := string(b)

			// Manually construct the expected colored string for a precise check.
			c := levelColorMap[string(LogLevelError)]
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
