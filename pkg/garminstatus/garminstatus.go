package garminstatus

import (
	"log"
	"net/http"
	"strings"

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
	GarminConnectStatusURI = "https://status.garminconnectweb.workers.dev/garmin-connect-status-content.html"
)

// ServiceMap represents a map of service names to their statuses.
type ServiceMap map[string]ServiceStatus

// GarminStatus represents the status of Garmin services.
type GarminStatus struct {
	Platforms ServiceMap `json:"Platforms"`
	Features  ServiceMap `json:"Features"`
}

// FetchStatus fetches the Garmin service status and returns it as a GarminStatus struct.
func FetchStatus() (GarminStatus, error) {
	// Fetch the HTML content from the URL
	resp, err := http.Get(GarminConnectStatusURI)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GarminStatus{}, err
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

		// Store the service status in the map
		services[serviceName] = status
	})

	return services
}
