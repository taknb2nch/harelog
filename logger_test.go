package harelog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// osExitMutex protects the global osExit variable during tests.
var osExitMutex sync.Mutex

// mockOsExit safely mocks the global osExit variable for the duration of a test.
// It returns a function that returns the captured exit code.
func mockOsExit(t *testing.T) func() int {
	t.Helper() // This function is a test helper.

	osExitMutex.Lock() // Lock before changing the global variable.

	var exitCode int
	originalExit := osExit
	osExit = func(code int) {
		exitCode = code
	}

	// Use t.Cleanup to restore the original osExit function and unlock the mutex
	// when the test (and all its subtests) are complete.
	t.Cleanup(func() {
		osExit = originalExit
		osExitMutex.Unlock() // Unlock after restoring.
	})

	return func() int {
		return exitCode
	}
}

// TestNew verifies that New() creates a logger with correct default values.
func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("Default values", func(t *testing.T) {
		t.Parallel()

		l := New()
		if l.out != os.Stderr {
			t.Errorf("expected default output to be os.Stderr, got %v", l.out)
		}
		if l.logLevel != logLevelValueInfo {
			t.Errorf("expected default level to be Info, got %v", l.logLevel)
		}
		if _, ok := l.formatter.(*jsonFormatter); !ok {
			t.Errorf("expected default formatter to be jsonFormatter, got %T", l.formatter)
		}
	})

	t.Run("WithLogLevel option", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		// Create a logger with a non-default log level.
		logger := New(
			WithOutput(&buf),
			WithLogLevel(LogLevelDebug),
		)

		if logger.logLevel != logLevelValueDebug {
			t.Errorf("expected log level to be DEBUG, but got %v", logger.logLevel)
		}

		// Verify that the level is applied correctly.
		logger.Infof("this is an info message") // Should be logged
		if buf.Len() == 0 {
			t.Error("expected info message to be logged when level is DEBUG, but it was not")
		}
	})
}

// TestParseLogLevel tests the log level parsing function.
func TestParseLogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		want      LogLevel
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
	t.Parallel()

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
	t.Parallel()

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

// TestWithMethod verifies the functionality of the contextual logger.
func TestWithMethod(t *testing.T) {
	t.Parallel()

	t.Run("Context is added to logs", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf))
		childLogger := logger.With("service", "api", "requestID", "abc-123")

		childLogger.Infof("request received")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if service, _ := entry["service"].(string); service != "api" {
			t.Errorf("expected service to be 'api', got %q", service)
		}
		if reqID, _ := entry["requestID"].(string); reqID != "abc-123" {
			t.Errorf("expected requestID to be 'abc-123', got %q", reqID)
		}
	})

	t.Run("Formatted logs include context", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf))
		err := errors.New("context error")
		childLogger := logger.With("error", err, "requestID", "xyz-789")

		childLogger.Warnf("Operation failed for user %d", 123)

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if errMsg, _ := entry["error"].(string); errMsg != "context error" {
			t.Errorf("expected special key 'error' to be processed, got %q", errMsg)
		}
		if reqID, _ := entry["requestID"].(string); reqID != "xyz-789" {
			t.Errorf("expected requestID to be 'xyz-789', got %q", reqID)
		}
		if msg, _ := entry["message"].(string); msg != "Operation failed for user 123" {
			t.Errorf("unexpected message: got %q", msg)
		}
	})

	t.Run("Special keys with wrong type are kept at top level", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf))
		childLogger := logger.With(
			"httpRequest", "this is not a request object",
			"sourceLocation", 12345,
		)

		childLogger.Infof("testing wrong types")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		// The key should exist at the top level, but its value will be the raw (incorrect) type.
		if val, ok := entry["httpRequest"].(string); !ok || val != "this is not a request object" {
			t.Errorf("expected httpRequest to be a string in the output, got %T with value %v", entry["httpRequest"], entry["httpRequest"])
		}
		if val, ok := entry["sourceLocation"].(float64); !ok || int(val) != 12345 {
			t.Errorf("expected sourceLocation to be a number in the output, got %T with value %v", entry["sourceLocation"], entry["sourceLocation"])
		}
	})

	t.Run("With is immutable", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		parentLogger := New(WithOutput(&buf))
		_ = parentLogger.With("temporary", "value")

		parentLogger.Infof("parent log")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if _, exists := entry["temporary"]; exists {
			t.Error("parent logger should not be mutated by With")
		}
	})

	t.Run("Local scope overrides context", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf))
		childLogger := logger.With("status", "pending")

		childLogger.Infow("request completed", "status", "success")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		if status, _ := entry["status"].(string); status != "success" {
			t.Errorf("expected status to be 'success' (overridden), but got %q", status)
		}
	})

	t.Run("Panics on odd number of arguments", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected With to panic with an odd number of arguments, but it did not")
			}
		}()
		logger := New()
		_ = logger.With("key1", "value1", "key2")
	})

	t.Run("Panics on non-string key", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected With to panic with a non-string key, but it did not")
			}
		}()
		logger := New()
		_ = logger.With(123, "value1")
	})
}

