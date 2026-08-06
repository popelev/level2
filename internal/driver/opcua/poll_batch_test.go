package opcua

import "testing"

func TestChunkRanges(t *testing.T) {
	t.Parallel()
	if got := chunkRanges(0, 100); got != nil {
		t.Fatalf("empty n: %#v", got)
	}
	if got := chunkRanges(10, 0); got != nil {
		t.Fatalf("bad size: %#v", got)
	}
	got := chunkRanges(250, 100)
	want := [][2]int{{0, 100}, {100, 200}, {200, 250}}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d]=%v want %v", i, got[i], want[i])
		}
	}
	single := chunkRanges(50, 100)
	if len(single) != 1 || single[0] != [2]int{0, 50} {
		t.Fatalf("single %#v", single)
	}
	// ~3212 tags -> 33 batches of <=100 (same as production scale).
	n := 3212
	chunks := chunkRanges(n, maxNodesPerRead)
	if len(chunks) != 33 {
		t.Fatalf("3212/100 batches=%d want 33", len(chunks))
	}
	last := chunks[len(chunks)-1]
	if last[1] != n || last[1]-last[0] != 12 {
		t.Fatalf("last chunk %#v", last)
	}
}

func TestPollReadConcurrency(t *testing.T) {
	t.Parallel()
	cases := []struct {
		batches, limit, want int
	}{
		{0, 4, 0},
		{1, 4, 1},
		{33, 1, 1},
		{33, 4, 4},
		{3, 4, 3},
		{33, 0, 1},
		{33, -1, 1},
	}
	for _, tc := range cases {
		if got := pollReadConcurrency(tc.batches, tc.limit); got != tc.want {
			t.Fatalf("batches=%d limit=%d got=%d want=%d", tc.batches, tc.limit, got, tc.want)
		}
	}
}