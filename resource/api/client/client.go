// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

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
	doer    Doer
	envUUID string
}

// NewClient returns a new Client for the given raw API caller.
func NewClient(caller FacadeCaller, doer Doer, envUUID string, closer io.Closer) *Client {
	return &Client{
		FacadeCaller: caller,
		Closer:       closer,
		doer:         doer,
		envUUID:      envUUID,
	}
}

// ListResources calls the ListResources API server method with
// the given service names.
func (c Client) ListResources(services []string) ([][]resource.Resource, error) {
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
	results := make([][]resource.Resource, len(services))
	for i := range services {
		apiResult := apiResults.Results[i]

		result, err := api.APIResult2Resources(apiResult)
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
	if !names.IsValidService(service) {
		return errors.Errorf("invalid service %q", service)
	}
	path := fmt.Sprintf(api.HTTPEndpointPath, c.envUUID, service, name)

	req, err := http.NewRequest("PUT", path, nil)
	if err != nil {
		return errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	fp, size, err := inspectData(reader)
	if err != nil {
		return errors.Trace(err)
	}
	req.URL.Query().Set("fingerprint", fp.String())
	req.URL.Query().Set("size", strconv.FormatInt(size, 10))

	if err := c.doer.Do(req, reader, nil); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func inspectData(reader io.ReadSeeker) (charmresource.Fingerprint, int64, error) {
	var fp charmresource.Fingerprint

	// TODO(ericsnow) We need to stream the data through rather than
	// writing it to memory.
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return fp, 0, errors.Trace(err)
	}
	fp, err = charmresource.GenerateFingerprint(data)
	if err != nil {
		return fp, 0, errors.Trace(err)
	}
	size := int64(len(data))

	_, err = reader.Seek(0, os.SEEK_SET)
	if err != nil {
		return fp, 0, errors.Trace(err)
	}

	return fp, size, nil
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