// TestStructuredOutput verifies the JSON output of Infow.
func TestStructuredOutput(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	t.Run("error field", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
	// Save and restore original std logger
	originalStd := std
	defer func() {
		std = originalStd
	}()

	// setup helper resets std to a clean logger for each subtest
	setup := func() *bytes.Buffer {
		buf := &bytes.Buffer{}
		// Create a clean logger instance and set it as the default
		std = New(WithOutput(buf))
		return buf
	}

	t.Run("SetDefaultLogLevel", func(t *testing.T) {
		buf := setup()
		SetDefaultLogLevel(LogLevelError)

		Infof("info message") // Should be suppressed
		if buf.Len() > 0 {
			t.Errorf("expected info message to be suppressed, but got: %s", buf.String())
		}

		Errorf("error message") // Should be logged
		if buf.Len() == 0 {
			t.Error("expected error message to be logged, but buffer is empty")
		}
	})

	t.Run("SetDefaultFormatter", func(t *testing.T) {
		// IMPORTANT: Intended for non-TTY environments
		t.Setenv("HARELOG_NO_COLOR", "1")

		buf := setup()
		// Switch the default logger to use the text formatter
		SetDefaultFormatter(NewTextFormatter())

		Infow("text output test", "key", "value")

		got := strings.TrimSpace(buf.String())

		// Verify the output is in text format, not JSON
		if !strings.Contains(got, "[INFO] text output test") {
			t.Errorf("output does not contain text message: %s", got)
		}
		if !strings.Contains(got, `{key="value"}`) {
			t.Errorf("output does not contain text payload: %s", got)
		}
		if strings.HasPrefix(got, "{") {
			t.Errorf("output appears to be JSON, not text: %s", got)
		}
	})

	t.Run("Concurrency", func(t *testing.T) {
		// Set up a clean logger with a discard writer to avoid noisy output
		std = New(WithOutput(io.Discard))
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				Infof("concurrent info %d", id)
				Errorf("concurrent error %d", id)
				if id%10 == 0 {
					// Concurrently modify the default logger
					SetDefaultPrefix(fmt.Sprintf("[%d]", id))
				}
			}(i)
		}
		wg.Wait()
		// This test just checks for race conditions and panics.
	})
}

// TestPrintMethods verifies the Print, Printf, and Println methods.
func TestPrintMethods(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	var buf bytes.Buffer
	l := New().WithOutput(&buf)

	getExitCode := mockOsExit(t)

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

			tc.logFunc()

			if !strings.HasPrefix(buf.String(), tc.expected) {
				t.Errorf("unexpected log output for %s:\ngot:  %s\nwant prefix: %s", tc.name, buf.String(), tc.expected)
			}
			if getExitCode() != 1 {
				t.Errorf("expected os.Exit(1) to be called for %s, but exit code was %d", tc.name, getExitCode())
			}
		})
	}
}

