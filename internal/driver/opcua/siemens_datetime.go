package opcua

import (
	"fmt"
	"time"
)

// siemensLDT decodes S7-1500 LDT (nanoseconds since Unix epoch, big-endian).
func siemensLDT(b []byte) (time.Time, error) {
	if len(b) != 8 {
		return time.Time{}, fmt.Errorf("LDT needs 8 bytes")
	}
	var nano int64
	for _, x := range b {
		nano = (nano << 8) | int64(x)
	}
	// Also try little-endian (OPC UA arrays sometimes LE).
	le := int64(uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56)
	for _, n := range []int64{nano, le} {
		if n == 0 {
			continue
		}
		tm := time.Unix(0, n).UTC()
		// Plausible industrial range.
		if tm.Year() >= 1990 && tm.Year() <= 2100 {
			return tm, nil
		}
	}
	return time.Time{}, fmt.Errorf("LDT out of range")
}

// siemensDateAndTime decodes S7 DATE_AND_TIME (DT) — 8 BCD bytes.
// Layout matches gos7 Helper.GetDateTimeAt / Siemens DT online format.
func siemensDateAndTime(b []byte) (time.Time, error) {
	if len(b) != 8 {
		return time.Time{}, fmt.Errorf("siemens DT needs 8 bytes, got %d", len(b))
	}
	// Reject clearly non-BCD nibbles early (values > 9).
	for i := 0; i < 6; i++ {
		if (b[i]>>4) > 9 || (b[i]&0x0F) > 9 {
			return time.Time{}, fmt.Errorf("non-BCD byte at %d: %02x", i, b[i])
		}
	}
	year := decodeBCD(b[0])
	if year < 90 {
		year += 2000
	} else {
		year += 1900
	}
	month := decodeBCD(b[1])
	day := decodeBCD(b[2])
	hour := decodeBCD(b[3])
	min := decodeBCD(b[4])
	sec := decodeBCD(b[5])
	msec := decodeBCD(b[6])*10 + int(b[7]>>4)
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || min > 59 || sec > 59 || msec > 999 {
		return time.Time{}, fmt.Errorf("invalid siemens DT fields y=%d m=%d d=%d %02d:%02d:%02d.%03d hex=%x",
			year, month, day, hour, min, sec, msec, b)
	}
	return time.Date(year, time.Month(month), day, hour, min, sec, msec*1_000_000, time.UTC), nil
}

func decodeBCD(b byte) int {
	return int((b>>4)*10 + (b & 0x0F))
}
