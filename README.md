# harelog [![Go](https://github.com/taknb2nch/harelog/actions/workflows/go.yaml/badge.svg?branch=main)](https://github.com/taknb2nch/harelog/actions/workflows/go.yaml)

A simple and flexible Go logger for Google Cloud, with powerful context handling and developer-friendly output.

---

## Installation

```bash
go get github.com/taknb2nch/harelog
```

---

## Usage

### Basic & Structured Logging

`harelog` provides familiar functions for different logging styles.

```go
import "github.com/taknb2nch/harelog"

// Simple logging (compatible with standard log package)
harelog.Println("Server is starting...")

// Formatted logging
harelog.Infof("Server started on port %d", 8080)

// Structured logging with key-value pairs
harelog.Infow("User logged in",
    "userID", "user-123",
    "ipAddress", "127.0.0.1",
)
```

### Adding Context with the `With` Method (Child Loggers)

You can create a contextual logger (or "child logger") that carries a predefined set of key-value pairs. This is extremely useful for request-scoped logging, as you don't need to repeat fields like a `requestID` in every log call.

```go
var logger = harelog.New() // Your base logger

func handleRequest(w http.ResponseWriter, r *http.Request) {
    // Create a new child logger with context for this specific request.
    // The base logger is not modified.
    reqLogger := logger.With("requestID", "abc-123", "remoteAddr", r.RemoteAddr)

    reqLogger.Infof("request received")
    reqLogger.Infow("user authenticated", "userID", "user-456")
}
```

**Example Output from `reqLogger`:**

The `requestID` and `remoteAddr` fields are automatically added to all logs.

```json
{"message":"request received","severity":"INFO","requestID":"abc-123","remoteAddr":"127.0.0.1:12345",...}
{"message":"user authenticated","severity":"INFO","userID":"user-456","requestID":"abc-123","remoteAddr":"127.0.0.1:12345",...}
```

### Logging with `context.Context` (`...Ctx` methods)

For integration with tracing systems, you can use the `...Ctx` variants of the logging methods. `harelog` can automatically extract trace information from a `context.Context` (see Configuration section for setup).

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    // The request context `r.Context()` typically contains the trace header.
    logger.InfofCtx(r.Context(), "handling request")
}
```

---

## Configuration

`harelog` provides a consistent and flexible API for configuration through three main patterns: Functional Options for `New()`, `With...` methods for deriving loggers, and `SetDefault...` functions for the global logger.

The most common way to configure a logger is at initialization using functional options.

```go
// Example of a fully configured logger
logger := harelog.New(
    harelog.WithOutput(os.Stdout),
    harelog.WithLogLevel(harelog.LogLevelDebug),
    harelog.WithFormatter(harelog.NewTextFormatter()),
    harelog.WithAutoSource(harelog.SourceLocationModeAlways),
    harelog.WithPrefix("[app] "),
    harelog.WithLabels(map[string]string{"service": "api"}),
    harelog.WithFields("version", "v1.5.0"),
)
```

### Automatic Source Code Location

For easier debugging, `harelog` can automatically log the file and line number of the log call site. This feature has a performance cost and is configurable via different modes.

```go
// In production, you might only want source location for errors.
prodLogger := harelog.New(
    harelog.WithAutoSource(harelog.SourceLocationModeErrorOrAbove),
)

prodLogger.Infof("This will NOT have source info.")
prodLogger.Errorf("This WILL have source info.")
```

### Output Formatters

`harelog` provides multiple formatters to suit different environments. The default is the `JSONFormatter`, ideal for production and log collection systems. For development, you can choose a more human-readable format.

#### TextFormatter

The `TextFormatter` provides a simple, single-line text output. By default, it automatically enables color-coding for log levels when outputting to a terminal.

```go
// Use the WithFormatter option to switch to the text logger.
logger := harelog.New(
    harelog.WithFormatter(harelog.NewTextFormatter()),
)

