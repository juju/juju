// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"log"
	"net/http"
)

func resource(w http.ResponseWriter, _ *http.Request) {
	log.Printf("new request to resource 2 whoami server")
	fmt.Fprintf(w, "I am resource 2")
}

func main() {
	log.Printf("starting whoami server of resource 2")
	http.HandleFunc("/", resource)
	err := http.ListenAndServe("0.0.0.0:8080", nil)
	if err != nil {
		log.Fatalf("resource whoami server failed: %v", err)
	}
	log.Printf("stopping resource 2 whoami server")
}
