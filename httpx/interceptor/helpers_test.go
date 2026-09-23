package interceptor_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/japansms40-web/gohttpkit/httpx"
)

func newClient(t *testing.T, srv *httptest.Server, mutate func(*httpx.Options)) *httpx.Client {
	t.Helper()
	opts := httpx.Options{
		Headers: httpx.StaticHeaders{
			Base:    srv.URL,
			Headers: map[string]string{"accept": "application/json", "user-agent": "kit-test/1.0"},
		},
		Retry: httpx.WithRetry(3, time.Millisecond, 0),
	}
	if mutate != nil {
		mutate(&opts)
	}
	c, err := httpx.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

type recordingServer struct {
	*httptest.Server
	mu       chan struct{}
	requests []*http.Request
	bodies   [][]byte
}

func newRecordingServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *recordingServer {
	t.Helper()
	rs := &recordingServer{mu: make(chan struct{}, 1)}
	rs.mu <- struct{}{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		<-rs.mu
		rs.requests = append(rs.requests, r.Clone(context.Background()))
		rs.bodies = append(rs.bodies, body)
		rs.mu <- struct{}{}
		handler(w, r)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) count() int {
	<-rs.mu
	n := len(rs.requests)
	rs.mu <- struct{}{}
	return n
}

func assertNilBuildHeaders(t *testing.T, err error, providerType string) {
	t.Helper()
	var got *httpx.NilBuildHeadersError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.NilBuildHeadersError", err, err)
	}
	if got.ProviderType != providerType {
		t.Fatalf("ProviderType = %q, want %q", got.ProviderType, providerType)
	}
}

func assertReadResponseBody(t *testing.T, err error, encoding httpx.ContentEncoding, cause error) {
	t.Helper()
	var got *httpx.ReadResponseBodyError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *httpx.ReadResponseBodyError", err, err)
	}
	if got.Encoding != encoding || !errors.Is(got, cause) {
		t.Fatalf("Encoding=%q cause=%v, want %q / %v", got.Encoding, got.Err, encoding, cause)
	}
}

func newClientWith(t *testing.T, opts httpx.Options) *httpx.Client {
	t.Helper()
	c, err := httpx.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}
