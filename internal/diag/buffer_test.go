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

func TestNormalizeCategory(t *testing.T) {
	cases := map[string]string{
		"":          "all",
		"all":       "all",
		"ALL":       "all",
		"opc_read":  CategoryOPCRead,
		"opc":       CategoryOPCRead,
		"OPC":       CategoryOPCRead,
		"opc-read":  CategoryOPCRead,
		"db_write":  CategoryDBWrite,
		"db":        CategoryDBWrite,
		"DB-WRITE":  CategoryDBWrite,
		"opc_write": CategoryOPCWrite,
		"write":     CategoryOPCWrite,
		"other":     "other",
	}
	for in, want := range cases {
		if got := NormalizeCategory(in); got != want {
			t.Fatalf("NormalizeCategory(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBuffer_QueryCategoryAliases(t *testing.T) {
	b := NewBuffer(20)
	b.Add(Entry{Level: LevelWarn, Category: CategoryOPCRead, Message: "poll failed"})
	b.Add(Entry{Level: LevelInfo, Category: CategoryOPCRead, Message: "opc poll ok"})
	b.Add(Entry{Level: LevelInfo, Category: CategoryDBWrite, Message: "wrote batch", Count: 3})

	opc := b.Query("opc", false, 10)
	if len(opc) != 2 {
		t.Fatalf("opc alias: got %d entries %#v", len(opc), opc)
	}
	for _, e := range opc {
		if e.Category != CategoryOPCRead {
			t.Fatalf("unexpected category %q", e.Category)
		}
	}

	db := b.Query("db", false, 10)
	if len(db) != 1 || db[0].Category != CategoryDBWrite {
		t.Fatalf("db alias: %#v", db)
	}

	all := b.Query("all", false, 10)
	if len(all) != 3 {
		t.Fatalf("all: got %d", len(all))
	}
}
