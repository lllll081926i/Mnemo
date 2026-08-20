package drive_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"mnemo-go/internal/drive"
	_ "mnemo-go/internal/drive/providers/webdav"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
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

func TestContextAwareDriveOpsStopBeforeProviderRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := drive.ListDirAllContext(ctx, "webdav:test", "webdav", "root", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListDirAllContext error = %v, want context.Canceled", err)
	}
	if _, err := drive.GetFileContext(ctx, "webdav:test", "webdav", "file"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetFileContext error = %v, want context.Canceled", err)
	}
	if _, err := drive.DeleteBatchContext(ctx, "webdav:test", "webdav", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("DeleteBatchContext error = %v, want context.Canceled", err)
	}
	if _, err := drive.RapidUploadByHashContext(ctx, "webdav:test", "webdav", drive.RapidUploadRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("RapidUploadByHashContext error = %v, want context.Canceled", err)
	}
	if _, err := drive.ResolveTransferHashContext(ctx, "webdav:test", "webdav", "file", "md5", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveTransferHashContext error = %v, want context.Canceled", err)
	}
	if _, err := drive.StreamUploadHandlerContext(ctx, "webdav:test", "webdav"); !errors.Is(err, context.Canceled) {
		t.Fatalf("StreamUploadHandlerContext error = %v, want context.Canceled", err)
	}
}

func TestListDirContextCancelsInFlightWebDAVRequest(t *testing.T) {
	started := make(chan struct{}, 1)
	previousTransport := netx.TestTransportHook
	netx.TestTransportHook = cancellationRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	t.Cleanup(func() { netx.TestTransportHook = previousTransport })

	drive.SetTokenResolver(func(_, _ string) (*model.TokenInfo, error) {
		return &model.TokenInfo{
			TokenFrom: model.ProviderWebdav,
			Conn:      &model.ConnConfig{Endpoint: "http://127.0.0.1:1/"},
		}, nil
	})
	t.Cleanup(func() { drive.SetTokenResolver(nil) })

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := drive.ListDirContext(ctx, "webdav:test", "webdav", "/", nil)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("WebDAV request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ListDirContext error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ListDirContext did not return after cancellation")
	}
}

type cancellationRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cancellationRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