// TestFatalwMethod verifies the Fatalw method.
func TestFatalwMethod(t *testing.T) {
	var buf bytes.Buffer

	l := New(WithOutput(&buf))

	getExitCode := mockOsExit(t)

	// Call the method to be tested
	l.Fatalw("database connection failed", "host", "localhost", "port", 5432)

	// 1. Verify that os.Exit(1) was called
	if getExitCode() != 1 {
		t.Errorf("expected os.Exit(1) to be called, but exit code was %d", getExitCode())
	}

	// 2. Verify the structured log output
	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to unmarshal log output: %v", err)
	}

	if msg, _ := entry["message"].(string); msg != "database connection failed" {
		t.Errorf("unexpected message: got %q, want %q", msg, "database connection failed")
	}
	if severity, _ := entry["severity"].(string); severity != string(LogLevelCritical) {
		t.Errorf("unexpected severity: got %q, want %q", severity, LogLevelCritical)
	}
	if host, _ := entry["host"].(string); host != "localhost" {
		t.Errorf("unexpected host: got %q, want %q", host, "localhost")
	}
	if port, _ := entry["port"].(float64); int(port) != 5432 {
		t.Errorf("unexpected port: got %v, want %v", port, 5432)
	}
}

// TestCtxMethods verifies the functionality of all context-aware methods.
func TestCtxMethods(t *testing.T) {
	t.Parallel()

	// Define a custom context key for testing, mimicking how real applications do it.
	type contextKey string
	const traceContextKey = contextKey("x-cloud-trace-context")

	t.Run("Values are extracted from context with ProjectID", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		// Create a logger with the Project ID configured via the new option.
		logger := New(
			WithOutput(&buf),
			WithProjectID("test-project"),
			WithTraceContextKey(traceContextKey),
		)
		ctx := context.WithValue(context.Background(), traceContextKey, "trace-from-ctx/span-from-ctx;o=1")

		logger.InfofCtx(ctx, "message with trace")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		expectedTrace := "projects/test-project/traces/trace-from-ctx"
		if trace, _ := entry["logging.googleapis.com/trace"].(string); trace != expectedTrace {
			t.Errorf("expected trace %q to be extracted, got %q", expectedTrace, trace)
		}
		if span, _ := entry["logging.googleapis.com/spanId"].(string); span != "span-from-ctx" {
			t.Errorf("expected spanId %q to be extracted, got %q", "span-from-ctx", span)
		}
	})

	t.Run("Precedence: Method args > With > Context", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		ctx := context.WithValue(context.Background(), traceContextKey, "ctx-trace/ctx-span")

		// Create a child logger with a conflicting trace value.
		loggerWithContext := New(WithOutput(&buf)).With("[logging.googleapis.com/trace](https://logging.googleapis.com/trace)", "with-trace")

		// Call a ...wCtx method with another conflicting trace value.
		loggerWithContext.InfowCtx(ctx, "testing precedence", "[logging.googleapis.com/trace](https://logging.googleapis.com/trace)", "arg-trace")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		// The value from the method argument ("arg-trace") should win.
		expectedTrace := "arg-trace"
		if trace, _ := entry["[logging.googleapis.com/trace](https://logging.googleapis.com/trace)"].(string); trace != expectedTrace {
			t.Errorf("precedence failed: expected trace to be %q, got %q", expectedTrace, trace)
		}
	})

	t.Run("Nil context behaves like non-Ctx version", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf))

		// Log with the non-Ctx version
		logger.Warnf("message %d", 1)
		expected := strings.TrimSpace(buf.String())
		buf.Reset()

		// Log with the Ctx version passing nil
		logger.WarnfCtx(nil, "message %d", 1)
		got := strings.TrimSpace(buf.String())

		// We can't compare directly due to timestamp, so we check for the message part.
		if !strings.Contains(got, `"message":"message 1"`) {
			t.Errorf("nil context call did not produce the expected message. Got: %s", got)
		}
		if !strings.Contains(expected, `"message":"message 1"`) {
			t.Errorf("non-Ctx call did not produce the expected message. Got: %s", expected)
		}
	})

	t.Run("FatalCtx logs and exits", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf))
		ctx := context.Background()

		getExitCode := mockOsExit(t)

		logger.FatalCtx(ctx, "fatal message from ctx")

		if !strings.Contains(buf.String(), `"message":"fatal message from ctx"`) {
			t.Errorf("FatalCtx did not log the correct message. Got: %s", buf.String())
		}
		if getExitCode() != 1 {
			t.Errorf("expected os.Exit(1) to be called from FatalCtx, but exit code was %d", getExitCode())
		}
	})
}

