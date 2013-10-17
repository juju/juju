// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/tls"
	"net/http"
	"sync"
)

var insecureClient = (*http.Client)(nil)
var insecureClientMutex = sync.Mutex{}

func GetNonValidatingHTTPClient() *http.Client {
	insecureClientMutex.Lock()
	defer insecureClientMutex.Unlock()
	if insecureClient == nil {
		insecureConfig := &tls.Config{InsecureSkipVerify: true}
		insecureTransport := &http.Transport{TLSClientConfig: insecureConfig}
		insecureClient = &http.Client{Transport: insecureTransport}
	}
	return insecureClient
}

// HTTPGet issues a GET to the specified URL using the http client.
func HTTPGet(c *http.Client, url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return HTTPSendRequest(c, req)
}

// HTTPSendRequest dispatches the request on the client.
func HTTPSendRequest(c *http.Client, req *http.Request) (resp *http.Response, err error) {
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	req.Close = true
	return c.Do(req)
}
