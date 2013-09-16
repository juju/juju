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
