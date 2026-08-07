package core

import "strings"

// NormalizeValueType maps aliases (e.g. plant Excel date_time) to platform types.
func NormalizeValueType(dt ValueType) ValueType {
	switch strings.ToLower(strings.TrimSpace(string(dt))) {
	case "date_time", "datetim", "timestamp", "utctime":
		return ValueDateTime
	case "bool", "boolean":
		return ValueBool
	case "int", "int16", "int32", "int64", "sbyte", "integer":
		return ValueInt64
	case "uint", "uint16", "uint32", "uint64", "byte", "unsigned", "unsignedint":
		return ValueUint
	case "float", "float32", "float64", "double":
		return ValueFloat64
	case "string", "bytestring", "localizedtext", "xmlelement":
		return ValueString
	case "datetime":
		return ValueDateTime
	default:
		s := strings.ToLower(strings.TrimSpace(string(dt)))
		switch ValueType(s) {
		case ValueBool, ValueInt64, ValueUint, ValueFloat64, ValueString, ValueDateTime:
			return ValueType(s)
		}
		return dt
	}
}

// ValidValueType reports whether dt is a supported tag datatype after normalization.
func ValidValueType(dt ValueType) bool {
	switch NormalizeValueType(dt) {
	case ValueBool, ValueInt64, ValueUint, ValueFloat64, ValueString, ValueDateTime:
		return true
	default:
		return false
	}
}
