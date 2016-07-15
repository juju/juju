// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package idmclient

import (
	"io"
	"net/http"
	"net/url"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/idmclient/params"
)

// Note: tests for this code are in the server implementation.

const (
	Production = "https://api.jujucharms.com/identity"
	Staging    = "https://api.staging.jujucharms.com/identity"
)

// Client represents the client of an identity server.
type Client struct {
	client
}

// NewParams holds the parameters for creating a new client.
type NewParams struct {
	BaseURL string
	Client  *httpbakery.Client

	// AuthUsername holds the username for admin login.
	AuthUsername string

	// AuthPassword holds the password for admin login.
	AuthPassword string
}

// New returns a new client.
func New(p NewParams) *Client {
	var c Client
	c.Client.BaseURL = p.BaseURL
	if p.AuthUsername != "" {
		c.Client.Doer = &basicAuthClient{
			client:   p.Client,
			user:     p.AuthUsername,
			password: p.AuthPassword,
		}
	} else {
		c.Client.Doer = p.Client
	}
	c.Client.UnmarshalError = httprequest.ErrorUnmarshaler(new(params.Error))
	return &c
}

// basicAuthClient wraps a bakery.Client, adding a basic auth
// header to every request.
type basicAuthClient struct {
	client   *httpbakery.Client
	user     string
	password string
}

func (c *basicAuthClient) Do(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(c.user, c.password)
	return c.client.Do(req)
}

func (c *basicAuthClient) DoWithBody(req *http.Request, r io.ReadSeeker) (*http.Response, error) {
	req.SetBasicAuth(c.user, c.password)
	return c.client.DoWithBody(req, r)
}

// LoginMethods returns information about the available login methods
// for the given URL, which is expected to be a URL as passed to
// a VisitWebPage function during the macaroon bakery discharge process.
func LoginMethods(client *http.Client, u *url.URL) (*params.LoginMethods, error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot create request")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errgo.Notef(err, "cannot do request")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var herr httpbakery.Error
		if err := httprequest.UnmarshalJSONResponse(resp, &herr); err != nil {
			return nil, errgo.Notef(err, "cannot unmarshal error")
		}
		return nil, &herr
	}
	var lm params.LoginMethods
	if err := httprequest.UnmarshalJSONResponse(resp, &lm); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal login methods")
	}
	return &lm, nil
}

//go:generate httprequest-generate-client $IDM_SERVER_REPO/internal/v1 apiHandler client
