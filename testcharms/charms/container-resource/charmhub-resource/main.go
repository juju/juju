// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"log"
	"net/http"
)

func resource(w http.ResponseWriter, _ *http.Request) {
	log.Printf("new request to the charmhub resource whoami server")
	fmt.Fprintf(w, "I am the charmhub resource (revision 4)")
}

func main() {
	log.Printf("starting whoami server of the charmhub resource")
	http.HandleFunc("/", resource)
	err := http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatalf("resource whoami server failed: %v", err)
	}
	log.Printf("stopping the charmhub resource whoami server")
}
