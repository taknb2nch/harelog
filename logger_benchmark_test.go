package harelog

import (
	"io"
	"testing"
)

// Benchmark for a simple formatted log message without any extra fields.
func BenchmarkSimpleLog(b *testing.B) {
	// Setup: Create a logger with options. Discarding output ensures we measure
	// the logger's overhead, not the I/O performance of the writer.
	logger := New(WithOutput(io.Discard))

	// Reset the timer to start the measurement from here.
	// ReportAllocs() enables memory allocation statistics in the output.

	b.ReportAllocs()

	// The benchmark loop. The `testing` package automatically determines
	// the number of iterations (b.N) needed to get a stable measurement.
	for i := 0; b.Loop(); i++ {
		logger.Infof("simple log message for benchmark, value: %d", i)
	}
}

// Benchmark for a structured log message using the 'w' (with) method.
func BenchmarkLogWithFields(b *testing.B) {
	// Setup
	logger := New(WithOutput(io.Discard))

	// Reset timer and enable memory allocation reporting.

	b.ReportAllocs()

	for b.Loop() {
		// The 'w' methods (e.g., Errorw, Infow) are designed for efficient
		// structured logging with key-value pairs. This simulates a realistic
		// logging scenario in an application.
		logger.Errorw("log message with fields for benchmark",
			"service", "harelog-bench",
			"user_id", 12345,
			"is_member", true,
			"request_id", "abc-123-xyz",
		)
	}
}
