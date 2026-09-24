package harelog

import (
	"testing"
	"time"
)

// benchmarkTime is a fixed time shared across all benchmarks.
var benchmarkTime = time.Date(2025, 9, 30, 14, 0, 0, 0, time.UTC)

// benchmarkEntrySimple is a shared, simple log entry for all "Simple" benchmarks.
// It uses "server-started" (no spaces) to ensure a fair comparison,
// preventing skewed allocations for logfmtFormatter which would otherwise need
// to quote the message.
var benchmarkEntrySimple = &LogEntry{
	Message:  "server-started", // No space, fair to all formatters
	Severity: LogLevelInfo,
	Time:     benchmarkTime,
}

// benchmarkEntryComplex is a shared, complex log entry for all "Complex" benchmarks.
// It includes all special fields (TraceID, SpanID, HTTPRequest, etc.) and
// a payload with multiple data types (string with spaces, float, bool)
// to test quoting and type handling.
var benchmarkEntryComplex = &LogEntry{
	Message:        "complex event", // No space in message
	Severity:       LogLevelWarn,
	Time:           benchmarkTime,
	TraceID:        "trace-id-123",
	SpanID:         "span-id-456",
	CorrelationID:  "corr-id-789",
	Labels:         map[string]string{"region": "jp-east", "cluster": "A"},
	SourceLocation: &SourceLocation{File: "app/server.go", Line: 152},
	HTTPRequest: &HTTPRequest{
		RequestMethod: "POST",
		Status:        401,
		RequestURL:    "/api/v1/login",
	},
	Payload: map[string]interface{}{
		"userID": "user-abc",
		"dept":   "eng department", // ★ Includes space to test quoting logic
		"rate":   123.45,
		"active": true,
		"count":  int64(99),
	},
}

// benchmarkEntryComplexMasking is a shared, complex log entry for "Masking" benchmarks.
// Its content is identical to benchmarkEntryComplex to ensure a fair comparison
// of performance with and without masking enabled.
var benchmarkEntryComplexMasking = &LogEntry{
	Message:        "complex event masking", // Changed message for clarity
	Severity:       LogLevelWarn,
	Time:           benchmarkTime,
	TraceID:        "trace-id-123",
	SpanID:         "span-id-456",
	CorrelationID:  "corr-id-789",
	Labels:         map[string]string{"region": "jp-east", "cluster": "A"},
	SourceLocation: &SourceLocation{File: "app/server.go", Line: 152},
	HTTPRequest: &HTTPRequest{
		RequestMethod: "POST",
		Status:        401,
		RequestURL:    "/api/v1/login",
	},
	Payload: map[string]interface{}{
		"userID": "user-abc",
		"dept":   "eng department",
		"rate":   123.45,
		"active": true,
		"count":  int64(99),
	},
}

// BenchmarkJsonFormatter_Simple benchmarks formatting a simple log entry.
func BenchmarkJsonFormatter_Simple(b *testing.B) {
	f := JSON.NewFormatter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(benchmarkEntrySimple) // Use shared entry
	}
}

// BenchmarkJsonFormatter_Complex benchmarks formatting a complex log entry.
func BenchmarkJsonFormatter_Complex(b *testing.B) {
	f := JSON.NewFormatter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(benchmarkEntryComplex) // Use shared entry
	}
}

// BenchmarkJSONFormatter_Complex_Masking benchmarks a complex entry
// with several masking rules enabled.
func BenchmarkJSONFormatter_Complex_Masking(b *testing.B) {
	f := JSON.NewFormatter(
		JSON.WithMaskingKeys("userID"),
		JSON.WithMaskingKeysIgnoreCase("DEPT"),
		JSON.WithMaskingKeysIgnoreCase("region"),
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(cloneEntry(benchmarkEntryComplexMasking))
	}
}

// BenchmarkTextFormatter_Simple benchmarks formatting a simple log entry.
func BenchmarkTextFormatter_Simple(b *testing.B) {
	f := Text.NewFormatter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// The error is ignored in benchmarks as we test correctness in unit tests.
		_, _ = f.Format(benchmarkEntrySimple) // Use shared entry
	}
}

// BenchmarkTextFormatter_Complex benchmarks formatting a complex log entry.
func BenchmarkTextFormatter_Complex(b *testing.B) {
	f := Text.NewFormatter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(benchmarkEntryComplex) // Use shared entry
	}
}

// BenchmarkTextFormatter_Complex_Masking benchmarks a complex entry
// with several masking rules enabled.
func BenchmarkTextFormatter_Complex_Masking(b *testing.B) {
	f := Text.NewFormatter(
		Text.WithMaskingKeys("userID"),
		Text.WithMaskingKeysIgnoreCase("DEPT"),
		Text.WithMaskingKeysIgnoreCase("region"),
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(cloneEntry(benchmarkEntryComplexMasking))
	}
}

// BenchmarkConsoleFormatter_Simple benchmarks the console formatter with a simple log entry.
func BenchmarkConsoleFormatter_Simple(b *testing.B) {
	f := Console.NewFormatter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(benchmarkEntrySimple) // Use shared entry
	}
}

// BenchmarkConsoleFormatter_Complex benchmarks the console formatter with a complex log entry.
func BenchmarkConsoleFormatter_Complex(b *testing.B) {
	// Highlight options are retained as they are a valid
	// part of the ConsoleFormatter's complex use case.
	f := Console.NewFormatter(
		Console.WithLogLevelColor(true),
		Console.WithKeyHighlight("userID", FgCyan),
		Console.WithKeyHighlight("dept", FgMagenta, AttrBold),
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(benchmarkEntryComplex) // Use shared entry
	}
}

// BenchmarkConsoleFormatter_Complex_Masking benchmarks a complex entry
// with several masking rules enabled.
func BenchmarkConsoleFormatter_Complex_Masking(b *testing.B) {
	f := Console.NewFormatter(
		Console.WithLogLevelColor(true),
		Console.WithKeyHighlight("userID", FgCyan),
		Console.WithKeyHighlight("dept", FgMagenta, AttrBold),
		Console.WithMaskingKeys("userID"),
		Console.WithMaskingKeysIgnoreCase("DEPT"),
		Console.WithMaskingKeysIgnoreCase("region"),
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(cloneEntry(benchmarkEntryComplexMasking))
	}
}

// BenchmarkLogfmtFormatter_Simple benchmarks formatting a simple log entry.
func BenchmarkLogfmtFormatter_Simple(b *testing.B) {
	f := Logfmt.NewFormatter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(benchmarkEntrySimple) // Use shared entry
	}
}

// BenchmarkLogfmtFormatter_Complex benchmarks formatting a complex log entry.
func BenchmarkLogfmtFormatter_Complex(b *testing.B) {
	f := Logfmt.NewFormatter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(benchmarkEntryComplex) // Use shared entry
	}
}

// BenchmarkLogfmtFormatter_Complex_Masking benchmarks a complex entry
// with several masking rules enabled.
func BenchmarkLogfmtFormatter_Complex_Masking(b *testing.B) {
	f := Logfmt.NewFormatter(
		Logfmt.WithMaskingKeys("userID"),
		Logfmt.WithMaskingKeysIgnoreCase("DEPT"),
		Logfmt.WithMaskingKeysIgnoreCase("region"),
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Format(cloneEntry(benchmarkEntryComplexMasking))
	}
}
