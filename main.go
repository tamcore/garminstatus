package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func main() {
	// Define the URL to fetch
	url := "https://status.garminconnectweb.workers.dev/garmin-connect-status-content.html"

	// Fetch the HTML content from the URL
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("HTTP request failed with status code: %d", resp.StatusCode)
	}

	// Parse the HTML content with goquery
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize maps to store the service status information for "Platforms" and "Features"
	platforms := make(map[string]string)
	features := make(map[string]string)

	// Find the div element with id "platforms" and extract its services
	doc.Find("#platforms .service").Each(func(index int, item *goquery.Selection) {
		// Extract service name and status class
		serviceName := strings.TrimSpace(item.Find(".item").Text())
		statusClass, _ := item.Attr("class")

		// Determine the service status based on the status class
		status := "unknown"
		if strings.Contains(statusClass, "green") {
			status = "up"
		} else if strings.Contains(statusClass, "red") {
			status = "down"
		}

		// Store the service status in the "Platforms" map
		platforms[serviceName] = status
	})

	// Find the div element with id "features" and extract its services
	doc.Find("#features .service").Each(func(index int, item *goquery.Selection) {
		// Extract service name and status class
		serviceName := strings.TrimSpace(item.Find(".item").Text())
		statusClass, _ := item.Attr("class")

		// Determine the service status based on the status class
		status := "unknown"
		if strings.Contains(statusClass, "green") {
			status = "up"
		} else if strings.Contains(statusClass, "red") {
			status = "down"
		}

		// Store the service status in the "Features" map
		features[serviceName] = status
	})

	// Create a map to store the categorized service status
	serviceStatus := map[string]map[string]string{
		"Platforms": platforms,
		"Features":  features,
	}

	// Convert the categorized service status map to JSON
	jsonData, err := json.Marshal(serviceStatus)
	if err != nil {
		log.Fatal(err)
	}

	// Print the JSON data
	fmt.Println(string(jsonData))
}
