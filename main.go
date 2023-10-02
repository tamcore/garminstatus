package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

func main() {
	// Fetch Garmin service status
	status, err := garminstatus.FetchStatus()
	if err != nil {
		log.Fatal(err)
	}

	// Convert the status to JSON
	jsonData, err := json.Marshal(status)
	if err != nil {
		log.Fatal(err)
	}

	// Print the JSON data
	fmt.Println(string(jsonData))
}
