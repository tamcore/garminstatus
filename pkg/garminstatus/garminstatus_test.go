// pkg/garminstatus/garminstatus_test.go

package garminstatus

import (
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func TestFetchStatus(t *testing.T) {
	// Test case: Successful fetch
	status, err := FetchStatus()
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
	if status.Platforms == nil || status.Features == nil {
		t.Errorf("Expected non-nil ServicesMap, but got nil")
	}

	// Test case: Failed fetch ( mock error in HTTP request)
	httpClient = &http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			Dial: func(string, string) (net.Conn, error) {
				return nil, errors.New("mock error")
			},
		},
	}
	_, err = FetchStatus()
	if err == nil {
		t.Errorf("Expected error, but got nil")
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
