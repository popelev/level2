package diag

import "testing"

func TestQuery_CategoryAndErrorsOnly(t *testing.T) {
	b := NewBuffer(20)
	b.Add(Entry{Level: LevelInfo, Category: CategoryOPCRead, Message: "i1"})
	b.Add(Entry{Level: LevelError, Category: CategoryOPCRead, Message: "e1"})
	b.Add(Entry{Level: LevelWarn, Category: CategoryDBWrite, Message: "w1"})
	b.Add(Entry{Level: LevelError, Category: CategoryOPCWrite, Message: "e2"})

	all := b.Query("all", false, 10)
	if len(all) != 4 {
		t.Fatalf("all=%d", len(all))
	}
	errs := b.Query("all", true, 10)
	// errorsOnly includes warn + error
	if len(errs) != 3 {
		t.Fatalf("errors=%#v", errs)
	}
	opc := b.Query(CategoryOPCRead, false, 10)
	if len(opc) != 2 {
		t.Fatalf("opc read %#v", opc)
	}
	opcErr := b.Query(CategoryOPCRead, true, 10)
	if len(opcErr) != 1 || opcErr[0].Message != "e1" {
		t.Fatalf("%#v", opcErr)
	}
	if got := NormalizeCategory(" OPC_READ "); got != CategoryOPCRead {
		t.Fatalf("normalize %q", got)
	}
	if got := NormalizeCategory("write"); got != CategoryOPCWrite {
		t.Fatalf("write alias %q", got)
	}
	b.Clear()
	if len(b.Query("all", false, 10)) != 0 {
		t.Fatal("cleared")
	}
}
