package garminstatus

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ServiceStatus represents the status of a service.
type ServiceStatus string

const (
	// Up represents an "up" service status.
	Up ServiceStatus = "up"

	// Down represents a "down" service status.
	Down ServiceStatus = "down"

	// Garmin Connect status page
	GarminConnectStatusURI = "https://connect.garmin.com/site-status/garmin-connect-status-content.html"
)

// ServiceInfo represents the status and reasons for a service.
type ServiceInfo struct {
	Status       ServiceStatus `json:"status"`
	StatusReason []string      `json:"status_reason,omitempty"`
}

// ServiceMap represents a map of service names to their statuses.
type ServiceMap map[string]ServiceInfo

// GarminStatus represents the status of Garmin services.
type GarminStatus struct {
	Platforms ServiceMap `json:"Platforms"`
	Features  ServiceMap `json:"Features"`
}

var httpClient = &http.Client{
	Timeout: time.Second * 10,
}

// FetchStatus fetches the Garmin service status and returns it as a GarminStatus struct.
func FetchStatus() (GarminStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetch the HTML content from the URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GarminConnectStatusURI, http.NoBody)
	if err != nil {
		return GarminStatus{}, fmt.Errorf("error creating request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return GarminStatus{}, fmt.Errorf("error during http request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return GarminStatus{}, fmt.Errorf("invalid response status code: %d", resp.StatusCode)
	}

	// Parse the HTML content with goquery
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return GarminStatus{}, err
	}

	// Extract services from "Platforms" and "Features" categories using the ExtractServices function
	platforms := extractServices(doc, "#platforms .service")
	features := extractServices(doc, "#features .service")

	// Create a GarminStatus struct
	status := GarminStatus{
		Platforms: platforms,
		Features:  features,
	}

	return status, nil
}

// extractServices extracts services and their statuses from the specified selector.
func extractServices(doc *goquery.Document, selector string) ServiceMap {
	services := make(ServiceMap)

	doc.Find(selector).Each(func(index int, item *goquery.Selection) {
		// Extract service name and status class
		serviceName := strings.TrimSpace(item.Find(".item").Text())
		statusClass, _ := item.Attr("class")

		// Determine the service status based on the status class
		status := Down
		if strings.Contains(statusClass, "green") {
			status = Up
		}

		// Extract status reasons if present
		statusReasons := []string{}
		item.Find(".status-reasons").Each(func(i int, reasonItem *goquery.Selection) {
			reason := strings.TrimSpace(reasonItem.Text())
			if reason != "" {
				statusReasons = append(statusReasons, reason)
			}
		})

		// Store the service status and reasons in the map
		serviceInfo := ServiceInfo{
			Status:       status,
			StatusReason: statusReasons,
		}
		services[serviceName] = serviceInfo
	})

	return services
}
