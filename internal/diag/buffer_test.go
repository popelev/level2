package diag

import "testing"

func TestBuffer_QueryErrorsOnly(t *testing.T) {
	b := NewBuffer(10)
	b.Add(Entry{Level: LevelInfo, Category: CategoryOPCRead, Message: "ok"})
	b.Add(Entry{Level: LevelWarn, Category: CategoryOPCRead, Message: "fail"})
	b.Add(Entry{Level: LevelInfo, Category: CategoryDBWrite, Message: "wrote", Count: 5})

	got := b.Query(CategoryOPCRead, true, 10)
	if len(got) != 1 || got[0].Message != "fail" {
		t.Fatalf("got %#v", got)
	}
}
