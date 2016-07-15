// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent // import "gopkg.in/juju/charmstore.v5-unstable/internal/agent"

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

var logger = loggo.GetLogger("charmstore.internal.agent")

type loginMethods struct {
	Agent string `json:"agent"`
}

type agentLoginRequest struct {
	Username  string            `json:"username"`
	PublicKey *bakery.PublicKey `json:"public_key"`
}

// TODO make VisitWebPage support using different usernames (and possibly
// keys) for different sites.

// VisitWebPage returns a function that can be used with
// httpbakery.Client.VisitWebPage. The returned function will attept to
// perform an agent login with the server.
func VisitWebPage(c *httpbakery.Client, username string) func(u *url.URL) error {
	return func(u *url.URL) error {
		logger.Infof("Attempting agent login to %q", u)
		req, err := http.NewRequest("GET", u.String(), nil)
		if err != nil {
			return errgo.Notef(err, "cannot create request")
		}
		// Set the Accept header to indicate that we're asking for a
		// non-interactive login.
		req.Header.Set("Accept", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			return errgo.Notef(err, "cannot get login methods")
		}
		defer resp.Body.Close()
		var lm loginMethods
		if err := router.UnmarshalJSONResponse(resp, &lm, getError); err != nil {
			return errgo.Notef(err, "cannot get login methods")
		}
		if lm.Agent == "" {
			return errgo.New("agent login not supported")
		}
		lr := &agentLoginRequest{
			Username: username,
		}
		if c.Key != nil {
			lr.PublicKey = &c.Key.Public
		}
		body, err := json.Marshal(lr)
		if err != nil {
			return errgo.Notef(err, "cannot marshal login request")
		}
		req, err = http.NewRequest("POST", lm.Agent, nil)
		if err != nil {
			return errgo.Notef(err, "cannot create login request")
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err = c.DoWithBody(req, bytes.NewReader(body))
		if err != nil {
			return errgo.Notef(err, "cannot post login request")
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			return errgo.Notef(getError(resp), "cannot log in")
		}
		return nil
	}
}

// NewClient creates an httpbakery.Client that is configured to use agent
// login. The agent login attempts will be made using the provided
// username and key.
func NewClient(username string, key *bakery.KeyPair) *httpbakery.Client {
	c := httpbakery.NewClient()
	c.Key = key
	c.VisitWebPage = VisitWebPage(c, username)
	return c
}

func getError(resp *http.Response) error {
	var aerr agentError
	if err := router.UnmarshalJSONResponse(resp, &aerr, nil); err != nil {
		return err
	}
	return aerr
}

type agentError struct {
	Message string
}

func (e agentError) Error() string {
	return e.Message
}
