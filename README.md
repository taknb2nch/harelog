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

**Example Output (JSON):**

```json
{"message":"Server is starting...\n","severity":"INFO","timestamp":"..."}
{"message":"Server started on port 8080","severity":"INFO","timestamp":"..."}
{"message":"User logged in","severity":"INFO","userID":"user-123","ipAddress":"127.0.0.1","timestamp":"..."}
```

### Adding Context with the `With` Method (Child Loggers)

You can create a contextual logger (or "child logger") that carries a predefined set of key-value pairs. This is extremely useful for request-scoped logging.

```go
var logger = harelog.New() // Your base logger

func handleRequest(w http.ResponseWriter, r *http.Request) {
    // Create a new child logger with context for this specific request.
    reqLogger := logger.With("requestID", "abc-123", "remoteAddr", r.RemoteAddr)

    reqLogger.Infof("request received")
    reqLogger.Infow("user authenticated", "userID", "user-456")
}
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

## Configuration & Features

`harelog` can be configured using functional options at initialization.

### Setting the Log Level

You can set the initial log level when creating a new logger.

```go
// Create a logger that only outputs DEBUG level logs or higher.
logger := harelog.New(
    harelog.WithLogLevel(harelog.LogLevelDebug),
)
```

### Automatic Source Code Location

For easier debugging, `harelog` can automatically log the file and line number of the log call site. This feature has a performance cost and is configurable via different modes.

```go
// In production, you might only want source location for errors.
logger := harelog.New(
    harelog.WithAutoSource(harelog.SourceLocationModeErrorOrAbove),
)

logger.Infof("This will NOT have source info.")
logger.Errorf("This WILL have source info.")
```

**Example Verification:**

The accuracy of this feature is best verified by running a sample application. For a complete, runnable example, please see the `examples/main.go` file in this repository.

*Expected Output from the example:*
The `sourceLocation` will correctly point to the file and line number of the call site.

```json
{"message":"...","severity":"ERROR","logging.googleapis.com/sourceLocation":{"file":"/path/to/your/project/examples/main.go","line":42,...}}
```

### Output Format (Formatter)

By default, logs are in JSON format. For local development, you can switch to a human-readable text format with "smart" color-coding.

```go
// Use the WithFormatter option to switch to the text logger
logger := harelog.New(
    harelog.WithFormatter(harelog.NewTextFormatter()),
)
logger.Infow("server started", "port", 8080)
```

**Example Text Output (in a terminal):**

```
2025-09-30T22:00:00Z [INFO] server started {port=8080}
```
(Note: The `[INFO]` part will be colorized in a supported terminal.)

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