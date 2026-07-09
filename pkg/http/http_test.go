package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerEndpoints(t *testing.T) {
	ts := httptest.NewServer(Handler())
	defer ts.Close()

	for _, path := range []string{"/metrics", "/live", "/ready"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("GET %s -> %d", path, resp.StatusCode)
		}
	}
}

func TestServeInvalidAddr(t *testing.T) {
	// An invalid port makes ListenAndServe fail immediately, exercising the
	// error path (the success path blocks forever and is not unit-testable).
	if err := Serve("127.0.0.1:99999999"); err == nil {
		t.Error("expected error for invalid address")
	}
}
