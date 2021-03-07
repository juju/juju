// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v3/httpbakery"
	"gopkg.in/macaroon-bakery.v3/httpbakery/form"
)

const authMethod = "juju_userpass"

// Visitor is a httpbakery.Visitor that will login directly
// to the Juju controller using password authentication. This
// only applies when logging in as a local user.
type Interactor struct {
	username    string
	getPassword func(string) (string, error)
}

// NewInteractor returns a new Interactor.
func NewInteractor(username string, getPassword func(string) (string, error)) httpbakery.Interactor {
	return &Interactor{
		username:    username,
		getPassword: getPassword,
	}
}

// Kind implements httpbakery.Interactor.Kind.
func (i Interactor) Kind() string {
	return authMethod
}

// Interact implements httpbakery.Interactor for the Interactor.
func (i Interactor) Interact(ctx context.Context, client *httpbakery.Client, location string, interactionRequiredErr *httpbakery.Error) (*httpbakery.DischargeToken, error) {
	var p form.InteractionInfo
	if err := interactionRequiredErr.InteractionMethod(authMethod, &p); err != nil {
		return nil, errors.Trace(err)
	}
	if p.URL == "" {
		return nil, errors.New("no URL found in form information")
	}
	schemaURL, err := relativeURL(location, p.URL)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid url %q", p.URL)
	}
	httpReqClient := &httprequest.Client{
		Doer: client,
	}
	password, err := i.getPassword(i.username)
	if err != nil {
		return nil, errors.Trace(err)
	}
	lr := form.LoginRequest{
		Body: form.LoginBody{
			Form: map[string]interface{}{
				"user":     i.username,
				"password": password,
			},
		},
	}
	var lresp form.LoginResponse
	if err := httpReqClient.CallURL(ctx, schemaURL.String(), &lr, &lresp); err != nil {
		return nil, errors.Annotate(err, "cannot submit form")
	}
	if lresp.Token == nil {
		return nil, errors.New("no token found in form response")
	}
	return lresp.Token, nil
}

// relativeURL returns newPath relative to an original URL.
func relativeURL(base, new string) (*url.URL, error) {
	if new == "" {
		return nil, errors.New("empty URL")
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, errors.Annotate(err, "cannot parse URL")
	}
	newURL, err := url.Parse(new)
	if err != nil {
		return nil, errors.Annotate(err, "cannot parse URL")
	}
	return baseURL.ResolveReference(newURL), nil
}

// LegacyInteract implements httpbakery.LegacyInteractor for the Interactor.
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
