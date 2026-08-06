package core

import "testing"

func TestNormalizePollConcurrency(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want int
	}{
		{0, DefaultPollConcurrency},
		{-3, DefaultPollConcurrency},
		{1, 1},
		{4, 4},
		{16, 16},
		{17, MaxPollConcurrency},
		{100, MaxPollConcurrency},
	}
	for _, tc := range cases {
		if got := NormalizePollConcurrency(tc.in); got != tc.want {
			t.Fatalf("in=%d got=%d want=%d", tc.in, got, tc.want)
		}
	}
}
