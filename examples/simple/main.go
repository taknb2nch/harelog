// examples/main.go
package main

import (
	"context"
	"time"

	"github.com/taknb2nch/harelog"
)

// main is a simple application to manually verify all features of harelog.
func main() {
	// ---
	println("\n--- 1. Testing Default JSON Formatter (no options) ---")
	// ---
	defaultLogger := harelog.New()
	defaultLogger.Infow("This is the default JSON output.")

	// ---
	println("\n--- 2. Testing TextFormatter (with smart color detection) ---")
	println("NOTE: Run this in a terminal to see colors. Redirect to a file to disable them.")
	// ---
	textLogger := harelog.New(
		harelog.WithFormatter(harelog.Text.NewFormatter()),
	)

	textLogger = textLogger.WithLogLevel(harelog.LogLevelAll)

	textLogger.Debugw("This is a debug message.")
	textLogger.Infow("This is an info message.")
	textLogger.Warnw("This is a warning message.")
	textLogger.Errorw("This is an error message.")
	textLogger.Criticalw("This is a critical message.")

	// ---
	println("\n--- 3. Testing Auto Source Location Modes ---")
	// ---
	// Mode: Always
	println("\n[Mode: Always] Should show source for both INFO and ERROR.")
	loggerAlways := harelog.New(harelog.WithAutoSource(harelog.SourceLocationModeAlways))
	loggerAlways.Infof("This INFO log should have source.")
	loggerAlways.Errorf("This ERROR log should have source.")

	// ---
	println("\n--- 4. Verifying Source Location with TextFormatter ---")
	println("NOTE: This is the feature we worked hard on!")
	// ---
	loggerTextWithSource := harelog.New(
		harelog.WithAutoSource(harelog.SourceLocationModeAlways),
		harelog.WithFormatter(harelog.Text.NewFormatter()),
	)
	// This log should contain the file and line number in a readable format.
	loggerTextWithSource.Warnf("This text log should contain the source location.")
	helperFunction(loggerTextWithSource)

	// Mode: OnError
	println("\n[Mode: OnError] Should show source for ERROR only.")
	loggerOnError := harelog.New(harelog.WithAutoSource(harelog.SourceLocationModeErrorOrAbove))
	loggerOnError.Infof("This INFO log should NOT have source.")
	loggerOnError.Errorf("This ERROR log SHOULD have source.")

	// ---
	println("\n--- 5. Testing Contextual Logger (With method) ---")
	// ---
	baseLogger := harelog.New(harelog.WithFormatter(harelog.Text.NewFormatter()))
	// Create a child logger with request-specific context.
	reqLogger := baseLogger.With("requestID", "abc-123", "user", "gopher")
	reqLogger.Infow("Request processed.", "status", 200)
	reqLogger.Warnf("Upstream service took %dms", 250)

	// ---
	println("\n--- 6. Testing context.Context Integration (Ctx methods) ---")
	// ---
	// The key your web framework would use to store the trace header.
	const traceHeaderKey = "x-cloud-trace-context"
	ctxLogger := harelog.New(
		harelog.WithProjectID("my-gcp-project-id"),
		harelog.WithTraceContextKey(traceHeaderKey),
	)
	// Simulate a context that has the trace header value.
	ctx := context.WithValue(context.Background(), traceHeaderKey, "my-trace-id-from-ctx/my-span-id;o=1")
	ctxLogger.InfofCtx(ctx, "This log should contain trace and span info from the context.")

	// Sleep briefly to ensure all logs are flushed if they were asynchronous.
	time.Sleep(10 * time.Millisecond)
}

func helperFunction(l *harelog.Logger) {
	l.ErrorwCtx(context.Background(), "This log is from a helper function.")
}
