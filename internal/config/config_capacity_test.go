package config

import "testing"

func TestValidateCapacityPolicy(t *testing.T) {
	p, pol, err := ValidateCapacityPolicy(80, "drop_oldest")
	if err != nil || p != 80 || pol != FullPolicyDropOldest {
		t.Fatalf("got %d %q err=%v", p, pol, err)
	}
	if _, _, err := ValidateCapacityPolicy(0, "stop"); err == nil {
		t.Fatal("expected error for percent 0")
	}
	if _, _, err := ValidateCapacityPolicy(50, "nope"); err == nil {
		t.Fatal("expected error for bad policy")
	}
	_, pol, err = ValidateCapacityPolicy(10, "")
	if err != nil || pol != FullPolicyStop {
		t.Fatalf("empty policy should default to stop, got %q err=%v", pol, err)
	}
}

func TestNormalizeDatabaseCapacityDefaults(t *testing.T) {
	db := Database{}
	if err := normalizeDatabaseCapacity(&db); err != nil {
		t.Fatal(err)
	}
	if db.CapacityPercent != 90 || db.FullPolicy != FullPolicyStop {
		t.Fatalf("%+v", db)
	}
}
