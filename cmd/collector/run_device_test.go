package main

import (
	"context"
	"testing"
	"time"

	"github.com/popelev/level2/internal/core"
	"github.com/popelev/level2/internal/driver/mock"
	opcuaDriver "github.com/popelev/level2/internal/driver/opcua"
)

func TestRunDevice_EmptyOPCTagsThenCancel(t *testing.T) {
	log := testLog()
	dev := testDevice("plc", core.Tag{
		ID: "sim", NodeID: "ns=4;i=1", DataType: core.ValueFloat64,
		Enabled: true, Simulate: true, IntervalMs: 100,
	})
	cfg := testConfig(t, dev)
	drv := opcuaDriver.New(dev, log)
	drv.SetConnectedForTest(true)

	samples := make(chan core.Sample, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDevice(ctx, log, cfg, "plc", drv, samples)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDevice did not exit on empty-tags path")
	}
}

func TestRunDevice_SubscribeThenCancel(t *testing.T) {
	log := testLog()
	dev := testDevice("plc", testTag("t", "ns=4;i=1"))
	cfg := testConfig(t, dev)
	drv := opcuaDriver.New(dev, log)
	drv.SetConnectedForTest(true)

	samples := make(chan core.Sample, 8)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDevice(ctx, log, cfg, "plc", drv, samples)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("runDevice did not exit after subscribe cancel")
	}
}

func TestRunDevice_ConnectBackoffThenCancel(t *testing.T) {
	log := testLog()
	dev := core.Device{
		ID: "plc", Endpoint: "opc.tcp://127.0.0.1:1", Security: "None",
		Tags: []core.Tag{testTag("t", "ns=4;i=1")},
	}
	cfg := testConfig(t, dev)
	drv := opcuaDriver.New(dev, log)

	samples := make(chan core.Sample, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDevice(ctx, log, cfg, "plc", drv, samples)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runDevice did not exit during connect backoff")
	}
}

func TestRunDevice_MissingDeviceTags(t *testing.T) {
	log := testLog()
	cfg := testConfig(t)
	drv := opcuaDriver.New(core.Device{ID: "ghost", Endpoint: "opc.tcp://127.0.0.1:1"}, log)
	drv.SetConnectedForTest(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		runDevice(ctx, log, cfg, "ghost", drv, make(chan core.Sample, 1))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runDevice should exit when DeviceTags fails")
	}
}

func TestRunDevice_ConfigReloadWhileEmpty(t *testing.T) {
	log := testLog()
	dev := testDevice("plc")
	cfg := testConfig(t, dev)
	drv := opcuaDriver.New(dev, log)
	drv.SetConnectedForTest(true)

	samples := make(chan core.Sample, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		runDevice(ctx, log, cfg, "plc", drv, samples)
		close(done)
	}()
	if err := cfg.UpsertTag("plc", core.Tag{
		ID: "sim", NodeID: "ns=4;i=9", DataType: core.ValueBool,
		Enabled: true, Simulate: true, IntervalMs: 50,
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDevice did not exit after config reload")
	}
}

func TestRunDevice_SubscribeErrorThenBackoff(t *testing.T) {
	log := testLog()
	dev := testDevice("plc", core.Tag{
		ID: "off", NodeID: "ns=4;i=1", DataType: core.ValueFloat64,
		Enabled: false, IntervalMs: 100,
	})
	cfg := testConfig(t, dev)
	drv := opcuaDriver.New(dev, log)
	drv.SetConnectedForTest(true)

	samples := make(chan core.Sample, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDevice(ctx, log, cfg, "plc", drv, samples)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runDevice did not exit after subscribe error backoff")
	}
}

func TestRunDevice_BadNodeSubscribeError(t *testing.T) {
	log := testLog()
	dev := testDevice("plc", core.Tag{
		ID: "bad", NodeID: "not-a-node", DataType: core.ValueFloat64,
		Enabled: true, IntervalMs: 50,
	})
	cfg := testConfig(t, dev)
	drv := opcuaDriver.New(dev, log)
	drv.SetConnectedForTest(true)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDevice(ctx, log, cfg, "plc", drv, make(chan core.Sample, 2))
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runDevice bad-node path stuck")
	}
}

func TestRunDemo_AllTagsAndCancel(t *testing.T) {
	log := testLog()
	cfg := testConfig(t, testDevice("plc", core.Tag{
		ID: "a", NodeID: "ns=1;i=1", DataType: core.ValueFloat64,
		Enabled: true, IntervalMs: 40,
	}))
	demo := mock.NewDemo(20 * time.Millisecond)
	if err := demo.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	samples := make(chan core.Sample, 16)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDemo(ctx, log, demo, cfg, samples, true)
		close(done)
	}()
	deadline := time.Now().Add(3 * time.Second)
	got := false
	for time.Now().Before(deadline) {
		select {
		case <-samples:
			got = true
		default:
			time.Sleep(10 * time.Millisecond)
		}
		if got {
			break
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDemo did not exit")
	}
	if !got {
		t.Fatal("expected demo samples with allTags=true")
	}
}

func TestRunDemo_CancelWhileEmpty(t *testing.T) {
	log := testLog()
	cfg := testConfig(t)
	demo := mock.NewDemo(50 * time.Millisecond)
	_ = demo.Connect(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDemo(ctx, log, demo, cfg, make(chan core.Sample, 1), false)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDemo empty path did not exit")
	}
}

func TestRunDemo_ConfigReloadDuringSubscribe(t *testing.T) {
	log := testLog()
	cfg := testConfig(t, testDevice("plc", core.Tag{
		ID: "sim1", NodeID: "ns=1;i=1", DataType: core.ValueFloat64,
		Enabled: true, Simulate: true, IntervalMs: 40,
	}))
	demo := mock.NewDemo(20 * time.Millisecond)
	_ = demo.Connect(context.Background())
	samples := make(chan core.Sample, 16)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runDemo(ctx, log, demo, cfg, samples, false)
		close(done)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-samples:
			goto reloaded
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	t.Fatal("no initial demo sample")
reloaded:
	if err := cfg.UpsertTag("plc", core.Tag{
		ID: "sim2", NodeID: "ns=1;i=2", DataType: core.ValueBool,
		Enabled: true, Simulate: true, IntervalMs: 40,
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDemo did not exit after reload")
	}
}
