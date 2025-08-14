package harelog

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestNew verifies that New() creates a logger with correct default values.
func TestNew(t *testing.T) {
	l := New()
	if l.out != os.Stderr {
		t.Errorf("expected default output to be os.Stderr, got %v", l.out)
	}
	if l.logLevel != logLevelValueInfo {
		t.Errorf("expected default level to be Info, got %v", l.logLevel)
	}
}

// TestParseLogLevel tests the log level parsing function.
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      logLevel
		expectErr bool
	}{
		{"Valid uppercase", "INFO", LogLevelInfo, false},
		{"Valid lowercase", "debug", LogLevelDebug, false},
		{"Valid mixed case", "WaRn", LogLevelWarn, false},
		{"Invalid level", "INVALID", "", true},
		{"Empty string", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLogLevel(tt.input)
			if (err != nil) != tt.expectErr {
				t.Errorf("ParseLogLevel() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseLogLevel() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLogLevels verifies that logging methods respect the set log level.
func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	l := New().WithLogLevel(LogLevelInfo)
	l = l.WithOutput(&buf)

	// This should be logged
	l.Infof("info message")
	if buf.Len() == 0 {
		t.Error("expected info message to be logged, but buffer is empty")
	}
	buf.Reset()

	// This should NOT be logged
	l.Debugf("debug message")
	if buf.Len() > 0 {
		t.Errorf("expected debug message not to be logged, but got: %s", buf.String())
	}
}

// TestWithMethods verifies the immutability of the logger for request-scoped info.
func TestWithMethods(t *testing.T) {
	var buf bytes.Buffer
	l1 := New().WithOutput(&buf)
	l2 := l1.WithPrefix("[request] ")
	l3 := l2.WithLabels(map[string]string{"user": "test"})
	l4 := l3.WithTrace("test-trace")

	// Ensure original loggers are not modified
	if l1.prefix != "" || len(l1.labels) > 0 || l1.trace != "" {
		t.Error("l1 should not have been modified")
	}
	if l2.prefix == "" || len(l2.labels) > 0 || l2.trace != "" {
		t.Error("l2 should only have a prefix")
	}
	if l3.prefix == "" || len(l3.labels) == 0 || l3.trace != "" {
		t.Error("l3 should have prefix and labels")
	}

	// Test output of the final logger
	l4.Infof("test message")
	output := buf.String()
	if !strings.Contains(output, "[request] test message") {
		t.Errorf("output should contain prefix and message, got: %s", output)
	}
	if !strings.Contains(output, `"user":"test"`) {
		t.Errorf("output should contain labels, got: %s", output)
	}
	if !strings.Contains(output, `"logging.googleapis.com/trace":"test-trace"`) {
		t.Errorf("output should contain trace, got: %s", output)
	}
}

// TestStructuredOutput verifies the JSON output of Infow for general payloads.
func TestStructuredOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New().WithOutput(&buf)

	l.Infow("user logged in", "user_id", 123, "ip_address", "127.0.0.1", "error", errors.New("test error"))

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log output: %v", err)
	}

	if msg, _ := entry["message"].(string); msg != "user logged in" {
		t.Errorf("unexpected message: got %q, want %q", msg, "user logged in")
	}
	if userID, _ := entry["user_id"].(float64); int(userID) != 123 {
		t.Errorf("unexpected user_id: got %v, want 123", userID)
	}
	if ip, _ := entry["ip_address"].(string); ip != "127.0.0.1" {
		t.Errorf("unexpected ip_address: got %q, want %q", ip, "127.0.0.1")
	}
	if errMsg, _ := entry["error"].(string); errMsg != "test error" {
		t.Errorf("unexpected error message: got %q, want %q", errMsg, "test error")
	}
}

// TestEventScopedFields verifies that event-scoped fields like SourceLocation and HTTPRequest are handled correctly.
func TestEventScopedFields(t *testing.T) {
	var buf bytes.Buffer
	l := New().WithOutput(&buf)

	sl := &SourceLocation{
		File:     "main.go",
		Line:     42,
		Function: "main.main",
	}
	req := &HTTPRequest{
		RequestMethod: "GET",
		RequestURL:    "/test",
		Status:        200,
	}

	l.Errorw("operation failed", "sourceLocation", sl, "httpRequest", req)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log output: %v", err)
	}

	// Check for sourceLocation
	slMap, ok := entry["logging.googleapis.com/sourceLocation"].(map[string]interface{})
	if !ok {
		t.Fatal("sourceLocation not found in log output")
	}
	if slMap["file"] != "main.go" || slMap["line"] != 42.0 { // JSON numbers are float64
		t.Errorf("sourceLocation is incorrect: %+v", slMap)
	}

	// Check for httpRequest
	reqMap, ok := entry["httpRequest"].(map[string]interface{})
	if !ok {
		t.Fatal("httpRequest not found in log output")
	}
	if reqMap["requestMethod"] != "GET" || reqMap["status"] != 200.0 {
		t.Errorf("httpRequest is incorrect: %+v", reqMap)
	}
}

// TestDefaultLogger verifies package-level functions and their thread safety.
func TestDefaultLogger(t *testing.T) {
	// Ensure we don't interfere with other tests by resetting at the end
	originalOutput := std.out
	originalLevel := std.logLevel
	defer func() {
		std.out = originalOutput
		std.logLevel = originalLevel
	}()

	var buf bytes.Buffer
	SetDefaultOutput(&buf)
	level, _ := ParseLogLevel("ERROR")
	SetDefaultLogLevel(level)

	// This should not be logged
	Infof("info message")
	if buf.Len() > 0 {
		t.Fatalf("expected info message to be suppressed, but got: %s", buf.String())
	}

	// This should be logged
	Errorf("error message")
	if buf.Len() == 0 {
		t.Fatal("expected error message to be logged, but buffer is empty")
	}
	buf.Reset()

	// Basic concurrency test
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			Infof("concurrent info") // Should be suppressed
			Errorf("concurrent error %d", id)
		}(i)
	}
	wg.Wait()

	// Count how many error messages were logged
	output := buf.String()
	lineCount := strings.Count(output, "\n")
	if lineCount != 10 {
		t.Errorf("expected 10 error lines from concurrent logging, but got %d", lineCount)
	}
}
