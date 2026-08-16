package sync

import (
	"testing"
)

func TestGuardDeleteThreshold(t *testing.T) {
	eng := NewEngine(nil)
	// guardDelete protects against deleting more than 50% of snapshot entries.
	// With a snapshot of 10 entries, deleting 4 should be allowed, 6 blocked.
	// 4 deletions (< 50%) → allowed
	if !eng.guardDelete("t1", 4, 10) {
		t.Error("guardDelete(10,4) should allow")
	}
	// 6 deletions (> 50%) → blocked
	if eng.guardDelete("t1", 6, 10) {
		t.Error("guardDelete(10,6) should block")
	}
	// exactly 50% → allowed (ratio > 0.5 blocked, 0.5 is not > 0.5)
	if !eng.guardDelete("t1", 5, 10) {
		t.Error("guardDelete(10,5) should allow (ratio==0.5 not >0.5)")
	}
	// empty snapshot → block
	if eng.guardDelete("t1", 1, 0) {
		t.Error("guardDelete(0,1) should block")
	}
}

func TestConfigFields(t *testing.T) {
	cfg := Config{ID: "t1", Enabled: true, IntervalMin: 10, DeletePropagation: true}
	if cfg.IntervalMin != 10 {
		t.Error("IntervalMin not set")
	}
	if !cfg.DeletePropagation {
		t.Error("DeletePropagation not set")
	}
}
