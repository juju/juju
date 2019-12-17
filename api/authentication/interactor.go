// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	schemaform "gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon-bakery.v2/httpbakery/form"
)

const authMethod = "juju_userpass"

// Visitor is a httpbakery.Visitor that will login directly
// to the Juju controller using password authentication. This
// only applies when logging in as a local user.
type Interactor struct {
	form.Interactor
	username    string
	getPassword func(string) (string, error)
}

// NewInteractor returns a new Interactor.
func NewInteractor(username string, getPassword func(string) (string, error)) httpbakery.Interactor {
	return &Interactor{
		Interactor:  form.Interactor{Filler: schemaform.IOFiller{}},
		username:    username,
		getPassword: getPassword,
	}
}

// Kind implements httpbakery.Interactor.Kind.
func (i Interactor) Kind() string {
	return authMethod
}

// LegacyInteract implements httpbakery.LegacyInteractor
// for the Interactor.
func (i *Interactor) LegacyInteract(ctx context.Context, client *httpbakery.Client, location string, methodURL *url.URL) error {
	password, err := i.getPassword(i.username)
	if err != nil {
		return err
	}

	// POST to the URL with username and password.
	resp, err := client.PostForm(methodURL.String(), url.Values{
		"user":     {i.username},
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
