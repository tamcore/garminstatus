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

	// Initialize a map to store the service status information
	serviceStatus := make(map[string]string)

	// Find all elements with class "service"
	doc.Find(".service").Each(func(index int, item *goquery.Selection) {
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

		// Store the service status in the map
		serviceStatus[serviceName] = status
	})

	// Convert the service status map to JSON
	jsonData, err := json.Marshal(serviceStatus)
	if err != nil {
		log.Fatal(err)
	}

	// Print the JSON data
	fmt.Println(string(jsonData))
}
