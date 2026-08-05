package core

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsedNodeID is a resolved OPC UA node identifier.
type ParsedNodeID struct {
	Namespace      uint16
	IdentifierType string // i, s, g, b
	Identifier     string
}

// ParseNodeID parses strings like "ns=4;i=4208" or "ns=3;s=Foo.Bar".
func ParseNodeID(raw string) (ParsedNodeID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedNodeID{}, fmt.Errorf("empty node id")
	}
	parts := strings.Split(raw, ";")
	var ns uint16
	var idType, id string
	hasNS := false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case strings.HasPrefix(p, "ns="):
			n, err := strconv.ParseUint(p[3:], 10, 16)
			if err != nil {
				return ParsedNodeID{}, fmt.Errorf("invalid namespace in %q: %w", raw, err)
			}
			ns = uint16(n)
			hasNS = true
		case strings.HasPrefix(p, "i="):
			idType = "i"
			id = p[2:]
		case strings.HasPrefix(p, "s="):
			idType = "s"
			id = p[2:]
		case strings.HasPrefix(p, "g="):
			idType = "g"
			id = p[2:]
		case strings.HasPrefix(p, "b="):
			idType = "b"
			id = p[2:]
		default:
			return ParsedNodeID{}, fmt.Errorf("unsupported node id part %q in %q", p, raw)
		}
	}
	if !hasNS {
		return ParsedNodeID{}, fmt.Errorf("missing namespace in %q", raw)
	}
	if idType == "" || id == "" {
		return ParsedNodeID{}, fmt.Errorf("missing identifier in %q", raw)
	}
	if idType == "i" {
		if _, err := strconv.ParseUint(id, 10, 32); err != nil {
			return ParsedNodeID{}, fmt.Errorf("invalid numeric identifier in %q: %w", raw, err)
		}
	}
	return ParsedNodeID{Namespace: ns, IdentifierType: idType, Identifier: id}, nil
}

// MaxStringBytes truncates long OPC strings for storage safety.
const MaxStringBytes = 4 * 1024

func TruncateString(s string) (string, bool) {
	if len(s) <= MaxStringBytes {
		return s, false
	}
	return s[:MaxStringBytes], true
}
