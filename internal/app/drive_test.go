package app

import (
	"testing"

	"mnemo-go/internal/model"
)

func TestShouldPersistShareHistory(t *testing.T) {
	if shouldPersistShareHistory(&model.ShareItem{SharePolicy: "presigned"}) {
		t.Fatal("presigned URL must not be persisted as share history")
	}
	if !shouldPersistShareHistory(&model.ShareItem{SharePolicy: "public"}) {
		t.Fatal("provider-managed share should be persisted")
	}
}

func TestValidateShareRecordProvider(t *testing.T) {
	if err := validateShareRecordProvider(model.ShareHistoryEntry{}, model.ProviderDropbox); err != nil {
		t.Fatalf("legacy share record should remain usable: %v", err)
	}
	if err := validateShareRecordProvider(model.ShareHistoryEntry{Provider: model.ProviderDropbox}, model.ProviderDropbox); err != nil {
		t.Fatalf("matching provider should be accepted: %v", err)
	}
	if err := validateShareRecordProvider(model.ShareHistoryEntry{Provider: model.ProviderDropbox}, model.ProviderOnedrive); err == nil {
		t.Fatal("mismatched provider must be rejected before cancellation")
	}
}
