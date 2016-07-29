// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package terms contains the terms service API client.
package terms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var BaseURL = "https://api.jujucharms.com/terms/v1"

// CheckAgreementsRequest holds a slice of terms and the /v1/agreement
// endpoint will check if the user has agreed to the specified terms
// and return a slice of terms the user has not agreed to yet.
type CheckAgreementsRequest struct {
	Terms []string
}

// GetTermsResponse holds the response of the GetTerms call.
type GetTermsResponse struct {
	Name      string    `json:"name"`
	Title     string    `json:"title"`
	Revision  int       `json:"revision"`
	CreatedOn time.Time `json:"created-on"`
	Content   string    `json:"content"`
}

// SaveAgreementResponses holds the response of the SaveAgreement
// call.
type SaveAgreementResponses struct {
	Agreements []AgreementResponse `json:"agreements"`
}

// AgreementResponse holds the a single agreement made by
// the user to a specific revision of terms and conditions
// document.
type AgreementResponse struct {
	User      string    `json:"user"`
	Term      string    `json:"term"`
	Revision  int       `json:"revision"`
	CreatedOn time.Time `json:"created-on"`
}

// SaveAgreements holds the parameters for creating new
// user agreements to one or more specific revisions of terms.
type SaveAgreements struct {
	Agreements []SaveAgreement `json:"agreements"`
}

// SaveAgreement holds the parameters for creating a new
// user agreement to a specific revision of terms.
type SaveAgreement struct {
	TermName     string `json:"termname"`
	TermRevision int    `json:"termrevision"`
}

// Client defines method needed for the Terms Service CLI
// commands.
type Client interface {
	GetUnsignedTerms(p *CheckAgreementsRequest) ([]GetTermsResponse, error)
	SaveAgreement(p *SaveAgreements) (*SaveAgreementResponses, error)
	GetUsersAgreements() ([]AgreementResponse, error)
}

var _ Client = (*client)(nil)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
	DoWithBody(req *http.Request, body io.ReadSeeker) (*http.Response, error)
}

// client is the implementation of the Client interface.
type client struct {
	client httpClient
}

// ClientOption defines a function which configures a Client.
type ClientOption func(h *client) error

// HTTPClient returns a function that sets the http client used by the API
// (e.g. if we want to use TLS).
func HTTPClient(c httpClient) func(h *client) error {
	return func(h *client) error {
		h.client = c
		return nil
	}
}

// NewClient returns a new client for plan management.
func NewClient(options ...ClientOption) (Client, error) {
	c := &client{
		client: httpbakery.NewClient(),
	}

	for _, option := range options {
		err := option(c)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return c, nil
}

func getBaseURL() string {
	baseURL := BaseURL
	if termsURL := os.Getenv("JUJU_TERMS"); termsURL != "" {
		baseURL = termsURL
	}
	return baseURL
}

// GetUnsignedTerms returns the default plan for the specified charm.
func (c *client) GetUnsignedTerms(p *CheckAgreementsRequest) ([]GetTermsResponse, error) {
	values := url.Values{}
	for _, t := range p.Terms {
		values.Add("Terms", t)
	}
	u := fmt.Sprintf("%s/agreement?%s", getBaseURL(), values.Encode())
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if response.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, errors.Errorf("failed to get unsigned terms: %v", response.Status)
		}
		return nil, errors.Errorf("failed to get unsigned terms: %v: %s", response.Status, string(b))
	}
	defer discardClose(response)
	var results []GetTermsResponse
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// SaveAgreements saves a user agreement to the specificed terms document.
func (c *client) SaveAgreement(p *SaveAgreements) (*SaveAgreementResponses, error) {
	u := fmt.Sprintf("%s/agreement", getBaseURL())
	req, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/json")
	data, err := json.Marshal(p.Agreements)
	if err != nil {
		return nil, errors.Trace(err)
	}
	response, err := c.client.DoWithBody(req, bytes.NewReader(data))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if response.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, errors.Errorf("failed to save agreement: %v", response.Status)
		}
		return nil, errors.Errorf("failed to save agreement: %v: %s", response.Status, string(b))
	}
	defer discardClose(response)
	var results SaveAgreementResponses
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &results, nil
}

// GetUsersAgreements returns all agreements the user has made.
func (c *client) GetUsersAgreements() ([]AgreementResponse, error) {
	u := fmt.Sprintf("%s/agreements", getBaseURL())
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if response.StatusCode != http.StatusOK {
		b, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, errors.Errorf("failed to get signed agreements: %v", response.Status)
		}
		return nil, errors.Errorf("failed to get signed agreements: %v: %s", response.Status, string(b))
	}
	defer discardClose(response)

	var results []AgreementResponse
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// discardClose reads any remaining data from the response body and closes it.
func discardClose(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
}
