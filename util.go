package harelog

import "strings"

// charsRequiringQuoting defines the set of characters that generally require
// quoting when used in unquoted keys or values in simple key=value formats (like logfmt).
// This includes space, =, and ".
const charsRequiringQuoting = " =\""

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

// isValidKey checks if the given key contains any characters
// defined in charsRequiringQuoting. It also considers an empty key invalid.
// This function helps enforce a stricter, safer convention for keys.
func isValidKey(key string) bool {
	if key == "" {
		return false
	}

	if strings.ContainsAny(key, charsRequiringQuoting) {
		return false
	}

	return true
}

// needsQuoting checks if the given string value contains any characters
// defined in charsRequiringQuoting or is empty, thus requiring quoting.
func needsQuoting(value string) bool {
	if value == "" {
		return true
	}

	return strings.ContainsAny(value, charsRequiringQuoting)
}
