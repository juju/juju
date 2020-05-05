// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"io"
	"net/http"
	"strings"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// TODO(ericsnow) Move FacadeCaller to a component-central package.

// FacadeCaller has the api/base.FacadeCaller methods needed for the component.
type FacadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

// Doer
type Doer interface {
	Do(ctx context.Context, req *http.Request, resp interface{}) error
}

// Client is the public client for the resources API facade.
type Client struct {
	FacadeCaller
	io.Closer
	doer Doer
	ctx  context.Context
}

// NewClient returns a new Client for the given raw API caller.
func NewClient(ctx context.Context, caller FacadeCaller, doer Doer, closer io.Closer) *Client {
	return &Client{
		FacadeCaller: caller,
		Closer:       closer,
		doer:         doer,
		ctx:          ctx,
	}
}

// ListResources calls the ListResources API server method with
// the given application names.
func (c Client) ListResources(applications []string) ([]resource.ApplicationResources, error) {
	args, err := newListResourcesArgs(applications)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var apiResults params.ResourcesResults
	if err := c.FacadeCall("ListResources", &args, &apiResults); err != nil {
		return nil, errors.Trace(err)
	}

	if len(apiResults.Results) != len(applications) {
		// We don't bother returning the results we *did* get since
		// something bad happened on the server.
		return nil, errors.Errorf("got invalid data from server (expected %d results, got %d)", len(applications), len(apiResults.Results))
	}

	var errs []error
	results := make([]resource.ApplicationResources, len(applications))
	for i := range applications {
		apiResult := apiResults.Results[i]

		result, err := api.APIResult2ApplicationResources(apiResult)
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

// newListResourcesArgs returns the arguments for the ListResources endpoint.
func newListResourcesArgs(applications []string) (params.ListResourcesArgs, error) {
	var args params.ListResourcesArgs
	var errs []error
	for _, application := range applications {
		if !names.IsValidApplication(application) {
			err := errors.Errorf("invalid application %q", application)
			errs = append(errs, err)
			continue
		}
		args.Entities = append(args.Entities, params.Entity{
			Tag: names.NewApplicationTag(application).String(),
		})
	}
	if err := resolveErrors(errs); err != nil {
		return args, errors.Trace(err)
	}
	return args, nil
}

// Upload sends the provided resource blob up to Juju.
func (c Client) Upload(application, name, filename string, reader io.ReadSeeker) error {
	uReq, err := api.NewUploadRequest(application, name, filename, reader)
	if err != nil {
		return errors.Trace(err)
	}
	req, err := uReq.HTTPRequest()
	if err != nil {
		return errors.Trace(err)
	}

	var response params.UploadResult // ignored
	if err := c.doer.Do(c.ctx, req, &response); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// AddPendingResourcesArgs holds the arguments to AddPendingResources().
type AddPendingResourcesArgs struct {
	// ApplicationID identifies the application being deployed.
	ApplicationID string

	// CharmID identifies the application's charm.
	CharmID charmstore.CharmID

	// CharmStoreMacaroon is the macaroon to use for the charm when
	// interacting with the charm store.
	CharmStoreMacaroon *macaroon.Macaroon

	// Resources holds the charm store info for each of the resources
	// that should be added/updated on the controller.
	Resources []charmresource.Resource
}

// AddPendingResources sends the provided resource info up to Juju
// without making it available yet.
func (c Client) AddPendingResources(args AddPendingResourcesArgs) (pendingIDs []string, err error) {
	apiArgs, err := newAddPendingResourcesArgs(args.ApplicationID, args.CharmID, args.CharmStoreMacaroon, args.Resources)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result params.AddPendingResourcesResult
	if err := c.FacadeCall("AddPendingResources", &apiArgs, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		err := common.RestoreError(result.Error)
		return nil, errors.Trace(err)
	}

	if len(result.PendingIDs) != len(args.Resources) {
		return nil, errors.Errorf("bad data from server: expected %d IDs, got %d", len(args.Resources), len(result.PendingIDs))
	}
	for i, id := range result.PendingIDs {
		if id == "" {
			return nil, errors.Errorf("bad data from server: got an empty ID for resource %q", args.Resources[i].Name)
		}
		// TODO(ericsnow) Do other validation?
	}

	return result.PendingIDs, nil
}

// newAddPendingResourcesArgs returns the arguments for the
// AddPendingResources API endpoint.
func newAddPendingResourcesArgs(applicationID string, chID charmstore.CharmID, csMac *macaroon.Macaroon, resources []charmresource.Resource) (params.AddPendingResourcesArgs, error) {
	var args params.AddPendingResourcesArgs

	if !names.IsValidApplication(applicationID) {
		return args, errors.Errorf("invalid application %q", applicationID)
	}
	tag := names.NewApplicationTag(applicationID).String()

	var apiResources []params.CharmResource
	for _, res := range resources {
		if err := res.Validate(); err != nil {
			return args, errors.Trace(err)
		}
		apiRes := api.CharmResource2API(res)
		apiResources = append(apiResources, apiRes)
	}
	args.Tag = tag
	args.Resources = apiResources
	if chID.URL != nil {
		args.URL = chID.URL.String()
		args.Channel = string(chID.Channel)
		args.CharmStoreMacaroon = csMac
	}
	return args, nil
}

// UploadPendingResource sends the provided resource blob up to Juju
// and makes it available.
func (c Client) UploadPendingResource(applicationID string, res charmresource.Resource, filename string, reader io.ReadSeeker) (pendingID string, err error) {
	ids, err := c.AddPendingResources(AddPendingResourcesArgs{
		ApplicationID: applicationID,
		Resources:     []charmresource.Resource{res},
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	pendingID = ids[0]

	if reader != nil {
		uReq, err := api.NewUploadRequest(applicationID, res.Name, filename, reader)
		if err != nil {
			return "", errors.Trace(err)
		}
		uReq.PendingID = pendingID
		req, err := uReq.HTTPRequest()
		if err != nil {
			return "", errors.Trace(err)
		}

		var response params.UploadResult // ignored
		if err := c.doer.Do(c.ctx, req, &response); err != nil {
			return "", errors.Trace(err)
		}
	}

	return pendingID, nil
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
