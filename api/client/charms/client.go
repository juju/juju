// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"

	"github.com/juju/charm/v13"
	charmresource "github.com/juju/charm/v13/resource"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/client/resources"
	apicharm "github.com/juju/juju/api/common/charm"
	commoncharms "github.com/juju/juju/api/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client allows access to the charms API endpoint.
type Client struct {
	base.ClientFacade
	*commoncharms.CharmInfoClient
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charms API.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "Charms", options...)
	commonClient := commoncharms.NewCharmInfoClient(backend)
	return &Client{ClientFacade: frontend, CharmInfoClient: commonClient, facade: backend}
}

// CharmToResolve holds the charm url and it's channel to be resolved.
type CharmToResolve struct {
	URL         *charm.URL
	Origin      apicharm.Origin
	SwitchCharm bool
}

// ResolvedCharm holds resolved charm data.
type ResolvedCharm struct {
	URL            *charm.URL
	Origin         apicharm.Origin
	SupportedBases []corebase.Base
	Error          error
}

// ResolveCharms resolves the given charm URLs with an optionally specified
// preferred channel.
func (c *Client) ResolveCharms(charms []CharmToResolve) ([]ResolvedCharm, error) {
	args := params.ResolveCharmsWithChannel{
		Resolve: make([]params.ResolveCharmWithChannel, len(charms)),
	}
	for i, ch := range charms {
		args.Resolve[i] = params.ResolveCharmWithChannel{
			Reference:   ch.URL.String(),
			Origin:      ch.Origin.ParamsCharmOrigin(),
			SwitchCharm: ch.SwitchCharm,
		}
	}
	if c.BestAPIVersion() < 7 {
		var result params.ResolveCharmWithChannelResultsV6
		if err := c.facade.FacadeCall(context.TODO(), "ResolveCharms", args, &result); err != nil {
			return nil, errors.Trace(apiservererrors.RestoreError(err))
		}
		return transform.Slice(result.Results, c.resolveCharmV6), nil
	}

	var result params.ResolveCharmWithChannelResults
	if err := c.facade.FacadeCall(context.TODO(), "ResolveCharms", args, &result); err != nil {
		return nil, errors.Trace(apiservererrors.RestoreError(err))
	}
	return transform.Slice(result.Results, c.resolveCharm), nil
}

func (c *Client) resolveCharm(r params.ResolveCharmWithChannelResult) ResolvedCharm {
	if r.Error != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(r.Error)}
	}
	curl, err := charm.ParseURL(r.URL)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	origin, err := apicharm.APICharmOrigin(r.Origin)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}

	supportedBases, err := transform.SliceOrErr(r.SupportedBases, func(in params.Base) (corebase.Base, error) {
		return corebase.ParseBase(in.Name, in.Channel)
	})
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	return ResolvedCharm{
		URL:            curl,
		Origin:         origin,
		SupportedBases: supportedBases,
	}
}

func (c *Client) resolveCharmV6(r params.ResolveCharmWithChannelResultV6) ResolvedCharm {
	if r.Error != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(r.Error)}
	}
	curl, err := charm.ParseURL(r.URL)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	origin, err := apicharm.APICharmOrigin(r.Origin)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	supportedBases, err := transform.SliceOrErr(r.SupportedSeries, corebase.GetBaseFromSeries)
	if err != nil {
		return ResolvedCharm{Error: apiservererrors.RestoreError(err)}
	}
	return ResolvedCharm{
		URL:            curl,
		Origin:         origin,
		SupportedBases: supportedBases,
	}
}

// DownloadInfo holds the URL and Origin for a charm that requires downloading
// on the client side. This is mainly for bundles as we don't resolve bundles
// on the server.
type DownloadInfo struct {
	URL    string
	Origin apicharm.Origin
}

// GetDownloadInfo will get a download information from the given charm URL
// using the appropriate charm store.
func (c *Client) GetDownloadInfo(curl *charm.URL, origin apicharm.Origin) (DownloadInfo, error) {
	args := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{{
			CharmURL: curl.String(),
			Origin:   origin.ParamsCharmOrigin(),
		}},
	}
	var results params.DownloadInfoResults
	if err := c.facade.FacadeCall(context.TODO(), "GetDownloadInfos", args, &results); err != nil {
		return DownloadInfo{}, errors.Trace(err)
	}
	if num := len(results.Results); num != 1 {
		return DownloadInfo{}, errors.Errorf("expected one result, received %d", num)
	}
	result := results.Results[0]
	origin, err := apicharm.APICharmOrigin(result.Origin)
	if err != nil {
		return DownloadInfo{}, errors.Trace(err)
	}
	return DownloadInfo{
		URL:    result.URL,
		Origin: origin,
	}, nil
}

// AddCharm adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store and charm hub URLs. See also AddLocalCharm().
//
// If the AddCharm API call fails because of an authorization error
// when retrieving the charm from the charm store, an error
// satisfying params.IsCodeUnauthorized will be returned.
func (c *Client) AddCharm(curl *charm.URL, origin apicharm.Origin, force bool) (apicharm.Origin, error) {
	args := params.AddCharmWithOrigin{
		URL:    curl.String(),
		Origin: origin.ParamsCharmOrigin(),
		Force:  force,
	}
	var result params.CharmOriginResult
	if err := c.facade.FacadeCall(context.TODO(), "AddCharm", args, &result); err != nil {
		return apicharm.Origin{}, errors.Trace(err)
	}
	return apicharm.APICharmOrigin(result.Origin)
}

// CheckCharmPlacement checks to see if a charm can be placed into the
// application. If the application doesn't exist then it is considered fine to
// be placed there.
func (c *Client) CheckCharmPlacement(applicationName string, curl *charm.URL) error {
	args := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: applicationName,
			CharmURL:    curl.String(),
		}},
	}
	var result params.ErrorResults
	if err := c.facade.FacadeCall(context.TODO(), "CheckCharmPlacement", args, &result); err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

// ListCharmResources returns a list of associated resources for a given charm.
func (c *Client) ListCharmResources(curl string, origin apicharm.Origin) ([]charmresource.Resource, error) {
	args := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{{
			CharmURL: curl,
			Origin:   origin.ParamsCharmOrigin(),
		}},
	}
	var results params.CharmResourcesResults
	if err := c.facade.FacadeCall(context.TODO(), "ListCharmResources", args, &results); err != nil {
		return nil, errors.Trace(err)
	}

	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, received %d", n)
	}

	result := results.Results[0]
	resources := make([]charmresource.Resource, len(result))
	for i, res := range result {
		if res.Error != nil {
			return nil, errors.Trace(res.Error)
		}

		chRes, err := api.API2CharmResource(res.CharmResource)
		if err != nil {
			return nil, errors.Annotate(err, "unexpected charm resource")
		}
		resources[i] = chRes
	}

	return resources, nil
}
