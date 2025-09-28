# harelog [![Go](https://github.com/taknb2nch/harelog/actions/workflows/go.yaml/badge.svg?branch=main)](https://github.com/taknb2nch/harelog/actions/workflows/go.yaml)

A simple and flexible Go logger for Google Cloud, with powerful context handling.

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

**Example Output:**

```json
{"message":"Server is starting...\n","severity":"INFO","timestamp":"..."}
{"message":"Server started on port 8080","severity":"INFO","timestamp":"..."}
{"message":"User logged in","severity":"INFO","userID":"user-123","ipAddress":"127.0.0.1","timestamp":"..."}
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

### Automatic Tracing with `context.Context` (`...Ctx` methods)

For seamless integration with distributed tracing systems like Google Cloud Trace, you can use the `...Ctx` variants of the logging methods. `harelog` can automatically extract trace information from a `context.Context`.

#### 1. Configuration

First, configure your logger with your Google Cloud Project ID and the context key your application uses to store the trace header.

```go
// In your application's setup (e.g., main.go)
const frameworkTraceKey = "x-cloud-trace-context" 

logger := harelog.New(
    harelog.WithProjectID("my-gcp-project-id"),
    harelog.WithTraceContextKey(frameworkTraceKey),
)
```

#### 2. Logging with Context

Now, simply pass the request's context to any `...Ctx` method.

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    // The request context `r.Context()` typically contains the trace header.
    logger.InfofCtx(r.Context(), "handling request")
}
```

**Example Output:**

```json
{"message":"handling request","severity":"INFO","logging.googleapis.com/trace":"projects/my-gcp-project-id/traces/...", ...}
```

---

## Customizing Output with Formatters

By default, `harelog` outputs logs in JSON format. You can easily switch to a human-readable text format for local development using the `WithFormatter` option.

```go
// Use the WithFormatter option to switch to the text logger
logger := harelog.New(
    harelog.WithFormatter(harelog.NewTextFormatter()),
)
logger.Infow("server started", "port", 8080)
```

**Example Text Output:**

```
2025-09-27T08:50:00Z [INFO] server started {port=8080}
```

### Colored Text Output

The `TextFormatter` provides "smart" color-coding: it is automatically enabled when writing to an interactive terminal (TTY) and disabled when writing to a file or pipe. You can also control it explicitly.

```go
// Force color to be enabled or disabled
formatter := harelog.NewTextFormatter(
    harelog.WithColor(true), // or false
)
logger := harelog.New(harelog.WithFormatter(formatter))
```

### Setting the Default Log Level via Environment Variable

You can control the default logger's verbosity without changing code by setting the `HARELOG_LEVEL` environment variable.

```bash
HARELOG_LEVEL=debug go run main.go
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