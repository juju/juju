// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	"launchpad.net/loggo"
)

const insecureScheme = "nonvalidating-https"

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

type NonValidatingTransport struct {
	*http.Transport
}

var nvtLogger = loggo.GetLogger("juju.NonValidatingTransport")

func (nvt NonValidatingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != insecureScheme {
		return nil, fmt.Errorf("NonValidatingTransport is only to be used with %s://", insecureScheme)
	}
	nvtLogger.Debugf("Rewriting URL: %v", req.URL)
	req.URL.Scheme = "https"
	return nvt.Transport.RoundTrip(req)
}

func RegisterNonValidatingHTTPS() {
	insecureConfig := &tls.Config{InsecureSkipVerify: true}
	insecureTransport := &NonValidatingTransport{&http.Transport{
		TLSClientConfig: insecureConfig,
		Proxy:           http.ProxyFromEnvironment,
	}}
	http.DefaultTransport.(*http.Transport).RegisterProtocol("nonvalidating-https", insecureTransport)
}

func init() {
	RegisterNonValidatingHTTPS()
}
