package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"gopkg.in/errgo.v1"

	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

// client represents a client of the target service.
// In this simple example, it just tries a GET
// request, which will fail unless the client
// has the required authorization.
func clientRequest(client *httpbakery.Client, serverEndpoint string) (string, error) {
	// The Do function implements the mechanics
	// of actually gathering discharge macaroons
	// when required, and retrying the request
	// when necessary.
	req, err := http.NewRequest("GET", serverEndpoint, nil)
	if err != nil {
		return "", errgo.Notef(err, "cannot make new HTTP request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", errgo.NoteMask(err, "GET failed", errgo.Any)
	}
	defer resp.Body.Close()
	// TODO(rog) unmarshal error
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cannot read response: %v", err)
	}
	return string(data), nil
}
