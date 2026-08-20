package migrate

import (
	"context"
	"testing"

	"mnemo-go/internal/model"
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

func TestRapidUploadAllowedExcludesYike(t *testing.T) {
	if rapidUploadAllowed(model.ProviderYike, "pan123") {
		t.Fatal("yike must not be used as a rapid-upload source")
	}
	if rapidUploadAllowed("pan123", model.ProviderYike) {
		t.Fatal("yike must not be used as a rapid-upload target")
	}
	if !rapidUploadAllowed("pan123", "pan189") {
		t.Fatal("compatible providers should remain eligible")
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

func TestValidateEndpointsRejectsSameDrive(t *testing.T) {
	if err := ValidateEndpoints("pikpak_user", "pikpak:drive", "pikpak_user", "pikpak:drive"); err == nil {
		t.Fatal("same source and target drive must be rejected")
	}
	if err := ValidateEndpoints("pikpak_user", "pikpak:drive", "pikpak_other", "pikpak:drive"); err != nil {
		t.Fatalf("different accounts should remain valid: %v", err)
	}
}

func TestEngineRunRejectsSameDriveBeforeChangingState(t *testing.T) {
	e := NewEngine(nil, nil)
	job := &Job{
		ID: "same-drive", SrcUser: "user", SrcDrive: "drive",
		DstUser: "user", DstDrive: "drive", FileIDs: []string{"file"},
	}
	if err := e.Run(context.Background(), job); err == nil {
		t.Fatal("same-drive migration should fail before provider access")
	}
	if job.Status != "" {
		t.Fatalf("job status changed after validation failure: %q", job.Status)
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

func TestPartialMigrationErrorUnwrapsThroughMigrationError(t *testing.T) {
	err := newMigrationError(partialError("source cleanup failed"), 1)
	if !isPartialError(err) {
		t.Fatal("partial cleanup errors must remain identifiable after wrapping")
	}
	if failureCount(err) != 1 {
		t.Fatalf("failure count = %d, want 1", failureCount(err))
	}
}
