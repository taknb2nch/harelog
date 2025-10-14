// This program demonstrates the advanced features of the ConsoleFormatter.
package main

import (
	"os"

	"github.com/taknb2nch/harelog"
)

func main() {
	// 1. Create a new ConsoleFormatter with custom highlight rules.
	// The API is designed to be intuitive and type-safe, using functional options.
	formatter := harelog.NewConsoleFormatter(
		// Enable log level coloring (e.g., INFO in green, ERROR in red).
		harelog.WithConsoleLevelColor(true),

		// Define highlight rules for specific keys.
		// You can chain as many WithKeyHighlight options as you need.
		harelog.WithKeyHighlight("userID", harelog.FgCyan, harelog.AttrBold),
		harelog.WithKeyHighlight("requestID", harelog.FgMagenta),
		harelog.WithKeyHighlight("error", harelog.FgRed, harelog.AttrUnderline),
		harelog.WithKeyHighlight("status", harelog.FgGreen),
	)

	// 2. Create a new logger with our custom formatter.
	logger := harelog.New(
		harelog.WithOutput(os.Stdout),
		harelog.WithFormatter(formatter),
	)

	// 3. Log some messages to see the colorful and readable output!
	logger.Infow("User logged in successfully",
		"userID", "user-123",
		"requestID", "req-abc-111",
		"ipAddress", "192.168.1.1",
	)

	logger.Warnw("Payment processing is slow",
		"userID", "user-456",
		"requestID", "req-def-222",
		"duration", "2.5s",
	)

	logger.Errorw("Failed to connect to database",
		"userID", "user-789",
		"requestID", "req-ghi-333",
		"error", "connection refused",
		"host", "db.example.com",
	)

	// Example with an HTTPRequest struct
	req := &harelog.HTTPRequest{
		RequestMethod: "GET",
		RequestURL:    "/api/users/user-123",
		Status:        200,
		Latency:       "150ms",
	}
	logger.Infow("API request handled",
		"userID", "user-123",
		"requestID", "req-jkl-444",
		"httpRequest", req,
		"status", req.Status, // The "status" key will be highlighted in green.
	)
}
