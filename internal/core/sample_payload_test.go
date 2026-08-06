package core

import (
	"testing"
	"time"
)

func TestSample_SamePayload(t *testing.T) {
	n1, n2 := 1.5, 1.5
	n3 := 2.0
	s1 := "a"
	s2 := "a"
	s3 := "b"
	bTrue, bFalse := true, false

	base := Sample{
		Time:     time.Unix(1, 0).UTC(),
		TagID:    "t1",
		ValueNum: &n1,
		Quality:  QualityGood,
	}
	sameTimeDiff := Sample{
		Time:     time.Unix(99, 0).UTC(),
		TagID:    "other",
		ValueNum: &n2,
		Quality:  QualityGood,
	}
	if !base.SamePayload(sameTimeDiff) {
		t.Fatal("time/tag_id must not affect payload equality")
	}

	changedNum := base
	changedNum.ValueNum = &n3
	if base.SamePayload(changedNum) {
		t.Fatal("different ValueNum should not match")
	}

	changedQ := base
	changedQ.Quality = QualityBad
	if base.SamePayload(changedQ) {
		t.Fatal("quality change should not match")
	}

	textA := Sample{ValueText: &s1, Quality: QualityGood}
	textB := Sample{ValueText: &s2, Quality: QualityGood}
	textC := Sample{ValueText: &s3, Quality: QualityGood}
	if !textA.SamePayload(textB) {
		t.Fatal("equal ValueText should match")
	}
	if textA.SamePayload(textC) {
		t.Fatal("different ValueText should not match")
	}

	boolA := Sample{ValueBool: &bTrue, Quality: QualityGood}
	boolB := Sample{ValueBool: &bTrue, Quality: QualityGood}
	boolC := Sample{ValueBool: &bFalse, Quality: QualityGood}
	if !boolA.SamePayload(boolB) {
		t.Fatal("equal ValueBool should match")
	}
	if boolA.SamePayload(boolC) {
		t.Fatal("different ValueBool should not match")
	}

	nilNum := Sample{Quality: QualityGood}
	withNum := Sample{ValueNum: &n1, Quality: QualityGood}
	if nilNum.SamePayload(withNum) {
		t.Fatal("nil vs non-nil ValueNum should not match")
	}
}
