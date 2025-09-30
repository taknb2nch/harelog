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
	if _, ok := l.formatter.(*jsonFormatter); !ok {
		t.Errorf("expected default formatter to be jsonFormatter, got %T", l.formatter)
	}
}

// TestSetupLogLevelFromEnv verifies that the default log level is correctly
// configured from the HARELOG_LEVEL environment variable.
func TestSetupLogLevelFromEnv(t *testing.T) {
	// Save and restore the original std logger state to avoid affecting other tests.
	originalStd := std
	defer func() {
		std = originalStd
	}()

	// setup helper resets std to a clean logger for each subtest
	setup := func() {
		std = New() // Reset to a known default state (INFO level)
	}

	t.Run("Variable not set", func(t *testing.T) {
		setup()
		// Ensure the variable is unset for this test.
		t.Setenv("HARELOG_LEVEL", "")

		setupLogLevelFromEnv()

		if std.logLevel != logLevelValueInfo {
			t.Errorf("expected level to remain default INFO, but got %v", std.logLevel)
		}
	})

	t.Run("Valid level set", func(t *testing.T) {
		setup()
		// t.Setenv automatically handles restoring the original value after the test.
		t.Setenv("HARELOG_LEVEL", "DEBUG")

		setupLogLevelFromEnv()

		if std.logLevel != logLevelValueDebug {
			t.Errorf("expected level to be set to DEBUG, but got %v", std.logLevel)
		}
	})

	t.Run("Invalid level set", func(t *testing.T) {
		setup()
		t.Setenv("HARELOG_LEVEL", "INVALID_VALUE")

		// We can capture the warning log for verification if needed, but for now,
		// we'll just check that the log level was not changed.
		// originalLogOutput := log.Writer()
		// defer log.SetOutput(originalLogOutput)
		// var buf bytes.Buffer
		// log.SetOutput(&buf)

		setupLogLevelFromEnv()

		if std.logLevel != logLevelValueInfo {
			t.Errorf("expected level to fall back to default INFO, but got %v", std.logLevel)
		}
		// if !strings.Contains(buf.String(), "invalid HARELOG_LEVEL") {
		// 	t.Error("expected a warning to be logged for an invalid level")
		// }
	})
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

// TestWithMethod verifies the functionality of the contextual logger.
func TestWithMethod(t *testing.T) {
	var buf bytes.Buffer

	// This helper resets the buffer for each subtest.
	setup := func() {
		buf.Reset()
	}

	t.Run("Context is added to logs", func(t *testing.T) {
		setup()
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
		setup()
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
		setup()
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
		setup()
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
		setup()
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
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected With to panic with an odd number of arguments, but it did not")
			}
		}()
		logger := New()
		_ = logger.With("key1", "value1", "key2")
	})

	t.Run("Panics on non-string key", func(t *testing.T) {
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

// TestFatalwMethod verifies the Fatalw method.
func TestFatalwMethod(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf))

	// Mock os.Exit to prevent test termination
	var exitCode int
	originalExit := osExit
	osExit = func(code int) {
		exitCode = code
	}
	defer func() { osExit = originalExit }()

	// Call the method to be tested
	l.Fatalw("database connection failed", "host", "localhost", "port", 5432)

	// 1. Verify that os.Exit(1) was called
	if exitCode != 1 {
		t.Errorf("expected os.Exit(1) to be called, but exit code was %d", exitCode)
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
	var buf bytes.Buffer

	// This helper resets the buffer for each subtest.
	setup := func() {
		buf.Reset()
	}

	// Define a custom context key for testing, mimicking how real applications do it.
	type contextKey string
	const traceContextKey = contextKey("x-cloud-trace-context")

	t.Run("Values are extracted from context with ProjectID", func(t *testing.T) {
		setup()
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
		setup()
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
		setup()
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
		setup()
		logger := New(WithOutput(&buf))
		ctx := context.Background()

		var exitCode int
		originalExit := osExit
		osExit = func(code int) { exitCode = code }
		defer func() { osExit = originalExit }()

		logger.FatalCtx(ctx, "fatal message from ctx")

		if !strings.Contains(buf.String(), `"message":"fatal message from ctx"`) {
			t.Errorf("FatalCtx did not log the correct message. Got: %s", buf.String())
		}
		if exitCode != 1 {
			t.Errorf("expected os.Exit(1) to be called from FatalCtx, but exit code was %d", exitCode)
		}
	})
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

// TestAutoSource_Modes verifies the behavior of the different SourceLocationMode options.
func TestAutoSource_Modes(t *testing.T) {
	var buf bytes.Buffer

	// This helper function captures a log and returns whether the source field exists.
	logAndCheckSourcePresence := func(logger *Logger, level logLevel) bool {
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
		logger := New(WithOutput(&buf), WithAutoSource(SourceLocationModeNever))
		if logAndCheckSourcePresence(logger, LogLevelError) {
			t.Error("source should NOT be present with ModeNever, even for errors")
		}
	})

	t.Run("Mode Always", func(t *testing.T) {
		logger := New(WithOutput(&buf), WithAutoSource(SourceLocationModeAlways))
		if !logAndCheckSourcePresence(logger, LogLevelInfo) {
			t.Error("source SHOULD be present with ModeAlways for info logs")
		}
	})

	t.Run("Mode Error or Above", func(t *testing.T) {
		logger := New(WithOutput(&buf), WithAutoSource(SourceLocationModeErrorOrAbove))
		if logAndCheckSourcePresence(logger, LogLevelInfo) {
			t.Error("source should NOT be present with ModeErrorOrAbove for info logs")
		}
		if !logAndCheckSourcePresence(logger, LogLevelError) {
			t.Error("source SHOULD be present with ModeErrorOrAbove for error logs")
		}
	})

	t.Run("Manual source overrides auto source", func(t *testing.T) {
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
