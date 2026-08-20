package pan139

import (
	"testing"

	"mnemo-go/internal/model"
)

func TestParsePan139QuotaSupportsResponseVariants(t *testing.T) {
	used, total, ok := parsePan139Quota([]byte(`{"usedSize":"12","totalSize":100}`))
	if !ok || used != 12 || total != 100 {
		t.Fatalf("string quota = %d/%d ok=%v, want 12/100 true", used, total, ok)
	}

	used, total, ok = parsePan139Quota([]byte(`{"data":{"used":120,"diskSize":"100"}}`))
	if !ok || used != 100 || total != 100 {
		t.Fatalf("nested clamped quota = %d/%d ok=%v, want 100/100 true", used, total, ok)
	}

	used, total, ok = parsePan139Quota([]byte(`{"freeSize":"80","totalSize":"100"}`))
	if !ok || used != 20 || total != 100 {
		t.Fatalf("derived free quota = %d/%d ok=%v, want 20/100 true", used, total, ok)
	}

	if _, _, ok = parsePan139Quota([]byte(`{"usedSize":"12"}`)); ok {
		t.Fatal("quota without a total size should not be accepted")
	}
}

func TestApplyPan139QuotaPreservesLastKnownValueOnMissingQuota(t *testing.T) {
	token := &model.TokenInfo{UsedSize: 2, TotalSize: 10, FreeSize: 8}
	applyPan139Quota(token, 0, 0)
	if token.UsedSize != 2 || token.TotalSize != 10 || token.FreeSize != 8 {
		t.Fatalf("missing quota replaced last known values: %#v", token)
	}
}
