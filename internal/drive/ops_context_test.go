package drive_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
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

func TestContextAwareDriveOpsCancelInFlightWebDAVRequests(t *testing.T) {
	started := make(chan string, 3)
	previousTransport := netx.TestTransportHook
	netx.TestTransportHook = cancellationRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		started <- req.Method
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

	tests := []struct {
		name       string
		wantMethod string
		call       func(context.Context) error
	}{
		{
			name:       "读取文件元数据",
			wantMethod: "PROPFIND",
			call: func(ctx context.Context) error {
				_, err := drive.GetFileContext(ctx, "webdav:test", "webdav", "file")
				return err
			},
		},
		{
			name:       "创建目录",
			wantMethod: "MKCOL",
			call: func(ctx context.Context) error {
				result, err := drive.MkdirContext(ctx, "webdav:test", "webdav", "/", "folder")
				if err == nil && result != nil && result.Error != "" {
					return errors.New(result.Error)
				}
				return err
			},
		},
		{
			name:       "删除文件",
			wantMethod: http.MethodDelete,
			call: func(ctx context.Context) error {
				_, err := drive.DeleteBatchContext(ctx, "webdav:test", "webdav", []drive.FileRef{{ID: "file"}})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- tt.call(ctx) }()

			select {
			case got := <-started:
				if got != tt.wantMethod {
					t.Fatalf("request method = %q, want %q", got, tt.wantMethod)
				}
			case <-time.After(time.Second):
				t.Fatal("WebDAV request did not start")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) && (err == nil || !strings.Contains(err.Error(), context.Canceled.Error())) {
					t.Fatalf("operation error = %v, want context.Canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("operation did not return after cancellation")
			}
		})
	}
}

type cancellationRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cancellationRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
