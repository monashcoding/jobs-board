package unblocker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// noRetry keeps a test to a single attempt, so a failure surfaces immediately
// instead of being retried behind five seconds of backoff.
func noRetry() *RetryConfig {
	return &RetryConfig{MaxRetries: 0}
}

func newTestService(t *testing.T) *service {
	t.Helper()

	svc, err := NewService(Config{Retry: noRetry()})
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	s, ok := svc.(*service)
	if !ok {
		t.Fatalf("NewService returned %T, want *service", svc)
	}
	return s
}

// The client had no timeout at all, so a proxied fetch that connected and then
// went quiet hung the scrape with nothing able to interrupt it.
func TestClientBoundsEveryRequest(t *testing.T) {
	s := newTestService(t)

	if s.client.Timeout <= 0 {
		t.Errorf("the client must bound a request, Timeout = %v", s.client.Timeout)
	}
}

// The scrapers build their requests with http.NewRequest and no context, so
// nothing bound the pipeline's cancellation -- or SIGTERM, which cancels it --
// to a call already in flight. executeRequest binds it centrally instead.
func TestRequestIsAbortedWhenTheContextIsCancelled(t *testing.T) {
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-released
	}))
	defer server.Close()
	defer close(released)

	s := newTestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := s.MakeRequest(ctx, req)
		done <- err
	}()

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled request must not report success")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelling the context did not abort the request in flight")
	}
}

// stubIPManager hands out a rotating proxy session without touching BrightData,
// so a test can drive the path that used to swap the transport per request.
type stubIPManager struct {
	proxy     string
	rotations int
}

func (m *stubIPManager) GetProxy(ctx context.Context) (string, error) {
	m.rotations++
	return fmt.Sprintf("http://user-session-%d-zone:pass@%s", m.rotations, m.proxy), nil
}

func (m *stubIPManager) RotateProxy(ctx context.Context, proxyURL string) error { return nil }

// Every request used to build its own http.Transport and hand it to the shared
// client. A transport owns a connection pool and the goroutines serving it, and
// the discarded ones were never closed, so a long scrape accumulated hundreds
// of them holding idle connections. The proxy changes per request, so it is
// resolved per request instead of being baked into a new transport each time.
func TestTransportIsSharedAcrossRequests(t *testing.T) {
	var proxied int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied++
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := newTestService(t)
	s.ipManager = &stubIPManager{proxy: strings.TrimPrefix(proxy.URL, "http://")}

	if s.client.Transport == nil {
		t.Fatal("the service must configure its own transport rather than relying on the default")
	}

	fetch := func() {
		t.Helper()
		req, err := http.NewRequest("GET", server.URL, nil)
		if err != nil {
			t.Fatalf("failed to build request: %v", err)
		}
		resp, err := s.MakeRequest(context.Background(), req)
		if err != nil {
			t.Fatalf("MakeRequest returned error: %v", err)
		}
		resp.Body.Close()
	}

	// Compared after a first proxied request, so this catches the transport
	// being swapped per request and not merely how the client starts out.
	fetch()
	afterFirst := s.client.Transport

	fetch()
	fetch()

	if s.client.Transport != afterFirst {
		t.Error("the client's transport was replaced; a scrape must reuse one connection pool")
	}
	// Proof the proxy still applied: every request went through it, each on a
	// freshly rotated session, with one transport serving all of them.
	if proxied != 3 {
		t.Errorf("expected all 3 requests to go via the proxy, got %d", proxied)
	}
}

// With one shared transport the proxy can no longer be baked into it, so it is
// resolved per request instead. A request carrying no proxy must go direct.
func TestProxyHookFallsBackToADirectConnection(t *testing.T) {
	s := newTestService(t)

	transport, ok := s.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", s.client.Transport)
	}

	req, err := http.NewRequest("GET", "https://www.linkedin.com/jobs/view/1", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("proxy hook returned error: %v", err)
	}
	if proxyURL != nil {
		t.Errorf("a request with no proxy in its context must go direct, got %v", proxyURL)
	}
}
