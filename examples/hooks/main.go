package main

import (
	"fmt"
	"os"
	"time"

	"github.com/taknb2nch/harelog"
)

// SimpleHook is a custom hook that writes to standard error.
type SimpleHook struct {
	// You can add fields here, e.g., a writer or a client for an external service.
}

// Levels specifies that this hook should only fire for Error and Fatal level logs.
func (h *SimpleHook) Levels() []harelog.LogLevel {
	return []harelog.LogLevel{harelog.LogLevelError, harelog.LogLevelCritical}
}

// Fire is the action to be taken when a log event matching the levels occurs.
// It formats a message and writes it to stderr.
func (h *SimpleHook) Fire(entry *harelog.LogEntry) error {
	// In a real hook, you would send this entry to a service like Sentry or Slack.
	// For this example, we just print it to stderr to show it's working.
	fmt.Fprintf(os.Stderr, "[HOOK] An event occurred at level %s: %s\n", entry.Severity, entry.Message)

	return nil
}

func main() {
	// 1. Create an instance of our custom hook.
	myHook := &SimpleHook{}

	// 2. Create a new logger and register the hook using the WithHooks option.
	// We use the TextFormatter for clear, human-readable output.
	logger := harelog.New(
		harelog.WithFormatter(harelog.NewTextFormatter()),
		harelog.WithHooks(myHook),
	)

	// 3. IMPORTANT: Defer the Close() call.
	// This ensures the hook worker has time to process all buffered logs before the program exits.
	defer func() {
		fmt.Println("Closing logger...")
		if err := logger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing logger: %v\n", err)
		}
		fmt.Println("Logger closed.")
	}()

	fmt.Println("--- Logging examples ---")

	// This log is below the hook's level, so the hook will NOT fire.
	logger.Infof("This is an informational message.")

	// This log is also below the hook's level, so the hook will NOT fire.
	logger.Warnf("This is a warning message.")

	// This log matches the hook's level, so the hook WILL fire.
	logger.Errorf("This is an error message! The hook should capture this.")

	// Give the hook worker a moment to process the log.
	// In a real application, you wouldn't need this sleep.
	time.Sleep(100 * time.Millisecond)

	fmt.Println("------------------------")
}
