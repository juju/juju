// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// charms provides a client for accessing the charms API.
package charms

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/apiserver/params"
)

// Client allows access to the charms API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charms API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Charms")
	return &Client{ClientFacade: frontend, facade: backend}
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
	URL    *charm.URL
	Origin apicharm.Origin
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
func (c *Client) ResolveCharms(charms []CharmToResolve) ([]ResolvedCharm, error) {
	args := params.ResolveCharmsWithChannel{
		Resolve: make([]params.ResolveCharmWithChannel, len(charms)),
	}
	for i, ch := range charms {
		args.Resolve[i] = params.ResolveCharmWithChannel{
			Reference: ch.URL.String(),
			Origin:    ch.Origin.ParamsCharmOrigin(),
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

// CharmInfo holds information about a charm.
type CharmInfo struct {
	Revision   int
	URL        string
	Config     *charm.Config
	Meta       *charm.Meta
	Actions    *charm.Actions
	Metrics    *charm.Metrics
	LXDProfile *charm.LXDProfile
}

// CharmInfo returns information about the requested charm.
func (c *Client) CharmInfo(charmURL string) (*CharmInfo, error) {
	args := params.CharmURL{URL: charmURL}
	var info params.Charm
	if err := c.facade.FacadeCall("CharmInfo", args, &info); err != nil {
		return nil, errors.Trace(err)
	}
	meta, err := convertCharmMeta(info.Meta)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &CharmInfo{
		Revision:   info.Revision,
		URL:        info.URL,
		Config:     params.FromCharmOptionMap(info.Config),
		Meta:       meta,
		Actions:    convertCharmActions(info.Actions),
		Metrics:    convertCharmMetrics(info.Metrics),
		LXDProfile: convertCharmLXDProfile(info.LXDProfile),
	}
	return result, nil
}

// AddCharm adds the given charm URL (which must include revision) to
// the model, if it does not exist yet. Local charms are not
// supported, only charm store and charm hub URLs. See also AddLocalCharm()
// in the client-side API.
//
// If the AddCharm API call fails because of an authorization error
// when retrieving the charm from the charm store, an error
// satisfying params.IsCodeUnauthorized will be returned.
func (c *Client) AddCharm(curl *charm.URL, origin apicharm.Origin, force bool, series string) (apicharm.Origin, error) {
	args := params.AddCharmWithOrigin{
		URL:    curl.String(),
		Origin: origin.ParamsCharmOrigin(),
		Force:  force,
		Series: series,
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
func (c *Client) AddCharmWithAuthorization(curl *charm.URL, origin apicharm.Origin, csMac *macaroon.Macaroon, force bool, series string) (apicharm.Origin, error) {
	args := params.AddCharmWithAuth{
		URL:                curl.String(),
		Origin:             origin.ParamsCharmOrigin(),
		CharmStoreMacaroon: csMac,
		Force:              force,
		Series:             series,
	}
	var result params.CharmOriginResult
	if err := c.facade.FacadeCall("AddCharmWithAuthorization", args, &result); err != nil {
		return apicharm.Origin{}, errors.Trace(err)
	}
	return apicharm.APICharmOrigin(result.Origin), nil
}
