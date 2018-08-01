// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package budget contains the budget service API client.
package budget

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/romulus"
	wireformat "github.com/juju/romulus/wireformat/budget"
	"github.com/juju/romulus/wireformat/common"
)

type httpErrorResponse struct {
	Error string `json:"error"`
}

type httpClient interface {
	DoWithBody(*http.Request, io.ReadSeeker) (*http.Response, error)
}

type client struct {
	apiRoot string
	h       httpClient
}

// ClientOption defines a function which configures a Client.
type ClientOption func(h *client) error

// HTTPClient returns a function that sets the http client used by the API
// (e.g. if we want to use TLS).
func HTTPClient(h httpClient) func(h *client) error {
	return func(c *client) error {
		c.h = h
		return nil
	}
}

// APIRoot sets the base url for the api client.
func APIRoot(apiRoot string) func(h *client) error {
	return func(c *client) error {
		c.apiRoot = apiRoot
		return nil
	}
}

// NewClient returns a new budget API client using the provided http client.
func NewClient(options ...ClientOption) (*client, error) {
	c := &client{
		h:       httpbakery.NewClient(),
		apiRoot: romulus.DefaultAPIRoot,
	}

	for _, option := range options {
		err := option(c)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return c, nil
}

// CreateWallet creates a new wallet with the specified name and limit.
// The call returns the service's response message and an error if one occurred.
func (c *client) CreateWallet(name string, limit string) (string, error) {
	create := wireformat.CreateWalletRequest{
		Wallet: name,
		Limit:  limit,
	}
	var response string
	err := c.doRequest(create, &response)
	return response, err
}

// ListWallets lists the wallets belonging to the current user.
func (c *client) ListWallets() (*wireformat.ListWalletsResponse, error) {
	list := wireformat.ListWalletsRequest{}
	var response wireformat.ListWalletsResponse
	err := c.doRequest(list, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// SetWallet updates the wallet limit.
func (c *client) SetWallet(wallet, limit string) (string, error) {
	set := wireformat.SetWalletRequest{
		Wallet: wallet,
		Limit:  limit,
	}
	var response string
	err := c.doRequest(set, &response)
	return response, err
}

// GetWallet returns the information of a particular wallet.
func (c *client) GetWallet(wallet string) (*wireformat.WalletWithBudgets, error) {
	get := wireformat.GetWalletRequest{
		Wallet: wallet,
	}
	var response wireformat.WalletWithBudgets
	err := c.doRequest(get, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// CreateBudget creates a new budget in a specific wallet.
func (c *client) CreateBudget(wallet, limit string, model string) (string, error) {
	create := wireformat.CreateBudgetRequest{
		Wallet: wallet,
		Limit:  limit,
		Model:  model,
	}
	var response string
	err := c.doRequest(create, &response)
	return response, err
}

// UpdateBudget updates the budget associated with the specified model with new limit.
func (c *client) UpdateBudget(model, wallet, limit string) (string, error) {
	create := wireformat.UpdateBudgetRequest{
		Limit:  limit,
		Model:  model,
		Wallet: wallet,
	}
	var response string
	err := c.doRequest(create, &response)
	return response, err
}

// DeleteBudget deletes the budget associated with the specified model.
func (c *client) DeleteBudget(model string) (string, error) {
	create := wireformat.DeleteBudgetRequest{
		Model: model,
	}
	var response string
	err := c.doRequest(create, &response)
	return response, err
}

// hasURL is an interface implemented by request structures that
// modify the request URL.
type hasURL interface {
	URL(apiRoot string) string
}

// hasBody is an interface implemented by requests that send
// data in the request body
type hasBody interface {
	// Body returns the request body value.
	Body() interface{}
}

// hasMethod is an interface implemented by requests to
// specify the request method.
type hasMethod interface {
	// Method returns the request method.
	Method() string
}

// hasContentType is an interface implemented by requests to
// specify the content-type header to be set.
type hasContentType interface {
	ContentType() string
}

// doRequest executes a generic request, retrieving relevant information
// from the req interface. If result is not nil, the response will be
// decoded to it.
func (c *client) doRequest(req interface{}, result interface{}) error {
	reqURL := ""
	if urlP, ok := req.(hasURL); ok {
		reqURL = urlP.URL(c.apiRoot)
	} else {
		return errors.Errorf("unknown request URL")
	}

	u, err := url.Parse(reqURL)
	if err != nil {
		return errors.Trace(err)
	}

	method := "GET"
	if methodP, ok := req.(hasMethod); ok {
		method = methodP.Method()
	}

	var resp *http.Response
	if bodyP, ok := req.(hasBody); ok {
		reqBody := bodyP.Body()

		payload := &bytes.Buffer{}
		err = json.NewEncoder(payload).Encode(reqBody)
		if err != nil {
			return errors.Annotate(err, "failed to encode request")
		}
		r, err := http.NewRequest(method, u.String(), nil)
		if err != nil {
			return errors.Annotate(err, "failed to create request")
		}
		if ctype, ok := req.(hasContentType); ok {
			r.Header.Add("Content-Type", ctype.ContentType())
		}
		resp, err = c.h.DoWithBody(r, bytes.NewReader(payload.Bytes()))
		if err != nil {
			if strings.HasSuffix(err.Error(), "Connection refused") {
				return common.NotAvailError{}
			}
			return errors.Annotate(err, "failed to execute request")
		}
		defer discardClose(resp)
	} else {
		r, err := http.NewRequest(method, u.String(), nil)
		if err != nil {
			return errors.Annotate(err, "failed to create request")
		}
		resp, err = c.h.DoWithBody(r, nil)
		if err != nil {
			return errors.Annotate(err, "failed to execute request")
		}
		defer discardClose(resp)
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return common.NotAvailError{StatusCode: resp.StatusCode}
	} else if resp.StatusCode != http.StatusOK {
		response := httpErrorResponse{}
		json.NewDecoder(resp.Body).Decode(&response)
		return common.HTTPError{
			StatusCode: resp.StatusCode,
			Message:    response.Error,
		}

	}
	if result != nil {
		err = json.NewDecoder(resp.Body).Decode(result)
		if err != nil {
			return errors.Annotate(err, "failed to decode response")
		}
	}
	return nil
}

func discardClose(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
}
