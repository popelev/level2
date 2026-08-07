package core

import (
	"strconv"
	"strings"
)

// OPC UA built-in DataType NodeIds (ns=0). Kept as literals so core stays free of gopcua.
const (
	opcIDBoolean       uint32 = 1
	opcIDSByte         uint32 = 2
	opcIDByte          uint32 = 3
	opcIDInt16         uint32 = 4
	opcIDUInt16        uint32 = 5
	opcIDInt32         uint32 = 6
	opcIDUInt32        uint32 = 7
	opcIDInt64         uint32 = 8
	opcIDUInt64        uint32 = 9
	opcIDFloat         uint32 = 10
	opcIDDouble        uint32 = 11
	opcIDString        uint32 = 12
	opcIDDateTime      uint32 = 13
	opcIDByteString    uint32 = 15
	opcIDXMLElement    uint32 = 16
	opcIDLocalizedText uint32 = 21
	opcIDUtcTime       uint32 = 294
)

// MapOPCTypeID maps an OPC UA DataType NodeId IntID (ns=0) to a platform ValueType.
// Returns "" for unmapped / vendor types (caller may refine via name heuristics).
func MapOPCTypeID(id uint32) ValueType {
	switch id {
	case opcIDBoolean:
		return ValueBool
	case opcIDSByte, opcIDInt16, opcIDInt32, opcIDInt64:
		return ValueInt64
	case opcIDByte, opcIDUInt16, opcIDUInt32, opcIDUInt64:
		return ValueUint
	case opcIDFloat, opcIDDouble:
		return ValueFloat64
	case opcIDString, opcIDByteString, opcIDXMLElement, opcIDLocalizedText:
		// String / CharArray-as-String / LocalizedText / XMLElement.
		// ByteString is also how Siemens often exposes DATE_AND_TIME; refined by name.
		return ValueString
	case opcIDDateTime, opcIDUtcTime: // DateTime=i=13, UtcTime=i=294 (subtype)
		return ValueDateTime
	default:
		return ""
	}
}

// MapOPCTypeCode maps plant Excel / NodeId-style type codes ("i=10", "ns=0;i=1", "10")
// onto platform ValueType. Empty or unmapped → "".
func MapOPCTypeCode(code string) ValueType {
	id, ok := parseOPCTypeCode(code)
	if !ok {
		return ""
	}
	return MapOPCTypeID(id)
}

// MapOPCTypeName maps OPC / Excel type display names (Float, Boolean, Int16, …)
// onto platform ValueType via NormalizeValueType aliases. Empty or unknown → "".
func MapOPCTypeName(name string) ValueType {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return ""
	}
	dt := NormalizeValueType(ValueType(n))
	if ValidValueType(dt) {
		return NormalizeValueType(dt)
	}
	return ""
}

// OPCTypeCode returns the canonical plant Excel DataType code (i=N) for export.
func OPCTypeCode(dt ValueType) string {
	switch NormalizeValueType(dt) {
	case ValueBool:
		return "i=1"
	case ValueInt64:
		return "i=4"
	case ValueUint:
		return "i=7"
	case ValueString:
		return "i=12"
	case ValueDateTime:
		return "i=13"
	default:
		return "i=10"
	}
}

func parseOPCTypeCode(code string) (uint32, bool) {
	s := strings.TrimSpace(strings.ToLower(code))
	if s == "" {
		return 0, false
	}
	// Accept "ns=0;i=10", "i=10", or bare "10".
	if i := strings.LastIndex(s, "i="); i >= 0 {
		s = s[i+2:]
	}
	if j := strings.IndexAny(s, "; \t"); j >= 0 {
		s = s[:j]
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}
