package core

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsedNodeID is a resolved OPC UA node identifier.
type ParsedNodeID struct {
	Namespace      uint16
	NamespaceURI   string // when set (nsu=...), resolve index via OPC session
	IdentifierType string // i, s, g, b
	Identifier     string
}

// ParseNodeID parses "ns=4;i=4208", "ns=3;s=Foo.Bar", or "nsu=http://Tankhouse_Data_2;i=2880".
func ParseNodeID(raw string) (ParsedNodeID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedNodeID{}, fmt.Errorf("empty node id")
	}
	// Shorthand from some OPC stacks / legacy API responses (namespace 0).
	if !strings.HasPrefix(raw, "ns=") && !strings.HasPrefix(raw, "nsu=") {
		if strings.HasPrefix(raw, "i=") || strings.HasPrefix(raw, "s=") ||
			strings.HasPrefix(raw, "g=") || strings.HasPrefix(raw, "b=") {
			raw = "ns=0;" + raw
		}
	}

	idSep, idType := findIdentSep(raw)
	if idSep < 0 {
		return ParsedNodeID{}, fmt.Errorf("missing identifier in %q", raw)
	}
	prefix := strings.TrimSpace(raw[:idSep])
	id := strings.TrimSpace(raw[idSep+3:]) // skip ;x=
	if id == "" {
		return ParsedNodeID{}, fmt.Errorf("missing identifier in %q", raw)
	}

	out := ParsedNodeID{IdentifierType: idType, Identifier: id}
	switch {
	case strings.HasPrefix(prefix, "ns="):
		n, err := strconv.ParseUint(prefix[3:], 10, 16)
		if err != nil {
			return ParsedNodeID{}, fmt.Errorf("invalid namespace in %q: %w", raw, err)
		}
		out.Namespace = uint16(n)
	case strings.HasPrefix(prefix, "nsu="):
		uri := strings.TrimSpace(prefix[4:])
		if uri == "" {
			return ParsedNodeID{}, fmt.Errorf("empty namespace uri in %q", raw)
		}
		out.NamespaceURI = uri
	default:
		return ParsedNodeID{}, fmt.Errorf("missing namespace in %q", raw)
	}

	if idType == "i" {
		if _, err := strconv.ParseUint(id, 10, 32); err != nil {
			return ParsedNodeID{}, fmt.Errorf("invalid numeric identifier in %q: %w", raw, err)
		}
	}
	return out, nil
}

func findIdentSep(raw string) (int, string) {
	for _, kind := range []string{"i", "s", "g", "b"} {
		sep := ";" + kind + "="
		if i := strings.Index(raw, sep); i >= 0 {
			return i, kind
		}
	}
	return -1, ""
}

// MaxStringBytes truncates long OPC strings for storage safety.
const MaxStringBytes = 4 * 1024

func TruncateString(s string) (string, bool) {
	if len(s) <= MaxStringBytes {
		return s, false
	}
	return s[:MaxStringBytes], true
}