// TestFormatters verifies the WithFormatter option and logger's integration with formatters.
func TestFormatters(t *testing.T) {
	// Test that New() without options uses JSONFormatter
	t.Run("Default Formatter is JSON", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf))
		logger.Infow("json test", "key", "value")

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("expected valid JSON, but got error: %v", err)
		}
	})

	// Test switching to TextFormatter
	t.Run("WithFormatter switches to TextFormatter", func(t *testing.T) {
		// IMPORTANT: Intended for non-TTY environments
		t.Setenv("HARELOG_NO_COLOR", "1")

		var buf bytes.Buffer

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

// TestAutoSource_Modes verifies the behavior of the different SourceLocationMode options.
func TestAutoSource_Modes(t *testing.T) {
	t.Parallel()

	// This helper function captures a log and returns whether the source field exists.
	logAndCheckSourcePresence := func(logger *Logger, level LogLevel, buf *bytes.Buffer) bool {
		buf.Reset()

		switch level {
		case LogLevelInfo:
			logger.Infof("test message")
		case LogLevelError:
			logger.Errorf("test message")
		}

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			// Consider a failure to unmarshal as the field not being present.
			return false
		}
		_, exists := entry["logging.googleapis.com/sourceLocation"]
		return exists
	}

	t.Run("Mode Never", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf), WithAutoSource(SourceLocationModeNever))
		if logAndCheckSourcePresence(logger, LogLevelError, &buf) {
			t.Error("source should NOT be present with ModeNever, even for errors")
		}
	})

	t.Run("Mode Always", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf), WithAutoSource(SourceLocationModeAlways))
		if !logAndCheckSourcePresence(logger, LogLevelInfo, &buf) {
			t.Error("source SHOULD be present with ModeAlways for info logs")
		}
	})

	t.Run("Mode Error or Above", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		logger := New(WithOutput(&buf), WithAutoSource(SourceLocationModeErrorOrAbove))
		if logAndCheckSourcePresence(logger, LogLevelInfo, &buf) {
			t.Error("source should NOT be present with ModeErrorOrAbove for info logs")
		}
		if !logAndCheckSourcePresence(logger, LogLevelError, &buf) {
			t.Error("source SHOULD be present with ModeErrorOrAbove for error logs")
		}
	})

	t.Run("Manual source overrides auto source", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		buf.Reset()
		logger := New(WithOutput(&buf), WithAutoSource(SourceLocationModeAlways))
		manualLocation := &SourceLocation{File: "manual.go", Line: 101}
		logger.Infow("testing override", "sourceLocation", manualLocation)

		var entry map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatalf("failed to unmarshal JSON: %v", err)
		}

		slMap, ok := entry["logging.googleapis.com/sourceLocation"].(map[string]interface{})
		if !ok {
			t.Fatal("manual sourceLocation field should be present")
		}
		if file, _ := slMap["file"].(string); file != "manual.go" {
			t.Errorf("expected manual file to take precedence, got %q", file)
		}
	})
}

