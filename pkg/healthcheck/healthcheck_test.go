package healthcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetup(t *testing.T) {
	h := Setup()
	if h == nil {
		t.Fatal("Setup returned nil handler")
	}
	for name, fn := range map[string]http.HandlerFunc{
		"/ready": h.ReadyEndpoint,
		"/live":  h.LiveEndpoint,
	} {
		rr := httptest.NewRecorder()
		fn(rr, httptest.NewRequest(http.MethodGet, name, nil))
		// Either healthy (200) or failing checks (503) are acceptable outcomes;
		// we only assert the endpoint is wired and responds.
		if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s returned %d", name, rr.Code)
		}
	}
}
