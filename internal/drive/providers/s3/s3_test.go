package s3

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"mnemo-go/internal/model"
)

type countingRoundTripper struct {
	mu      sync.Mutex
	methods []string
}

func (rt *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.methods = append(rt.methods, req.Method)
	rt.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

func (rt *countingRoundTripper) snapshot() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.methods...)
}

func testS3Config() *model.ConnConfig {
	forcePathStyle := true
	return &model.ConnConfig{
		Endpoint:       "http://s3.test",
		Username:       "access-key",
		Password:       "secret-key",
		Bucket:         "bucket",
		Region:         "us-east-1",
		BasePath:       "mount/root",
		ForcePathStyle: &forcePathStyle,
	}
}

func TestValidateConnectionUsesOneRequestOnSuccess(t *testing.T) {
	transport := &countingRoundTripper{}
	previous := TransportOverride
	TransportOverride = transport
	defer func() { TransportOverride = previous }()

	if err := (&Driver{}).ValidateConnection(context.Background(), testS3Config()); err != nil {
		t.Fatalf("ValidateConnection: %v", err)
	}
	methods := transport.snapshot()
	if len(methods) != 1 || methods[0] != http.MethodHead {
		t.Fatalf("login validation methods = %#v, want one HEAD request", methods)
	}
}

func TestValidateWriteConnectionUsesPutAndDelete(t *testing.T) {
	transport := &countingRoundTripper{}
	previous := TransportOverride
	TransportOverride = transport
	defer func() { TransportOverride = previous }()

	if err := (&Driver{}).ValidateWriteConnection(context.Background(), testS3Config()); err != nil {
		t.Fatalf("ValidateWriteConnection: %v", err)
	}
	methods := transport.snapshot()
	if len(methods) != 2 || methods[0] != http.MethodPut || methods[1] != http.MethodDelete {
		t.Fatalf("write validation methods = %#v, want PUT then DELETE", methods)
	}
}

func TestWriteProbeKeyKeepsConfiguredPrefix(t *testing.T) {
	key, err := writeProbeKey("mount/root/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "mount/root/.mnemo-connection-check/") {
		t.Fatalf("probe key = %q, missing configured prefix", key)
	}
	if strings.Contains(key, "..") {
		t.Fatalf("probe key contains traversal segment: %q", key)
	}
}
