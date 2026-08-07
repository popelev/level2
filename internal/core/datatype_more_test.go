package core

import "testing"

func TestGuessDataType_Branches(t *testing.T) {
	cases := []struct {
		name string
		want ValueType
	}{
		{"sUnit", ValueString},
		{"tank_current_sunit", ValueString},
		{"bEnable", ValueBool},
		{"boolFlag", ValueBool},
		{"enablePump", ValueBool},
		{"motor_maintenance", ValueBool},
		{"line_operation", ValueBool},
		{"harvesting_ok", ValueBool},
		{"x_mode_y", ValueBool},
		{"x_mode_rvalue", ValueFloat64}, // contains mode but also rvalue → not bool branch
		{"pump_auto", ValueBool},
		{"motor_run", ValueBool},
		{"heater_active", ValueBool},
		{"LastCycleDateAndTime", ValueDateTime},
		{"foo_datetime", ValueDateTime},
		{"date_and_time_x", ValueDateTime},
		{"date_time_stamp", ValueDateTime},
		{"my_timestamp", ValueDateTime},
		{"sensor_time", ValueDateTime},
		{"date", ValueDateTime},
		{"time", ValueDateTime},
		{"iCount", ValueInt64},
		{"item_count", ValueInt64},
		{"rValueOut", ValueFloat64},
		{"runtime", ValueFloat64}, // datetime lookalike excluded
	}
	for _, tc := range cases {
		if got := GuessDataType(tc.name); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestLooksLikeDateTimeName_Edges(t *testing.T) {
	yes := []string{"datetime", "dateandtime", "date_and_time", "date_time", "timestamp", "foo.time", "bar_date", "x_time"}
	for _, n := range yes {
		if !looksLikeDateTimeName(n) {
			t.Fatalf("want true: %q", n)
		}
	}
	no := []string{"runtime", "timeout", "lifetime", "rvalue", "count"}
	for _, n := range no {
		if looksLikeDateTimeName(n) {
			t.Fatalf("want false: %q", n)
		}
	}
}

func TestOPCTypeCode_All(t *testing.T) {
	cases := []struct {
		dt   ValueType
		want string
	}{
		{ValueBool, "i=1"},
		{ValueInt64, "i=4"},
		{ValueUint, "i=7"},
		{ValueString, "i=12"},
		{ValueDateTime, "i=13"},
		{ValueFloat64, "i=10"},
		{ValueType("float"), "i=10"},
		{ValueType("nope"), "i=10"},
		{"", "i=10"},
	}
	for _, tc := range cases {
		if got := OPCTypeCode(tc.dt); got != tc.want {
			t.Fatalf("%q → %q want %q", tc.dt, got, tc.want)
		}
	}
}

func TestNormalizePollConcurrency_Clamp(t *testing.T) {
	if got := NormalizePollConcurrency(0); got != DefaultPollConcurrency {
		t.Fatalf("0 → %d", got)
	}
	if got := NormalizePollConcurrency(-3); got != DefaultPollConcurrency {
		t.Fatalf("neg → %d", got)
	}
	if got := NormalizePollConcurrency(1); got != 1 {
		t.Fatalf("1 → %d", got)
	}
	if got := NormalizePollConcurrency(16); got != 16 {
		t.Fatalf("16 → %d", got)
	}
	if got := NormalizePollConcurrency(99); got != MaxPollConcurrency {
		t.Fatalf("99 → %d", got)
	}
}
