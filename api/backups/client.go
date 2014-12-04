// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"io"
	"net/http"
	"strings"

	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
)

var logger = loggo.GetLogger("juju.api.backups")

// httpClient represents the methods of api.State (see api/http.go)
// needed by backups for direct HTTP requests.
type httpClient interface {
	// SendHTTPRequest sends an HTTP GET request relative to the client.
	SendHTTPRequest(path string, args interface{}) (*http.Request, *http.Response, error)
	// SendHTTPRequestReader sends an HTTP PUT request relative to the client.
	SendHTTPRequestReader(path string, attached io.Reader, meta interface{}, name string) (*http.Request, *http.Response, error)
}

type apiState interface {
	base.APICallCloser
	httpClient

	// Addr returns the address used to connect to the API server.
	Addr() string
}

// Client wraps the backups API for the client.
type Client struct {
	base.ClientFacade
	facade        base.FacadeCaller
	http          httpClient
	baseFacade    base.FacadeCaller
	publicAddress string
}

// NewClient returns a new backups API client.
func NewClient(st apiState) *Client {
	publicAddress := strings.SplitN(st.Addr(), ":", 2)[0]
	frontend, backend := base.NewClientFacade(st, "Backups")
	return &Client{
		ClientFacade:  frontend,
		facade:        backend,
		http:          st,
		publicAddress: publicAddress,
	}
}
