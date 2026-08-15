// Mock API routing helpers for provider integration tests. They let provider
// drivers make REAL HTTP requests that get transparently routed to local
// httptest servers, exercising request building, auth headers, response
// parsing and file mapping end to end (no live cloud credentials needed).
package e2e

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/store"
)

// rewriteRT routes any request whose Host matches fromHost to the mock target.
type rewriteRT struct {
	fromHost string
	mockHost string
	mu       sync.Mutex
	last     []byte
}

func (r *rewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Hostname() == r.fromHost {
		u := *req.URL
		u.Scheme = "http"
		u.Host = r.mockHost
		req2 := req.Clone(req.Context())
		req2.URL = &u
		req2.RequestURI = ""
		if req.Body != nil {
			body, _ := io.ReadAll(req.Body)
			req.Body.Close()
			req2.Body = io.NopCloser(bytes.NewReader(body))
			r.mu.Lock()
			r.last = body
			r.mu.Unlock()
		}
		return http.DefaultTransport.RoundTrip(req2)
	}
	return http.DefaultTransport.RoundTrip(req)
}

// MockAPI routes host to a local handler and returns the wrapper.
func MockAPI(t *testing.T, host string, handler http.Handler) *MockAPIServer {
	t.Helper()
	srv := httptest.NewServer(handler)
	mockHost := stripSchemeHost(srv.URL)
	rt := &rewriteRT{fromHost: host, mockHost: mockHost}
	netx.TestTransportHook = rt
	t.Cleanup(func() {
		netx.TestTransportHook = nil
		srv.Close()
	})
	return &MockAPIServer{Server: srv, rt: rt}
}

// MockAPIServer exposes the mock and its last captured request body.
type MockAPIServer struct {
	Server *httptest.Server
	rt     *rewriteRT
}

// LastBody returns the most recent request body routed to the mock.
func (m *MockAPIServer) LastBody() []byte {
	if m == nil || m.rt == nil {
		return nil
	}
	m.rt.mu.Lock()
	defer m.rt.mu.Unlock()
	return m.rt.last
}

// SeedAccount saves an account into a fresh store and wires the drive token
// resolver. Returns userID, driveID, store.
func SeedAccount(t *testing.T, provider string, tok *model.TokenInfo) (userID, driveID string, st *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	if tok.UserID == "" {
		tok.UserID = model.BuildUserID(provider, "test")
	}
	userID = tok.UserID
	driveID = model.BuildDriveID(provider, "test")
	acc := &model.Account{UserID: userID, DriveID: driveID, Token: tok}
	if err := st.SaveAccount(acc); err != nil {
		t.Fatal(err)
	}
	drive.SetTokenResolver(func(uid, _ string) (*model.TokenInfo, error) {
		a, err := st.GetAccount(uid)
		if err != nil {
			return nil, err
		}
		return a.Token, nil
	})
	return userID, driveID, st
}

// listNames lists a dir and returns file names.
func listNames(t *testing.T, userID, driveID, dirID string) []string {
	t.Helper()
	files, err := drive.ListDir(userID, driveID, dirID, nil)
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

func stripSchemeHost(u string) string {
	for _, prefix := range []string{"http://", "https://"} {
		if len(u) > len(prefix) && u[:len(prefix)] == prefix {
			return u[len(prefix):]
		}
	}
	return u
}

var _ = bytes.NewReader