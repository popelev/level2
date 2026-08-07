package core

import "strings"

// GuessDataType infers platform type from browse/signal name when OPC metadata is unavailable.
// Used as fallback after DataType Attribute Read fails, and to refine ByteString→datetime.
// Hint may be a short browse name (sUnit) or a full tag id (…_current_sunit).
func GuessDataType(browseName string) ValueType {
	n := strings.ToLower(browseName)
	switch {
	case looksLikeStringName(n):
		return ValueString
	case strings.HasPrefix(n, "b") || strings.Contains(n, "bool") || strings.HasPrefix(n, "enable"):
		return ValueBool
	case strings.HasSuffix(n, "_maintenance") || strings.HasSuffix(n, "_operation"):
		return ValueBool
	case strings.Contains(n, "harvesting"):
		return ValueBool
	case strings.Contains(n, "_mode_") && !strings.Contains(n, "rvalue"):
		return ValueBool
	case strings.HasSuffix(n, "_auto") || strings.HasSuffix(n, "_run") || strings.HasSuffix(n, "_active"):
		return ValueBool
	case looksLikeDateTimeName(n):
		return ValueDateTime
	case strings.HasPrefix(n, "i") || strings.Contains(n, "count"):
		return ValueInt64
	default:
		return ValueFloat64
	}
}

// looksLikeStringName detects Siemens/generic string leaves: sUnit, unit, *_sunit, sName, sText.
// Uses the last path/id segment so full tag ids (…_current_sunit) still match.
func looksLikeStringName(n string) bool {
	n = strings.ToLower(n)
	if strings.Contains(n, "sunit") {
		return true
	}
	base := n
	if i := strings.LastIndexAny(n, "._"); i >= 0 && i+1 < len(n) {
		base = n[i+1:]
	}
	switch base {
	case "unit", "name", "text":
		return true
	}
	// Hungarian notation on the leaf: sUnit, sName, sText.
	if strings.HasPrefix(base, "s") && (strings.Contains(base, "unit") ||
		strings.Contains(base, "name") || strings.Contains(base, "text")) {
		return true
	}
	return false
}

// looksLikeDateTimeName detects OPC DateTime / Siemens DATE_AND_TIME naming
// (e.g. LastCycleDateAndTime — "dateandtime", not "datetime"; leaf "Time" / *_time).
func looksLikeDateTimeName(n string) bool {
	n = strings.ToLower(n)
	if strings.Contains(n, "runtime") || strings.Contains(n, "timeout") ||
		strings.Contains(n, "lifetime") {
		return false
	}
	if strings.Contains(n, "datetime") || strings.Contains(n, "dateandtime") ||
		strings.Contains(n, "date_and_time") || strings.Contains(n, "date_time") ||
		strings.Contains(n, "timestamp") {
		return true
	}
	base := n
	if i := strings.LastIndexAny(n, "._"); i >= 0 && i+1 < len(n) {
		base = n[i+1:]
	}
	if base == "time" || base == "date" {
		return true
	}
	return strings.HasSuffix(n, "_time")
}
