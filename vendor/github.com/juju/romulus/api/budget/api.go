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

	wireformat "github.com/juju/romulus/wireformat/budget"
)

type httpErrorResponse struct {
	Error string `json:"error"`
}

type httpClient interface {
	DoWithBody(*http.Request, io.ReadSeeker) (*http.Response, error)
}

// NewClient returns a new budget API client using the provided http client.
func NewClient(c httpClient) *client {
	return &client{
		h: c,
	}
}

type client struct {
	h httpClient
}

// CreateBudget creates a new budget with the specified name and limit.
// The call returns the service's response message and an error if one occurred.
func (c *client) CreateBudget(name string, limit string) (string, error) {
	create := wireformat.CreateBudgetRequest{
		Budget: name,
		Limit:  limit,
	}
	var response string
	err := c.doRequest(create, &response)
	return response, err
}

// ListBudgets lists the budgets belonging to the current user.
func (c *client) ListBudgets() (*wireformat.ListBudgetsResponse, error) {
	list := wireformat.ListBudgetsRequest{}
	var response wireformat.ListBudgetsResponse
	err := c.doRequest(list, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// SetBudget updates the budget limit.
func (c *client) SetBudget(budget, limit string) (string, error) {
	set := wireformat.SetBudgetRequest{
		Budget: budget,
		Limit:  limit,
	}
	var response string
	err := c.doRequest(set, &response)
	return response, err
}

// GetBudget returns the information of a particular budget.
func (c *client) GetBudget(budget string) (*wireformat.BudgetWithAllocations, error) {
	get := wireformat.GetBudgetRequest{
		Budget: budget,
	}
	var response wireformat.BudgetWithAllocations
	err := c.doRequest(get, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// CreateAllocation creates a new allocation in a specific budget.
func (c *client) CreateAllocation(budget, limit string, model string, services []string) (string, error) {
	create := wireformat.CreateAllocationRequest{
		Budget:   budget,
		Limit:    limit,
		Model:    model,
		Services: services,
	}
	var response string
	err := c.doRequest(create, &response)
	return response, err
}

// UpdateAllocation updates the allocation associated with the specified service with new limit.
func (c *client) UpdateAllocation(model, service, limit string) (string, error) {
	create := wireformat.UpdateAllocationRequest{
		Limit:       limit,
		Model:       model,
		Application: service,
	}
	var response string
	err := c.doRequest(create, &response)
	return response, err
}

// DeleteAllocation deletes the allocation associated with the specified service.
func (c *client) DeleteAllocation(model, application string) (string, error) {
	create := wireformat.DeleteAllocationRequest{
		Model:       model,
		Application: application,
	}
	var response string
	err := c.doRequest(create, &response)
	return response, err
}

// hasURL is an interface implemented by request structures that
// modify the request URL.
type hasURL interface {
	// URL takes the base URL as a parameter and returns
	// the modified request URL.
	URL() string
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
		reqURL = urlP.URL()
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
				return wireformat.NotAvailError{}
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
		return wireformat.NotAvailError{Resp: resp.StatusCode}
	} else if resp.StatusCode != http.StatusOK {
		response := httpErrorResponse{}
		json.NewDecoder(resp.Body).Decode(&response)
		return wireformat.HttpError{
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
