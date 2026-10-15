package main

import (
	"strings"
	"testing"
	"time"
)

func TestCalculateGoldenSignals(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	samples := []Sample{{Timestamp: base, LatencyMS: 10, Status: 200, CPUPct: 20, MemoryBytes: 100, QueueDepth: 1}, {Timestamp: base.Add(time.Second), LatencyMS: 50, Status: 500, CPUPct: 60, MemoryBytes: 200, QueueDepth: 3}}
	m := Calculate(samples)
	if m.P50MS != 10 || m.P99MS != 50 || m.ErrorCount != 1 || m.MaxCPU != 60 || m.MaxMemoryBytes != 200 || m.MaxQueueDepth != 3 || m.RequestsPerSecond != 2 {
		t.Fatalf("unexpected metrics: %#v", m)
	}
}

func TestReadSamplesRejectsBadTimestamp(t *testing.T) {
	if _, err := ReadSamples(strings.NewReader(`{"timestamp":"tomorrow"}`)); err == nil {
		t.Fatal("expected timestamp error")
	}
}
