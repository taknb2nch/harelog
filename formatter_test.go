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

// TestTextFormatter_Format directly tests the various outputs of the textFormatter.
func TestTextFormatter_Format(t *testing.T) {
	f := NewTextFormatter()
	testTime := time.Date(2025, 9, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		entry    *logEntry
		expected string
	}{
		{
			name: "Simple message with no payload",
			entry: &logEntry{
				Message:  "server started",
				Severity: string(LogLevelInfo),
				Time:     jsonTime{testTime},
			},
			expected: `2025-09-25T12:00:00Z [INFO] server started`,
		},
		{
			name: "Message with payload",
			entry: &logEntry{
				Message:  "request failed",
				Severity: string(LogLevelError),
				Time:     jsonTime{testTime},
				Payload: map[string]interface{}{
					"status": 500,
					"path":   "/api/v1/users",
				},
			},
			expected: `2025-09-25T12:00:00Z [ERROR] request failed {path="/api/v1/users", status=500}`,
		},
		{
			name: "Println message with trailing newline",
			entry: &logEntry{
				Message:  "processing item\n",
				Severity: string(LogLevelDebug),
				Time:     jsonTime{testTime},
			},
			expected: `2025-09-25T12:00:00Z [DEBUG] processing item`,
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
}
