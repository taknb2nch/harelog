# harelog [![Go](https://github.com/taknb2nch/harelog/actions/workflows/go.yaml/badge.svg?branch=main)](https://github.com/taknb2nch/harelog/actions/workflows/go.yaml)

A simple Go logger for Google Cloud

---

## Installation

```bash
go get github.com/taknb2nch/harelog
```

---

## Usage

### Formatted Logging (`...f` series)

Outputs simple logs using a `printf`-style format.

```go
import "github.com/taknb2nch/harelog"

harelog.Infof("Server started on port %d", 8080)
```

**Example output:**

```json
{"message":"Server stared on port 8080","severity":"INFO","timestamp":"..."}
```

### Structured Logging (`...w` series)

You can add more detailed information to logs as key-value pairs. This is also how you add special fields for Google Cloud Logging.

```go
import (
    "errors"
    "github.com/taknb2nch/harelog"
)

func someFunction() {
    err := errors.New("failed to connect to database")
    sl := &harelog.SourceLocation{File: "app.go", Line: 123}

    harelog.Errorw("processing failed",
        "error", err,
        "userID", "user-abc",
        "sourceLocation", sl, // for Google Cloud Logging
    )
}
```

**Example output:**

```json
{"message":"processing failed","severity":"ERROR","error":"failed to connect to database","userID":"user-abc","[logging.googleapis.com/sourceLocation](https://logging.googleapis.com/sourceLocation)":{"file":"app.go","line":123},"timestamp":"..."}
```

### Request-Scoped Logger

The `With...` methods allow you to create a new logger instance with context, such as a trace ID.

```go
// Create a request-specific logger with a trace ID
reqLogger := harelog.WithTrace(traceID)
reqLogger.Infof("request processing started")
```

---

## Special Fields

When you provide the following keys to a `...w` function or method, the logger interprets them in a special way.

| Key | Type | Description |
| :--- | :--- | :--- |
| `error` | `error` | An error object. Its message is automatically added to the log. |
| `httpRequest` | `*harelog.HTTPRequest` | **For Google Cloud Logging:** HTTP request information. |
| `sourceLocation` | `*harelog.SourceLocation` | **For Google Cloud Logging:** Source code location information. |

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
