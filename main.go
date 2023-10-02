package main

import (
	"encoding/json"
	"fmt"
	"log"

	"flag"

	"net/http"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
)

var (
	serveHTTP bool
	httpPort  int
)

func init() {
	flag.BoolVar(&serveHTTP, "http", false, "Start an HTTP server to serve the JSON data")
	flag.IntVar(&httpPort, "port", 8080, "HTTP server port")
	flag.Parse()
}

func main() {
	if serveHTTP {
		// Start an HTTP server if the -http flag is provided
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Fetch Garmin service status on each request when serving via HTTP
			status, err := garminstatus.FetchStatus()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Convert the status to JSON
			jsonData, err := json.Marshal(status)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(jsonData)
		})

		fmt.Println("Starting HTTP server on port", httpPort)
		err := http.ListenAndServe(fmt.Sprintf(":%d", httpPort), nil)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		// Fetch Garmin service status and output to stdout when not serving via HTTP
		status, err := garminstatus.FetchStatus()
		if err != nil {
			log.Fatal(err)
		}

		// Convert the status to JSON
		jsonData, err := json.Marshal(status)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(jsonData))
	}
}
