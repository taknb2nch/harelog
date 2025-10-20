package harelog

func clearOrResetMap[V any](m *map[string]V, threshold int) {
	if m == nil {
		return
	}
	if *m == nil {
		*m = make(map[string]V)
		return
	}
	if len(*m) > threshold {
		*m = make(map[string]V)
	} else {
		clear(*m)
	}
}