// TestNew_WithOptions verifies that all functional options passed to New() are correctly applied.
func TestNew_WithOptions(t *testing.T) {
	t.Parallel()

	t.Run("Default values", func(t *testing.T) {
		l := New()
		if l.out != os.Stderr {
			t.Errorf("expected default output to be os.Stderr, got %v", l.out)
		}
		if l.logLevel != logLevelValueInfo {
			t.Errorf("expected default level to be Info, got %v", l.logLevel)
		}
		if _, ok := l.formatter.(*jsonFormatter); !ok {
			t.Errorf("expected default formatter to be jsonFormatter, got %T", l.formatter)
		}
	})

	t.Run("With all functional options", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		labels := map[string]string{"env": "test"}

		logger := New(
			WithOutput(&buf),
			WithLogLevel(LogLevelDebug),
			WithFormatter(NewTextFormatter()),
			WithAutoSource(SourceLocationModeAlways),
			WithProjectID("test-project"),
			WithTraceContextKey("test-key"),
			WithPrefix("[test] "),
			WithLabels(labels),
			WithFields("common_key", "common_value"),
		)

		if logger.out != &buf {
			t.Error("WithOutput failed")
		}
		if logger.logLevel != logLevelValueDebug {
			t.Error("WithLogLevel failed")
		}
		if _, ok := logger.formatter.(*textFormatter); !ok {
			t.Error("WithFormatter failed")
		}
		if logger.sourceLocationMode != SourceLocationModeAlways {
			t.Error("WithAutoSource failed")
		}
		if logger.projectID != "test-project" {
			t.Error("WithProjectID failed")
		}
		if logger.traceContextKey != "test-key" {
			t.Error("WithTraceContextKey failed")
		}
		if logger.prefix != "[test] " {
			t.Error("WithPrefix failed")
		}
		if logger.labels["env"] != "test" {
			t.Error("WithLabels failed")
		}
		if logger.payload["common_key"] != "common_value" {
			t.Error("WithFields failed")
		}
	})
}

// TestWithLogLevel_Panic verifies that the WithLogLevel option panics on invalid input.
func TestWithLogLevel_Panic(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected New(WithLogLevel) to panic with an invalid level, but it did not")
		}
	}()
	_ = New(WithLogLevel(LogLevel("invalid-level")))
}

// TestSetupLogLevelFromEnv verifies the HARELOG_LEVEL environment variable.
func TestSetupLogLevelFromEnv(t *testing.T) {
	originalStd := std
	defer func() {
		std = originalStd
	}()

	setup := func() {
		std = New()
	}

	t.Run("Valid level set", func(t *testing.T) {
		setup()
		t.Setenv("HARELOG_LEVEL", "DEBUG")
		setupLogLevelFromEnv()
		if std.logLevel != logLevelValueDebug {
			t.Errorf("expected level to be set to DEBUG, but got %v", std.logLevel)
		}
	})

	t.Run("Invalid level set", func(t *testing.T) {
		setup()
		t.Setenv("HARELOG_LEVEL", "INVALID_VALUE")
		setupLogLevelFromEnv()
		if std.logLevel != logLevelValueInfo {
			t.Errorf("expected level to fall back to default INFO, but got %v", std.logLevel)
		}
	})
}

// TestNew_WithOptions verifies that all functional options passed to New() are correctly applied.
// TestWithMethods_API verifies the immutability and correctness of all With... methods.
func TestWithMethods_API(t *testing.T) {
	t.Parallel()

	baseLogger := New()

	t.Run("WithLogLevel", func(t *testing.T) {
		t.Parallel()

		l2 := baseLogger.WithLogLevel(LogLevelDebug)
		if l2 == baseLogger {
			t.Fatal("Expected a new instance")
		}
		if l2.logLevel != logLevelValueDebug {
			t.Error("Change was not applied")
		}
		if baseLogger.logLevel == logLevelValueDebug {
			t.Error("Original logger was mutated")
		}
	})

	t.Run("WithAutoSource", func(t *testing.T) {
		t.Parallel()

		l2 := baseLogger.WithAutoSource(SourceLocationModeAlways)
		if l2 == baseLogger {
			t.Fatal("Expected a new instance")
		}
		if l2.sourceLocationMode != SourceLocationModeAlways {
			t.Error("Change was not applied")
		}
		if baseLogger.sourceLocationMode == SourceLocationModeAlways {
			t.Error("Original logger was mutated")
		}
	})
}

