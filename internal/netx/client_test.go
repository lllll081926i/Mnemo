package netx

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestClientDoCancelsBlockedTransport(t *testing.T) {
	started := make(chan struct{}, 1)
	previous := TestTransportHook
	TestTransportHook = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	t.Cleanup(func() { TestTransportHook = previous })

	client := NewClient(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Do(ctx, http.MethodGet, "http://127.0.0.1:1/blocked", nil, nil)
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("transport did not receive request")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Do error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Do did not return after cancellation")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
