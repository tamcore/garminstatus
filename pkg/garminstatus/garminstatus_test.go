// pkg/garminstatus/garminstatus_test.go

package garminstatus

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

const fixtureHTML = `
<div id="platforms">
	<div class="service green"><div class="item">Connect</div></div>
</div>
<div id="features">
	<div class="service red">
		<div class="item">Sync</div>
		<div class="status-reasons">Delayed</div>
	</div>
</div>`

func serve(t *testing.T, code int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchStatus(t *testing.T) {
	srv := serve(t, http.StatusOK, fixtureHTML)

	status, err := FetchStatus(t.Context(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := status.Platforms["Connect"]; got.Status != Up {
		t.Errorf("Connect = %+v, want up", got)
	}
	got := status.Features["Sync"]
	if got.Status != Down || len(got.StatusReason) != 1 || got.StatusReason[0] != "Delayed" {
		t.Errorf("Sync = %+v, want down with reason Delayed", got)
	}
}

func TestFetchStatusErrors(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
		want error
	}{
		{"empty page", http.StatusOK, "<html></html>", ErrNoServices},
		{"bad status", http.StatusServiceUnavailable, fixtureHTML, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := serve(t, tc.code, tc.body)
			_, err := FetchStatus(t.Context(), srv.Client(), srv.URL)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestFetchStatusTransportError(t *testing.T) {
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("mock error")
		},
	}}
	if _, err := FetchStatus(t.Context(), client, GarminConnectStatusURI); err == nil {
		t.Error("expected error, got nil")
	}
}

func TestExtractServices(t *testing.T) {
	// Test case: Successful extraction
	html := `
		<div id="platforms">
			<div class="service green">
				<div class="item">Service 1</div>
				<div class="status-reasons">Reason 1</div>
			</div>
			<div class="service red">
				<div class="item">Service 2</div>
			</div>
		</div>
	`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Fatal(err)
	}
	services := extractServices(doc, "#platforms .service")
	if len(services) != 2 {
		t.Errorf("Expected 2 services, but got %d", len(services))
	}
	service1, ok := services["Service 1"]
	if !ok {
		t.Errorf("Expected service 'Service 1' to exist")
	}
	if service1.Status != Up {
		t.Errorf("Expected service 'Service 1' to be up, but got %v", service1.Status)
	}
	if len(service1.StatusReason) != 1 {
		t.Errorf("Expected service 'Service 1' to have 1 reason, but got %d", len(service1.StatusReason))
	}
	service2, ok := services["Service 2"]
	if !ok {
		t.Errorf("Expected service 'Service 2' to exist")
	}
	if service2.Status != Down {
		t.Errorf("Expected service 'Service 2' to be down, but got %v", service2.Status)
	}
	if len(service2.StatusReason) != 0 {
		t.Errorf("Expected service 'Service 2' to have 0 reasons, but got %d", len(service2.StatusReason))
	}
}

func TestExtractServicesEmpty(t *testing.T) {
	// Test case: Successful extraction with empty HTML
	html := ""
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		log.Fatal(err)
	}
	services := extractServices(doc, "#platforms .service")
	if len(services) != 0 {
		t.Errorf("Expected 0 services, but got %d", len(services))
	}
}
