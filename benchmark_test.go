package harelog

import "testing"

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
