package migrate

import (
	"context"
	"testing"
)

func TestCommonHashMethod(t *testing.T) {
	cases := []struct {
		name string
		src  []string
		dst  []string
		want string
	}{
		{"both md5", []string{"md5"}, []string{"md5"}, "md5"},
		{"both sha1", []string{"sha1"}, []string{"sha1"}, "sha1"},
		{"no match", []string{"md5"}, []string{"sha1"}, ""},
		{"multi match", []string{"md5", "sha1"}, []string{"sha1", "md5"}, "md5"},
		{"empty src", []string{}, []string{"md5"}, ""},
	}
	for _, c := range cases {
		got := commonHashMethod(c.src, c.dst)
		if got != c.want {
			t.Errorf("%s: commonHashMethod(%v,%v) = %q, want %q", c.name, c.src, c.dst, got, c.want)
		}
	}
}

func TestEngineCancelUnknown(t *testing.T) {
	e := NewEngine(nil, nil)
	// should not panic on unknown id
	e.Cancel("nonexistent")
}

func TestEngineRunEmptyFileIDs(t *testing.T) {
	e := NewEngine(nil, nil)
	job := &Job{ID: "t1", FileIDs: []string{}}
	err := e.Run(context.Background(), job)
	if err != nil {
		t.Errorf("Run with empty FileIDs: %v", err)
	}
	if job.Status != "completed" {
		t.Errorf("expected completed, got %s", job.Status)
	}
}

func TestJobProcessedBytesTracking(t *testing.T) {
	j := &Job{ID: "t1", Total: 1000}
	j.ProcessedBytes = 500
	if j.ProcessedBytes != 500 {
		t.Error("ProcessedBytes not set")
	}
	j.Failed = 2
	if j.Failed != 2 {
		t.Error("Failed not set")
	}
}
