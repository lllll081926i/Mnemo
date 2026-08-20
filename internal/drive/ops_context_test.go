package drive_test

import (
	"context"
	"errors"
	"testing"

	"mnemo-go/internal/drive"
	_ "mnemo-go/internal/drive/providers/webdav"
)

// This only exercises the facade's setup cancellation gate. It deliberately
// uses a canceled context before a provider operation, so the test never
// opens a real WebDAV connection or sends a request to a user's account.
func TestQueueUploadHandlerContextRejectsCanceledSetup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := drive.QueueUploadHandlerContext(ctx, "webdav:test", "webdav")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
