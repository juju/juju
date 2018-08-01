// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

// The api package contains the interface and implementation of the
// terms service client.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/terms-client/api/wireformat"
)

var BaseURL = "https://api.jujucharms.com/terms"

// Client represents the interface of the terms service client ap client apii.
type Client interface {
	// Saves a Terms and Conditions document under the specified owner/name
	// and returns a term document with the new revision number
	// (only term owner, name and revision are returned).
	SaveTerm(owner, name, content string) (string, error)

	// GetTerm returns the term that matches the specified criteria.
	// If revision is 0, it will return the latest revision of the term.
	GetTerm(owner, name string, revision int) (*wireformat.Term, error)

	// GetUnsignedTerms checks for agreements to the specified terms
	// and returns all terms that the user has not agreed to.
	GetUnsignedTerms(*wireformat.CheckAgreementsRequest) ([]wireformat.GetTermsResponse, error)

	// SaveAgreement saves the users agreement to the specified terms (revision must always be specified).
	SaveAgreement(*wireformat.SaveAgreements) (*wireformat.SaveAgreementResponses, error)

	// GetUsersAgreements returns all agreements the user (the user making the request) has made.
	GetUsersAgreements() ([]wireformat.AgreementResponse, error)

	// Publish publishes the owned term identified by input parameters
	// and returns the published term id.
	// Only owned terms require publishing.
	Publish(owner, name string, revision int) (string, error)
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
	DoWithBody(req *http.Request, body io.ReadSeeker) (*http.Response, error)
}

// ClientOption defines a function which configures a Client.
type ClientOption func(h *client)

// HTTPClient returns a function that sets the http client used by the API
// (e.g. if we want to use TLS).
func HTTPClient(c httpClient) ClientOption {
	return func(h *client) {
		h.bclient = c
	}
}

// ServiceURL returns a function that sets the terms service URL used
// by the API.
func ServiceURL(serviceURL string) ClientOption {
	return func(h *client) {
		h.serviceURL = serviceURL
	}
}

// NewClient returns a terms service api client.
func NewClient(options ...ClientOption) (Client, error) {
	bakeryClient := httpbakery.NewClient()
	c := &client{
		serviceURL: getBaseURL(),
		bclient:    bakeryClient,
	}
	for _, option := range options {
		option(c)
	}
	return c, nil
}

type client struct {
	serviceURL string
	bclient    httpClient
}

func unmarshalError(data []byte) (string, error) {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	err := json.Unmarshal(data, &e)
	if err != nil {
		return "", errors.Trace(err)
	}
	if e.Error != "" {
		return e.Error, nil
	}
	return e.Message, nil
}

// Publish publishes the owned term identified by input parameters
// and returns the published term id.
func (c *client) Publish(owner, name string, revision int) (string, error) {
	fail := func(err error) (string, error) {
		return "", err
	}
	if owner == "" {
		return fmt.Sprintf("%s/%d", name, revision), nil
	}
	termURL := fmt.Sprintf("%s/v1/terms/%s/%s/%d/publish", c.serviceURL, owner, name, revision)

	req, err := http.NewRequest("POST", termURL, nil)
	if err != nil {
		return fail(errors.Trace(err))
	}

	response, err := c.bclient.DoWithBody(req, nil)
	if err != nil {
		return fail(errors.Trace(err))
	}
	defer discardClose(response)
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return fail(errors.Trace(err))
	}
	if response.StatusCode != http.StatusOK {
		message, uerr := unmarshalError(data)
		if uerr != nil {
			return fail(errors.New(string(data)))
		}
		return fail(errors.New(message))
	}
	var id struct {
		TermID string `json:"term-id"`
	}
	err = json.Unmarshal(data, &id)
	if err != nil {
		return fail(errors.Trace(err))
	}
	return id.TermID, nil
}

