package harelog

// Hook is an interface that allows you to process log entries.
// Hooks can be used to send logs to external services like Sentry or Slack.
//
// The Fire method will be called asynchronously in a dedicated goroutine,
// so it does not block the application's logging calls.
// Implementations of Fire must be safe for concurrent use if the hook instance is shared.
type Hook interface {
	// Levels returns the log levels that this hook should be fired for.
	// If an empty slice is returned, the hook will be fired for all levels.
	Levels() []LogLevel

	// Fire is called when a log entry is created for a level that the hook
	// is configured for. The received logEntry is a defensive copy, so modifications
	// to it will not affect other hooks or the main log output.
	Fire(entry *LogEntry) error
}
