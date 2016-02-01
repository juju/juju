// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

var logger = loggo.GetLogger("juju.resource.api.client")

// TODO(ericsnow) Move FacadeCaller to a component-central package.

// FacadeCaller has the api/base.FacadeCaller methods needed for the component.
type FacadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

// Doer
type Doer interface {
	Do(req *http.Request, body io.ReadSeeker, resp interface{}) error
}

// Client is the public client for the resources API facade.
type Client struct {
	FacadeCaller
	io.Closer
	doer Doer
}

// NewClient returns a new Client for the given raw API caller.
func NewClient(caller FacadeCaller, doer Doer, closer io.Closer) *Client {
	return &Client{
		FacadeCaller: caller,
		Closer:       closer,
		doer:         doer,
	}
}

// ListResources calls the ListResources API server method with
// the given service names.
func (c Client) ListResources(services []string) ([]resource.ServiceResources, error) {
	args, err := api.NewListResourcesArgs(services)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var apiResults api.ResourcesResults
	if err := c.FacadeCall("ListResources", &args, &apiResults); err != nil {
		return nil, errors.Trace(err)
	}

	if len(apiResults.Results) != len(services) {
		// We don't bother returning the results we *did* get since
		// something bad happened on the server.
		return nil, errors.Errorf("got invalid data from server (expected %d results, got %d)", len(services), len(apiResults.Results))
	}

	var errs []error
	results := make([]resource.ServiceResources, len(services))
	for i := range services {
		apiResult := apiResults.Results[i]

		result, err := api.APIResult2ServiceResources(apiResult)
		if err != nil {
			errs = append(errs, errors.Trace(err))
		}
		results[i] = result
	}
	if err := resolveErrors(errs); err != nil {
		return nil, errors.Trace(err)
	}

	return results, nil
}

// Upload sends the provided resource blob up to Juju.
func (c Client) Upload(service, name string, reader io.ReadSeeker) error {
	uReq, err := api.NewUploadRequest(service, name, reader)
	if err != nil {
		return errors.Trace(err)
	}
	req, err := uReq.HTTPRequest()
	if err != nil {
		return errors.Trace(err)
	}

	var response api.UploadResult // ignored
	if err := c.doer.Do(req, reader, &response); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// PreUpload sends the provided resource blob up to Juju without making
// it available yet. For example, PreUpload() is used before the service is deployed.
func (c Client) PreUpload(service, name string, reader io.ReadSeeker) (uploadID string, err error) {
	uReq, err := api.NewUploadRequest(service, name, reader)
	if err != nil {
		return "", errors.Trace(err)
	}
	uReq.PreUpload = true
	req, err := uReq.HTTPRequest()
	if err != nil {
		return "", errors.Trace(err)
	}

	var response api.UploadResult
	if err := c.doer.Do(req, reader, &response); err != nil {
		return "", errors.Trace(err)
	}

	return response.UploadID, nil
}

func resolveErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return errors.New(strings.Join(msgs, "\n"))
	}
}