// TestSetDefaultFunctions_API verifies all SetDefault... functions.
func TestSetDefaultFunctions_API(t *testing.T) {
	originalStd := std
	defer func() {
		std = originalStd
	}()

	setup := func() {
		std = New()
	}

	t.Run("SetDefaultLogLevel", func(t *testing.T) {
		setup()
		SetDefaultLogLevel(LogLevelDebug)
		if std.logLevel != logLevelValueDebug {
			t.Error("SetDefaultLogLevel was not applied")
		}
	})

	t.Run("SetDefaultAutoSource", func(t *testing.T) {
		setup()
		SetDefaultAutoSource(SourceLocationModeAlways)
		if std.sourceLocationMode != SourceLocationModeAlways {
			t.Error("SetDefaultAutoSource was not applied")
		}
	})
}

// TestFatalMethods_AlwaysExit verifies that Fatal... methods always call os.Exit.
func TestFatalMethods_AlwaysExit(t *testing.T) {
	t.Parallel()

	getExitCode := mockOsExit(t)

	logger := New(WithOutput(io.Discard), WithLogLevel(LogLevelOff))

	tests := map[string]func(){
		"Fatalf": func() { logger.Fatalf("should exit") },
		"Fatalw": func() { logger.Fatalw("should exit") },
	}

	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			fn()

			if getExitCode() != 1 {
				t.Errorf("%s did not call os.Exit when log level was OFF", name)
			}
		})
	}
}

// TestPanicScenarios verifies that configuration functions panic on invalid input.
func TestPanicScenarios(t *testing.T) {
	t.Parallel()

	t.Run("WithLogLevel option with invalid level", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected New(WithLogLevel) to panic")
			}
		}()
		_ = New(WithLogLevel(LogLevel("invalid")))
	})

	t.Run("WithAutoSource option with invalid mode", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected New(WithAutoSource) to panic")
			}
		}()
		_ = New(WithAutoSource(sourceLocationMode(99)))
	})

	t.Run("WithTraceContextKey option with nil key", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected New(WithTraceContextKey) to panic")
			}
		}()
		_ = New(WithTraceContextKey(nil))
	})

	t.Run("WithFields option with odd arguments", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected New(WithFields) to panic")
			}
		}()
		_ = New(WithFields("key"))
	})

	t.Run("Logger.WithLogLevel method with invalid level", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected (*Logger).WithLogLevel to panic")
			}
		}()
		l := New()
		_ = l.WithLogLevel(LogLevel("invalid"))
	})
}

// mockHook is a simple hook for testing purposes.
// It stores fired entries and allows for synchronization.
type mockHook struct {
	mu      sync.Mutex
	entries []*LogEntry
	levels  []LogLevel
	wg      *sync.WaitGroup
	delay   time.Duration // Optional delay to simulate work
}

// newMockHook creates a new mockHook.
func newMockHook(levels ...LogLevel) *mockHook {
	return &mockHook{
		levels: levels,
		wg:     &sync.WaitGroup{},
	}
}

// Levels returns the log levels this hook is interested in.
func (h *mockHook) Levels() []LogLevel {
	return h.levels
}

// Fire is called when a log entry matches the hook's levels.
func (h *mockHook) Fire(entry *LogEntry) error {
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, entry)
	if h.wg != nil {
		h.wg.Done()
	}
	return nil
}

// FiredEntries returns a copy of the entries this hook has received.
func (h *mockHook) FiredEntries() []*LogEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Return a copy to avoid race conditions
	entriesCopy := make([]*LogEntry, len(h.entries))
	copy(entriesCopy, h.entries)
	return entriesCopy
}

// Reset clears the stored entries.
func (h *mockHook) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = nil
}

// panicHook is a hook that always panics.
type panicHook struct{}

func (h *panicHook) Levels() []LogLevel {
	return []LogLevel{LogLevelError}
}

func (h *panicHook) Fire(entry *LogEntry) error {
	panic("test panic in hook")
}

// --- Hook Tests ---

