package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/tamcore/garminstatus/pkg/garminstatus"
	"github.com/tamcore/garminstatus/pkg/http"
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
		err := http.ServeHTTP(httpPort)
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
