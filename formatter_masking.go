package harelog

import "strings"

// maskingCore holds the logic for storing and checking sensitive keys.
// This struct is intended to be embedded in formatters.
type maskingCore struct {
	sensitiveKeys   map[string]struct{}
	insensitiveKeys map[string]struct{}
}

// addSensitive adds one or more keys for case-sensitive matching.
func (mc *maskingCore) addSensitive(keys ...string) {
	if mc.sensitiveKeys == nil {
		mc.sensitiveKeys = make(map[string]struct{})
	}

	for _, k := range keys {
		mc.sensitiveKeys[k] = struct{}{}
	}
}

// addInsensitive adds one or more keys for case-insensitive matching.
// The keys are stored in lower-case for efficient lookup.
func (mc *maskingCore) addInsensitive(keys ...string) {
	if mc.insensitiveKeys == nil {
		mc.insensitiveKeys = make(map[string]struct{})
	}

	for _, k := range keys {
		mc.insensitiveKeys[strings.ToLower(k)] = struct{}{}
	}
}

// isMasking checks if the given key should be masked.
// It performs a zero-cost check first if no keys are registered.
// It checks sensitive keys first, then falls back to insensitive keys.
func (mc *maskingCore) isMasking(key string) bool {
	if len(mc.sensitiveKeys) == 0 && len(mc.insensitiveKeys) == 0 {
		return false
	}

	if _, ok := mc.sensitiveKeys[key]; ok {
		return true
	}

	if len(mc.insensitiveKeys) > 0 {
		lowerKey := strings.ToLower(key)

		if _, ok := mc.insensitiveKeys[lowerKey]; ok {
			return true
		}
	}

	return false
}
