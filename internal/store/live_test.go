package store

import (
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
)

func TestLive_PollAvgMs(t *testing.T) {
	l := NewLive()
	n := 1.0
	l.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	time.Sleep(20 * time.Millisecond)
	l.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})
	time.Sleep(20 * time.Millisecond)
	l.Update(core.Sample{TagID: "t1", Time: time.Now().UTC(), ValueNum: &n, Quality: core.QualityGood})

	out := l.SnapshotDevices([]core.Device{{
		ID: "d",
		Tags: []core.Tag{{ID: "t1", NodeID: "ns=4;i=1", DataType: core.ValueFloat64}},
	}})
	if len(out) != 1 || out[0].PollAvgMs == nil || *out[0].PollAvgMs < 10 {
		t.Fatalf("got %#v", out[0].PollAvgMs)
	}
}
