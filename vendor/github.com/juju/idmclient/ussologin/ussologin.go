// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package ussologin defines functionality used for allowing clients
// to authenticate with the IDM server using USSO OAuth.
package ussologin

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/httprequest"
	"github.com/juju/usso"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
)

type tokenGetter interface {
	GetTokenWithOTP(username, password, otp, tokenName string) (*usso.SSOData, error)
}

// This is defined here to allow it to be stubbed out in tests
var server tokenGetter = usso.ProductionUbuntuSSOServer

var (
	userKey = "E-Mail"
	passKey = "Password"
	otpKey  = "Two-factor auth (Enter for none)"
)

// GetToken uses filler to interact with the user and uses the provided
// information to obtain an OAuth token from Ubuntu SSO. The returned
// token can subsequently be used with LoginWithToken to perform a login.
// The tokenName argument is used as the name of the generated token in
// Ubuntu SSO. If Ubuntu SSO returned an error when trying to retrieve
// the token the error will have a cause of type *usso.Error.
func GetToken(filler form.Filler, tokenName string) (*usso.SSOData, error) {
	login, err := filler.Fill(loginForm)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read login parameters")
	}
	tok, err := server.GetTokenWithOTP(
		login[userKey].(string),
		login[passKey].(string),
		login[otpKey].(string),
		tokenName,
	)

	if err != nil {
		return nil, errgo.NoteMask(err, "cannot get token", isUSSOError)
	}
	return tok, nil
}

// loginForm contains the fields required for login.
var loginForm = form.Form{
	Title: "Login to Ubuntu SSO",
	Fields: environschema.Fields{
		userKey: environschema.Attr{
			Description: "Username",
			Type:        environschema.Tstring,
			Mandatory:   true,
			Group:       "1",
		},
		passKey: environschema.Attr{
			Description: "Password",
			Type:        environschema.Tstring,
			Mandatory:   true,
			Secret:      true,
			Group:       "1",
		},
		otpKey: environschema.Attr{
			Description: "Two-factor auth",
			Type:        environschema.Tstring,
			Mandatory:   true,
			Group:       "2",
		},
	},
}

// LoginWithToken completes a login attempt using tok. The ussoAuthURL
// should have been obtained from the UbuntuSSOOAuth field in a response
// to a LoginMethods request from the target service.
func LoginWithToken(client *http.Client, ussoAuthUrl string, tok *usso.SSOData) error {
	req, err := http.NewRequest("GET", ussoAuthUrl, nil)
	if err != nil {
		return errgo.Notef(err, "cannot create request")
	}
	base := *req.URL
	base.RawQuery = ""
	rp := usso.RequestParameters{
		HTTPMethod:      req.Method,
		BaseURL:         base.String(),
		Params:          req.URL.Query(),
		SignatureMethod: usso.HMACSHA1{},
	}
	if err := tok.SignRequest(&rp, req); err != nil {
		return errgo.Notef(err, "cannot sign request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return errgo.Notef(err, "cannot do request")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var herr httpbakery.Error
	if err := httprequest.UnmarshalJSONResponse(resp, &herr); err != nil {
		return errgo.Notef(err, "cannot unmarshal error")
	}
	return &herr
}

// TokenStore defines the interface for something that can store and returns oauth tokens.
type TokenStore interface {
	// Put stores an Ubuntu SSO OAuth token.
	Put(tok *usso.SSOData) error
	// Get returns an Ubuntu SSO OAuth token from store
	Get() (*usso.SSOData, error)
}

// FileTokenStore implements the TokenStore interface by storing the
// JSON-encoded oauth token in a file.
type FileTokenStore struct {
	path string
}

// NewFileTokenStore returns a new FileTokenStore
// that uses the given path for storage.
func NewFileTokenStore(path string) *FileTokenStore {
	return &FileTokenStore{path}
}

// Put implements TokenStore.Put by writing the token to the
// FileTokenStore's file. If the file doesn't exist it will be created,
// including any required directories.
func (f *FileTokenStore) Put(tok *usso.SSOData) error {
	data, err := json.Marshal(tok)
	if err != nil {
		return errgo.Notef(err, "cannot marshal token")
	}
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errgo.Notef(err, "cannot create directory %q", dir)
	}
	if err := ioutil.WriteFile(f.path, data, 0600); err != nil {
		return errgo.Notef(err, "cannot write file")
	}
	return nil
}

// Get implements TokenStore.Get by
// reading the token from the FileTokenStore's file.
func (f *FileTokenStore) Get() (*usso.SSOData, error) {
	data, err := ioutil.ReadFile(f.path)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read token")
	}
	var tok usso.SSOData
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal token")
	}
	return &tok, nil
}

// isUSSOError determines if err represents an error of type *usso.Error.
func isUSSOError(err error) bool {
	_, ok := err.(*usso.Error)
	return ok
}
