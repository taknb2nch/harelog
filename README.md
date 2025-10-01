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

### Output Format (Formatter) & Color

By default, logs are in JSON format. For local development, you can switch to a human-readable text format with "smart" color-coding (enabled by default for terminals).

```go
// Use the WithFormatter option to switch to the text logger
logger := harelog.New(
    harelog.WithFormatter(harelog.NewTextFormatter()),
)

// You can also explicitly control color
colorFormatter := harelog.NewTextFormatter(harelog.WithColor(true))
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