func TestLogger_Hooks_BasicFiring(t *testing.T) {
	t.Parallel()

	hook := newMockHook(LogLevelError, LogLevelCritical)

	var buf bytes.Buffer

	logger := New(WithOutput(&buf), WithHooks(hook))

	defer logger.Close()

	logger.Infof("This should not be hooked.")
	logger.Warnf("This should not be hooked either.")
	logger.Errorf("This is an error.")
	logger.Criticalf("This is critical.")

	// Wait for hooks to be processed
	time.Sleep(50 * time.Millisecond)

	fired := hook.FiredEntries()
	if len(fired) != 2 {
		t.Fatalf("expected 2 fired entries, got %d", len(fired))
	}

	if fired[0].Severity != LogLevelError || fired[0].Message != "This is an error." {
		t.Errorf("unexpected entry for error log: got %+v", fired[0])
	}
	if fired[1].Severity != LogLevelCritical || fired[1].Message != "This is critical." {
		t.Errorf("unexpected entry for critical log: got %+v", fired[1])
	}
}

// safeBuffer is a thread-safe buffer for concurrent testing.
// It embeds a bytes.Buffer and protects its methods with a mutex.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write implements the io.Writer interface in a thread-safe manner.
func (sb *safeBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

// String returns the contents of the buffer as a string in a thread-safe manner.
func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

func TestLogger_Hooks_PanicRecovery(t *testing.T) {
	t.Parallel()

	hook := &panicHook{}

	var buf safeBuffer

	logger := New(
		WithOutput(&buf),
		WithHooks(hook),
		WithFormatter(NewTextFormatter()),
	)
	defer logger.Close()

	// This log call should not cause the test to panic
	logger.Errorf("This will trigger a panic in the hook.")

	// Give the hook worker a moment to process the panic and log the recovery.
	// このsleepは、非同期処理が完了するのを「保証」するものではありませんが、
	// bufへのアクセス自体がスレッドセーフになったため、ここでの競合は発生しません。
	time.Sleep(50 * time.Millisecond)

	// ★ この .String() 呼び出しがスレッドセーフになりました。
	output := buf.String()
	if !strings.Contains(output, "A hook panicked") {
		t.Errorf("expected output to contain panic recovery message, but it didn't. Output:\n%s", output)
	}
	if !strings.Contains(output, `panic="test panic in hook"`) {
		t.Errorf("expected output to contain the panic message, but it didn't. Output:\n%s", output)
	}
}

func TestLogger_Hooks_GracefulShutdown(t *testing.T) {
	t.Parallel()

	hook := newMockHook(LogLevelInfo)
	hook.delay = 100 * time.Millisecond // This hook is slow
	hook.wg.Add(1)

	logger := New(WithHooks(hook))

	startTime := time.Now()
	logger.Infof("A slow hook will be fired.")

	// This should block until the slow hook is finished.
	err := logger.Close()
	if err != nil {
		t.Fatalf("Close returned an error: %v", err)
	}
	duration := time.Since(startTime)

	if duration < hook.delay {
		t.Errorf("Close did not wait for the hook to finish. Took %v, expected at least %v", duration, hook.delay)
	}

	fired := hook.FiredEntries()
	if len(fired) != 1 {
		t.Errorf("expected hook to have fired, but it didn't")
	}
}

func TestLogger_Hooks_DefaultLogger(t *testing.T) {
	// Restore default logger after test
	originalStd := std
	defer func() {
		stdMutex.Lock()
		std = originalStd
		stdMutex.Unlock()
	}()

	hook := newMockHook(LogLevelError)
	hook.wg.Add(1)

	var buf bytes.Buffer
	SetDefaultOutput(&buf)
	SetDefaultHooks(hook)
	defer Close() // Ensure the default logger's worker is closed

	Errorf("global error log")

	hook.wg.Wait() // Wait for the hook to fire

	fired := hook.FiredEntries()
	if len(fired) != 1 {
		t.Fatalf("expected hook on default logger to fire, got %d entries", len(fired))
	}
	if fired[0].Message != "global error log" {
		t.Errorf("unexpected message from hook: %s", fired[0].Message)
	}

	// --- Test clearing hooks ---
	hook.Reset()
	SetDefaultHooks() // Call with no args to clear hooks

	Warnf("this should not be hooked now")
	Errorf("this should not be hooked now either")

	time.Sleep(50 * time.Millisecond) // Give time for any hooks to (incorrectly) fire

	fired = hook.FiredEntries()
	if len(fired) != 0 {
		t.Fatalf("hooks should have been cleared, but %d entries were fired", len(fired))
	}
}

func TestLogger_Hooks_Inheritance(t *testing.T) {
	t.Parallel()

	hook := newMockHook(LogLevelWarn)
	hook.wg.Add(2)

	parentLogger := New(WithHooks(hook))
	defer parentLogger.Close()

	childLogger := parentLogger.With("child_key", "child_value")
	grandChildLogger := childLogger.Clone()

	parentLogger.Warnf("from parent")
	childLogger.Warnf("from child")
	grandChildLogger.Infof("should not fire")

	hook.wg.Wait()

	fired := hook.FiredEntries()
	if len(fired) != 2 {
		t.Fatalf("expected 2 fired entries from parent and child, got %d", len(fired))
	}

	if fired[0].Message != "from parent" {
		t.Errorf("unexpected first entry message: %s", fired[0].Message)
	}
	if fired[1].Message != "from child" {
		t.Errorf("unexpected second entry message: %s", fired[1].Message)
	}
}

func TestLogger_Hooks_AllLevels(t *testing.T) {
	t.Parallel()

	// A hook with no levels specified should fire for all levels.
	hook := newMockHook()
	hook.wg.Add(5) // For 5 log levels (Critical to Debug)

	logger := New(
		WithHooks(hook),
		WithLogLevel(LogLevelDebug),
	)
	defer logger.Close()

	logger.Criticalf("1")
	logger.Errorf("2")
	logger.Warnf("3")
	logger.Infof("4")
	logger.Debugf("5")

	hook.wg.Wait()

	fired := hook.FiredEntries()
	if len(fired) != 5 {
		t.Fatalf("expected hook to fire for all 5 levels, got %d", len(fired))
	}
}

// Benchmark for a simple formatted log message without any extra fields.
func BenchmarkSimpleLog(b *testing.B) {
	// Setup: Create a logger with options. Discarding output ensures we measure
	// the logger's overhead, not the I/O performance of the writer.
	logger := New(WithOutput(io.Discard))

	// Reset the timer to start the measurement from here.
	// ReportAllocs() enables memory allocation statistics in the output.
	b.ResetTimer()
	b.ReportAllocs()

	// The benchmark loop. The `testing` package automatically determines
	// the number of iterations (b.N) needed to get a stable measurement.
	for i := 0; i < b.N; i++ {
		logger.Infof("simple log message for benchmark, value: %d", i)
	}
}

// Benchmark for a structured log message using the 'w' (with) method.
func BenchmarkLogWithFields(b *testing.B) {
	// Setup
	logger := New(WithOutput(io.Discard))

	// Reset timer and enable memory allocation reporting.
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// The 'w' methods (e.g., Errorw, Infow) are designed for efficient
		// structured logging with key-value pairs. This simulates a realistic
		// logging scenario in an application.
		logger.Errorw("log message with fields for benchmark",
			"service", "harelog-bench",
			"user_id", 12345,
			"is_member", true,
			"request_id", "abc-123-xyz",
		)
	}
}

func BenchmarkFormatOnly(b *testing.B) {
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

func BenchmarkPrintPath(b *testing.B) {
	f := &jsonFormatter{}
	e := &LogEntry{
		Message:  "world",
		Severity: "DEBUG",
		Time:     time.Now(),
		Payload: map[string]any{
			"active": true,
		},
	}
	var mu sync.Mutex

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		mu.Lock()
		f.Format(e)
		mu.Unlock()
	}
}