// You can also explicitly control the log level coloring.
textFormatter := harelog.NewTextFormatter(harelog.WithTextLevelColor(true))
```

#### ConsoleFormatter (for Development)

For the ultimate developer experience, the `ConsoleFormatter` extends the `TextFormatter` with the ability to **highlight specific key-value pairs**. This makes it incredibly easy to spot important information like a `userID` or `traceID` in a sea of logs.

```go
// Use the ConsoleFormatter to highlight important keys.
formatter := harelog.NewConsoleFormatter(
    // Enable coloring for log levels (e.g., [INFO] in green).
    harelog.WithConsoleLevelColor(true),
    
    // Define your highlight rules.
    harelog.WithKeyHighlight("userID", harelog.FgCyan, harelog.AttrBold),
    harelog.WithKeyHighlight("requestID", harelog.FgMagenta),
    harelog.WithKeyHighlight("error", harelog.FgRed, harelog.AttrUnderline),
)

logger := harelog.New(harelog.WithFormatter(formatter))

logger.Errorw("Failed to connect to database",
    "userID", "user-789",
    "requestID", "req-ghi-333",
    "error", "connection refused",
)
```

### Default Log Level via Environment Variable

You can control the default logger's verbosity by setting the `HARELOG_LEVEL` environment variable.

```bash
HARELOG_LEVEL=debug go run main.go
```

### Configuring for Google Cloud Trace

To enable automatic trace extraction from a `context.Context`, you must provide a Project ID and the context key your application uses.

```go
const frameworkTraceKey = "x-cloud-trace-context" 

logger := harelog.New(
    harelog.WithProjectID("my-gcp-project-id"),
    harelog.WithTraceContextKey(frameworkTraceKey),
)
```

---

## Extending with Hooks

Hooks provide a powerful way to extend `harelog`'s functionality, turning it into a logging platform. You can use hooks to send log entries to external services like Sentry, Slack, or a custom database based on the log level.

Hook execution is fully **asynchronous** and **panic-safe**, meaning a slow or faulty hook will never impact your application's performance or stability.

### Implementing a Custom Hook

To create a hook, simply implement the `harelog.Hook` interface.

```go
// simple_hook.go
package main

import (
	"fmt"
	"github.com/taknb2nch/harelog"
)

// SimpleHook is a custom hook that prints to Stderr for specific levels.
type SimpleHook struct{}

// Levels specifies that this hook should only fire for Error and Critical logs.
func (h *SimpleHook) Levels() []harelog.LogLevel {
	return []harelog.LogLevel{harelog.LogLevelError, harelog.LogLevelCritical}
}

// Fire is the action to be taken when a log event matches the levels.
func (h *SimpleHook) Fire(entry *harelog.LogEntry) error {
	// Here you would typically send the entry to an external service.
	// For this example, we'll just print it.
	fmt.Fprintf(os.Stderr, "[HOOK] %s: %s\n", entry.Severity, entry.Message)
	return nil
}
```

### Configuring Hooks

Register your custom hook at initialization using the `WithHooks` option.

**Important:** Because hooks run in the background, you must call `logger.Close()` (or `harelog.Close()` for the default logger) to ensure all buffered hook events are sent before your application exits. Using `defer` is the recommended approach.

```go
// main.go
package main

import (
	"github.com/taknb2nch/harelog"
)

func main() {
	// Create an instance of your custom hook.
	myHook := &SimpleHook{}

	// Register the hook with a new logger.
	logger := harelog.New(harelog.WithHooks(myHook))

	// Ensure graceful shutdown for the hook worker.
	defer logger.Close()

	logger.Info("This will not trigger the hook.")
	logger.Error("This ERROR will trigger the hook!")
}
```

---

## Special Fields

When you provide the following keys to a `...w` function or the `With` method, the logger interprets them in a special way.

| Key | Type | Description |
| :--- | :--- | :--- |
| `error` | `error` | An error object. Its message is automatically added to the log. |
| `httpRequest` | `*harelog.HTTPRequest` | **For Google Cloud Logging:** HTTP request information. |
| `sourceLocation` | `*harelog.SourceLocation` | **For Google Cloud Logging:** Source code location information. |

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.