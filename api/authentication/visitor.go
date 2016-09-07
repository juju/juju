// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/juju/errors"

	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

const authMethod = "juju_userpass"

// Visitor is a httpbakery.Visitor that will login directly
// to the Juju controller using password authentication. This
// only applies when logging in as a local user.
type Visitor struct {
	username    string
	getPassword func(string) (string, error)
}

// NewVisitor returns a new Visitor.
func NewVisitor(username string, getPassword func(string) (string, error)) *Visitor {
	return &Visitor{
		username:    username,
		getPassword: getPassword,
	}
}

// VisitWebPage is part of the httpbakery.Visitor interface.
func (v *Visitor) VisitWebPage(client *httpbakery.Client, methodURLs map[string]*url.URL) error {
	methodURL := methodURLs[authMethod]
	if methodURL == nil {
		return httpbakery.ErrMethodNotSupported
	}

	password, err := v.getPassword(v.username)
	if err != nil {
		return err
	}

	// POST to the URL with username and password.
	resp, err := client.PostForm(methodURL.String(), url.Values{
		"user":     {v.username},
		"password": {password},
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var jsonError httpbakery.Error
	if err := json.NewDecoder(resp.Body).Decode(&jsonError); err != nil {
		return errors.Annotate(err, "unmarshalling error")
	}
	return &jsonError
}
