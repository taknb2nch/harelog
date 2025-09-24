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
	// Test default formatter
	if _, ok := l.formatter.(*jsonFormatter); !ok {
		t.Errorf("expected default formatter to be jsonFormatter, got %T", l.formatter)
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

// TestWithMethods verifies the immutability of the logger.
func TestWithMethods(t *testing.T) {
	var buf bytes.Buffer
	l1 := New().WithOutput(&buf)
	l2 := l1.WithPrefix("[request] ")
	l3 := l2.WithLabels(map[string]string{"user": "test"})

	// Ensure l1 and l2 are not modified
	if l1.prefix != "" {
		t.Error("l1 should not have a prefix")
	}
	if _, ok := l1.labels["user"]; ok {
		t.Error("l1 should not have labels")
	}
	if l2.prefix == "" {
		t.Error("l2 should have a prefix")
	}
	if _, ok := l2.labels["user"]; ok {
		t.Error("l2 should not have labels")
	}

	// Test output of the final logger
	l3.Infof("test message")
	output := buf.String()
	if !strings.Contains(output, "[request] test message") {
		t.Errorf("output should contain prefix and message, got: %s", output)
	}
	if !strings.Contains(output, `"user":"test"`) {
		t.Errorf("output should contain labels, got: %s", output)
	}
}

// TestStructuredOutput verifies the JSON output of Infow.
func TestStructuredOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New().WithOutput(&buf)
	l.Infow("user logged in", "user_id", 123, "ip_address", "127.0.0.1")

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
}

// TestSpecialFields verifies the handling of special keys like error, httpRequest, and sourceLocation.
func TestSpecialFields(t *testing.T) {
	t.Run("error field", func(t *testing.T) {
		var buf bytes.Buffer
		l := New().WithOutput(&buf)
		err := errors.New("database connection failed")

		l.Errorw("operation failed", "error", err)

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal log output: %v", err)
		}
		if errMsg, _ := entry["error"].(string); errMsg != "database connection failed" {
			t.Errorf("unexpected error message: got %q, want %q", errMsg, "database connection failed")
		}
	})

	t.Run("sourceLocation field", func(t *testing.T) {
		var buf bytes.Buffer
		l := New().WithOutput(&buf)
		sl := &SourceLocation{
			File:     "main.go",
			Line:     42,
			Function: "main.main",
		}
		l.Errorw("error with source", "sourceLocation", sl)

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal log output: %v", err)
		}

		slMap, ok := entry["logging.googleapis.com/sourceLocation"].(map[string]interface{})
		if !ok {
			t.Fatal("sourceLocation not found or not a map in log output")
		}
		if file, _ := slMap["file"].(string); file != "main.go" {
			t.Errorf("unexpected file in sourceLocation: got %q, want %q", file, "main.go")
		}
		if line, _ := slMap["line"].(float64); int(line) != 42 {
			t.Errorf("unexpected line in sourceLocation: got %v, want 42", line)
		}
	})
}

// TestDefaultLogger verifies package-level functions.
func TestDefaultLogger(t *testing.T) {
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

	Infof("info message")
	if buf.Len() > 0 {
		t.Errorf("expected info message to be suppressed, but got: %s", buf.String())
	}

	Errorf("error message")
	if buf.Len() == 0 {
		t.Error("expected error message to be logged, but buffer is empty")
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			Infof("concurrent info")
			Errorf("concurrent error %d", id)
		}(i)
	}
	wg.Wait()
}

// TestPrintMethods verifies the Print, Printf, and Println methods.
func TestPrintMethods(t *testing.T) {
	var buf bytes.Buffer
	l := New().WithOutput(&buf)

	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{"Printf", func() { l.Printf("hello %s", "world") }, `{"message":"hello world","severity":"INFO"`},
		{"Print", func() { l.Print("hello", "world", 123) }, `{"message":"hello world 123","severity":"INFO"`},
		{"Println", func() { l.Println("hello", "world") }, `{"message":"hello world\n","severity":"INFO"`},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.logFunc()
			if !strings.HasPrefix(buf.String(), tc.expected) {
				t.Errorf("unexpected log output for %s:\ngot:  %s\nwant prefix: %s", tc.name, buf.String(), tc.expected)
			}
		})
	}
}

// TestFatalMethods verifies the Fatal, Fatalf, and Fatalln methods.
func TestFatalMethods(t *testing.T) {
	var buf bytes.Buffer
	l := New().WithOutput(&buf)

	var exitCode int
	originalExit := osExit
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = originalExit }()

	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{"Fatalf", func() { l.Fatalf("fatal %s", "error") }, `{"message":"fatal error","severity":"CRITICAL"`},
		{"Fatal", func() { l.Fatal("fatal", "error", 123) }, `{"message":"fatal error 123","severity":"CRITICAL"`},
		{"Fatalln", func() { l.Fatalln("fatal", "error") }, `{"message":"fatal error\n","severity":"CRITICAL"`},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			exitCode = 0
			tc.logFunc()
			if !strings.HasPrefix(buf.String(), tc.expected) {
				t.Errorf("unexpected log output for %s:\ngot:  %s\nwant prefix: %s", tc.name, buf.String(), tc.expected)
			}
			if exitCode != 1 {
				t.Errorf("expected os.Exit(1) to be called for %s, but exit code was %d", tc.name, exitCode)
			}
		})
	}
}

// TestFormatters verifies the WithFormatter option and logger's integration with formatters.
func TestFormatters(t *testing.T) {
	var buf bytes.Buffer

	// Test that New() without options uses JSONFormatter
	t.Run("Default Formatter is JSON", func(t *testing.T) {
		buf.Reset()
		logger := New(WithOutput(&buf))
		logger.Infow("json test", "key", "value")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("expected valid JSON, but got error: %v", err)
		}
	})

	// Test switching to TextFormatter
	t.Run("WithFormatter switches to TextFormatter", func(t *testing.T) {
		buf.Reset()
		logger := New(
			WithOutput(&buf),
			WithFormatter(NewTextFormatter()),
		)

		// This call should now use the text formatter
		logger.Infow("text test", "key", "value")

		got := strings.TrimSpace(buf.String())

		// Check for text format characteristics, not exact time
		if !strings.Contains(got, "[INFO] text test") {
			t.Errorf("output does not contain text message: %s", got)
		}
		if !strings.Contains(got, `{key="value"}`) {
			t.Errorf("output does not contain text payload: %s", got)
		}
		if strings.HasPrefix(got, "{") {
			t.Errorf("output appears to be JSON, not text: %s", got)
		}
	})
}
