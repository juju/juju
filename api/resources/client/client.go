// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	apicharm "github.com/juju/juju/api/common/charm"
	api "github.com/juju/juju/api/resources"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
)

// TODO(ericsnow) Move FacadeCaller to a component-central package.

// FacadeCaller has the api/base.FacadeCaller methods needed for the component.
type FacadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
	BestAPIVersion() int
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

// CharmID represents the underlying charm for a given application. This
// includes both the URL and the origin.
type CharmID struct {

	// URL of the given charm, includes the reference name and a revision.
	// Old style charm URLs are also supported i.e. charmstore.
	URL *charm.URL

	// Origin holds the origin of a charm. This includes the source of the
	// charm, along with the revision and channel to identify where the charm
	// originated from.
	Origin apicharm.Origin
}

// AddPendingResourcesArgs holds the arguments to AddPendingResources().
type AddPendingResourcesArgs struct {
	// ApplicationID identifies the application being deployed.
	ApplicationID string

	// CharmID identifies the application's charm.
	CharmID CharmID

	// CharmStoreMacaroon is the macaroon to use for the charm when
	// interacting with the charm store.
	CharmStoreMacaroon *macaroon.Macaroon

	// Resources holds the charm store info for each of the resources
	// that should be added/updated on the controller.
	Resources []charmresource.Resource
}

// AddPendingResources sends the provided resource info up to Juju
// without making it available yet.
func (c Client) AddPendingResources(args AddPendingResourcesArgs) ([]string, error) {
	tag := names.NewApplicationTag(args.ApplicationID)
	var apiArgs interface{}
	var err error
	if c.BestAPIVersion() < 2 {
		apiArgs, err = newAddPendingResourcesArgs(tag, args.CharmID, args.CharmStoreMacaroon, args.Resources)
	} else {
		apiArgs, err = newAddPendingResourcesArgsV2(tag, args.CharmID, args.CharmStoreMacaroon, args.Resources)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result params.AddPendingResourcesResult
	if err := c.FacadeCall("AddPendingResources", &apiArgs, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		err := apiservererrors.RestoreError(result.Error)
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
func newAddPendingResourcesArgs(tag names.ApplicationTag, chID CharmID, csMac *macaroon.Macaroon, resources []charmresource.Resource) (params.AddPendingResourcesArgs, error) {
	var args params.AddPendingResourcesArgs
	var apiResources []params.CharmResource
	for _, res := range resources {
		if err := res.Validate(); err != nil {
			return args, errors.Trace(err)
		}
		apiRes := api.CharmResource2API(res)
		apiResources = append(apiResources, apiRes)
	}
	args.Tag = tag.String()
	args.Resources = apiResources
	if chID.URL != nil {
		args.URL = chID.URL.String()
	}
	args.Channel = chID.Origin.CharmChannel().String()
	args.CharmStoreMacaroon = csMac

	return args, nil
}

// newAddPendingResourcesArgsV2 returns the arguments for the
// AddPendingResources APIv2 endpoint.
func newAddPendingResourcesArgsV2(tag names.ApplicationTag, chID CharmID, csMac *macaroon.Macaroon, resources []charmresource.Resource) (params.AddPendingResourcesArgsV2, error) {
	var args params.AddPendingResourcesArgsV2

	var apiResources []params.CharmResource
	for _, res := range resources {
		if err := res.Validate(); err != nil {
			return args, errors.Trace(err)
		}
		apiRes := api.CharmResource2API(res)
		apiResources = append(apiResources, apiRes)
	}
	args.Tag = tag.String()
	args.Resources = apiResources
	if chID.URL != nil {
		args.URL = chID.URL.String()
	}
	args.CharmOrigin = params.CharmOrigin{
		Source:       chID.Origin.Source.String(),
		ID:           chID.Origin.ID,
		Hash:         chID.Origin.Hash,
		Risk:         chID.Origin.Risk,
		Revision:     chID.Origin.Revision,
		Track:        chID.Origin.Track,
		Architecture: chID.Origin.Architecture,
		OS:           chID.Origin.OS,
		Series:       chID.Origin.Series,
	}
	args.CharmStoreMacaroon = csMac
	return args, nil
}

// UploadPendingResource sends the provided resource blob up to Juju
// and makes it available.
func (c Client) UploadPendingResource(application string, res charmresource.Resource, filename string, reader io.ReadSeeker) (pendingID string, err error) {
	if !names.IsValidApplication(application) {
		return "", errors.Errorf("invalid application %q", application)
	}

	ids, err := c.AddPendingResources(AddPendingResourcesArgs{
		ApplicationID: application,
		Resources:     []charmresource.Resource{res},
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	pendingID = ids[0]

	if reader != nil {
		uReq, err := api.NewUploadRequest(application, res.Name, filename, reader)
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
