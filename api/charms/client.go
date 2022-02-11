// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package charms provides a client for accessing the charms API.
package charms

import (
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	apicharm "github.com/juju/juju/api/common/charm"
	commoncharms "github.com/juju/juju/api/common/charms"
	api "github.com/juju/juju/api/resources"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the charms API end point.
type Client struct {
	base.ClientFacade
	*commoncharms.CharmInfoClient
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charms API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Charms")
	commonClient := commoncharms.NewCharmInfoClient(backend)
	return &Client{ClientFacade: frontend, CharmInfoClient: commonClient, facade: backend}
}

// IsMetered returns whether or not the charm is metered.
func (c *Client) IsMetered(charmURL string) (bool, error) {
	args := params.CharmURL{URL: charmURL}
	var metered params.IsMeteredResult
	if err := c.facade.FacadeCall("IsMetered", args, &metered); err != nil {
		return false, errors.Trace(err)
	}
	return metered.Metered, nil
}

// CharmToResolve holds the charm url and it's channel to be resolved.
type CharmToResolve struct {
	URL         *charm.URL
	Origin      apicharm.Origin
	SwitchCharm bool
}

// ResolvedCharm holds resolved charm data.
type ResolvedCharm struct {
	URL             *charm.URL
	Origin          apicharm.Origin
	SupportedSeries []string
	Error           error
}

// ResolveCharms resolves the given charm URLs with an optionally specified
// preferred channel.
// ResolveCharms is only supported in version 3 and above, it is expected that
// the consumer of the client is intended to handle the fallback.
func (c *Client) ResolveCharms(charms []CharmToResolve) ([]ResolvedCharm, error) {
	if c.facade.BestAPIVersion() < 3 {
		return nil, errors.NotSupportedf("resolve charms")
	}

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
	var result params.ResolveCharmWithChannelResults
	if err := c.facade.FacadeCall("ResolveCharms", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	resolvedCharms := make([]ResolvedCharm, len(charms))
	for i, r := range result.Results {
		if r.Error != nil {
			resolvedCharms[i] = ResolvedCharm{Error: r.Error}
			continue
		}
		curl, err := charm.ParseURL(r.URL)
		if err != nil {
			resolvedCharms[i] = ResolvedCharm{Error: err}
		}
		origin := apicharm.APICharmOrigin(r.Origin)
		resolvedCharms[i] = ResolvedCharm{
			URL:             curl,
			Origin:          origin,
			SupportedSeries: r.SupportedSeries,
		}
	}
	return resolvedCharms, nil
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
func (c *Client) GetDownloadInfo(curl *charm.URL, origin apicharm.Origin, mac *macaroon.Macaroon) (DownloadInfo, error) {
	if c.facade.BestAPIVersion() < 3 {
		return DownloadInfo{}, errors.NotSupportedf("get download info")
	}

	args := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{{
			CharmURL: curl.String(),
			Origin:   origin.ParamsCharmOrigin(),
			Macaroon: mac,
		}},
	}
	var results params.DownloadInfoResults
	if err := c.facade.FacadeCall("GetDownloadInfos", args, &results); err != nil {
		return DownloadInfo{}, errors.Trace(err)
	}
	if num := len(results.Results); num != 1 {
		return DownloadInfo{}, errors.Errorf("expected one result, received %d", num)
	}
	result := results.Results[0]
	return DownloadInfo{
		URL:    result.URL,
		Origin: apicharm.APICharmOrigin(result.Origin),
	}, nil
}

// AddCharm adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store and charm hub URLs. See also AddLocalCharm()
// in the client-side API.
//
// If the AddCharm API call fails because of an authorization error
// when retrieving the charm from the charm store, an error
// satisfying params.IsCodeUnauthorized will be returned.
func (c *Client) AddCharm(curl *charm.URL, origin apicharm.Origin, force bool) (apicharm.Origin, error) {
	args := params.AddCharmWithOrigin{
		URL:    curl.String(),
		Origin: origin.ParamsCharmOrigin(),
		Force:  force,
		// Deprecated: Series is used here to communicate with older
		// controllers and instead we use Origin to describe the platform.
		Series: origin.Series,
	}
	var result params.CharmOriginResult
	if err := c.facade.FacadeCall("AddCharm", args, &result); err != nil {
		return apicharm.Origin{}, errors.Trace(err)
	}
	return apicharm.APICharmOrigin(result.Origin), nil
}

// AddCharmWithAuthorization is like AddCharm except it also provides
// the given charmstore macaroon for the juju server to use when
// obtaining the charm from the charm store or from charm hub. The
// macaroon is conventionally obtained from the /delegatable-macaroon
// endpoint in the charm store.
//
// If the AddCharmWithAuthorization API call fails because of an
// authorization error when retrieving the charm from the charm store,
// an error satisfying params.IsCodeUnauthorized will be returned.
// Force is used to overload any validation errors that could occur during
// a deploy
func (c *Client) AddCharmWithAuthorization(curl *charm.URL, origin apicharm.Origin, csMac *macaroon.Macaroon, force bool) (apicharm.Origin, error) {
	args := params.AddCharmWithAuth{
		URL:                curl.String(),
		Origin:             origin.ParamsCharmOrigin(),
		CharmStoreMacaroon: csMac,
		Force:              force,
		// Deprecated: Series is used here to communicate with older
		// controllers and instead we use Origin to describe the platform.
		Series: origin.Series,
	}
	var result params.CharmOriginResult
	if err := c.facade.FacadeCall("AddCharmWithAuthorization", args, &result); err != nil {
		return apicharm.Origin{}, errors.Trace(err)
	}
	return apicharm.APICharmOrigin(result.Origin), nil
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
	if err := c.facade.FacadeCall("CheckCharmPlacement", args, &result); err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

// ListCharmResources returns a list of associated resources for a given charm.
func (c *Client) ListCharmResources(curl *charm.URL, origin apicharm.Origin) ([]charmresource.Resource, error) {
	if c.facade.BestAPIVersion() < 3 {
		return nil, errors.NotSupportedf("list resources")
	}

	args := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{{
			CharmURL: curl.String(),
			Origin:   origin.ParamsCharmOrigin(),
		}},
	}
	var results params.CharmResourcesResults
	if err := c.facade.FacadeCall("ListCharmResources", args, &results); err != nil {
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