// GetTerm implements the Client interface. It returns the term that
// matches the specified criteria. If revision is 0, it will return the
// latest revision of the term.
func (c *client) GetTerm(owner, name string, revision int) (*wireformat.Term, error) {
	termURL, err := appendTermURL(c.serviceURL, owner, name, revision)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req, err := http.NewRequest("GET", termURL.String(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	response, err := c.bclient.Do(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer discardClose(response)
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if response.StatusCode != http.StatusOK {
		message, uerr := unmarshalError(data)
		if uerr != nil {
			return nil, errors.New(string(data))
		}
		return nil, errors.New(message)
	}
	var terms []wireformat.Term
	err = json.Unmarshal(data, &terms)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(terms) == 0 {
		return nil, errors.NotFoundf("term")
	}
	return &terms[0], nil
}

// SaveTerm implements the Client interface. It saves a Terms and Conditions document
// under the specified owner/name and returns a term document with the new revision number
// (only term owner, name and revision are returned).
func (c *client) SaveTerm(owner, name, content string) (string, error) {
	termURL, err := appendTermURL(c.serviceURL, owner, name, 0)
	if err != nil {
		return "", errors.Trace(err)
	}

	term := wireformat.SaveTerm{
		Content: content,
	}
	data, err := json.Marshal(term)
	if err != nil {
		return "", errors.Trace(err)
	}

	req, err := http.NewRequest("POST", termURL.String(), nil)
	if err != nil {
		return "", errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := c.bclient.DoWithBody(req, bytes.NewReader(data))
	if err != nil {
		return "", errors.Trace(err)
	}
	defer discardClose(response)
	data, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return "", errors.Trace(err)
	}
	if response.StatusCode != http.StatusOK {
		message, uerr := unmarshalError(data)
		if uerr != nil {
			return "", errors.New(string(data))
		}
		return "", errors.New(message)
	}
	var savedTerm wireformat.TermIDResponse
	err = json.Unmarshal(data, &savedTerm)
	if err != nil {
		return "", errors.Trace(err)
	}
	return savedTerm.TermID, nil
}

// GetUsersAgreements implements the Client interface. It returns all
// agreements the user (the user making the request) has made.
func (c *client) GetUsersAgreements() ([]wireformat.AgreementResponse, error) {
	u := fmt.Sprintf("%s/v1/agreements", c.serviceURL)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	response, err := c.bclient.Do(req)
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

	var results []wireformat.AgreementResponse
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// SaveAgreement implements the Client interface. It saves the users
// agreement to the specified term (revision must always be specified).
func (c *client) SaveAgreement(request *wireformat.SaveAgreements) (*wireformat.SaveAgreementResponses, error) {
	u := fmt.Sprintf("%s/v1/agreement", c.serviceURL)
	req, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/json")
	data, err := json.Marshal(request.Agreements)
	if err != nil {
		return nil, errors.Trace(err)
	}
	response, err := c.bclient.DoWithBody(req, bytes.NewReader(data))
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
	var results wireformat.SaveAgreementResponses
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &results, nil
}

// GetUnsignedTerms implements the Client interface. It checks for agreements
// to the specified terms and returns all terms that the user has not agreed
// to.
func (c *client) GetUnsignedTerms(terms *wireformat.CheckAgreementsRequest) ([]wireformat.GetTermsResponse, error) {
	values := url.Values{}
	for _, t := range terms.Terms {
		values.Add("Terms", t)
	}
	u := fmt.Sprintf("%s/v1/agreement?%s", c.serviceURL, values.Encode())
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.bclient.Do(req)
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
	var results []wireformat.GetTermsResponse
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

func getBaseURL() string {
	baseURL := BaseURL
	if termsURL := os.Getenv("JUJU_TERMS"); termsURL != "" {
		baseURL = termsURL
	}
	return baseURL
}

func appendTermURL(baseURLStr, owner, term string, revision int) (*url.URL, error) {
	b, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot parse %q", baseURLStr)
	}
	b.Path = strings.TrimSuffix(b.Path, "/") + "/v1/terms"
	if owner != "" {
		b.Path = b.Path + "/" + strings.TrimPrefix(owner, "/")
	}
	if term == "" {
		return nil, errors.New("empty term name")
	}
	b.Path = strings.TrimSuffix(b.Path, "/") + "/" + strings.TrimPrefix(term, "/")
	if revision != 0 {
		values := b.Query()
		values.Set("revision", strconv.FormatInt(int64(revision), 10))
		b.RawQuery = values.Encode()
	}
	return b, nil
}

// discardClose reads any remaining data from the response body and closes it.
func discardClose(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
}
