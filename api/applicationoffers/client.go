// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
)

var logger = loggo.GetLogger("juju.api.applicationoffers")

// Client allows access to the cross model management API end points.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the application offers API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ApplicationOffers")
	return &Client{ClientFacade: frontend, facade: backend}
}

// Offer prepares application's endpoints for consumption.
func (c *Client) Offer(application string, endpoints []string, offerName string, desc string) ([]params.ErrorResult, error) {
	// TODO(wallyworld) - support endpoint aliases
	ep := make(map[string]string)
	for _, name := range endpoints {
		ep[name] = name
	}
	offers := []params.AddApplicationOffer{
		{
			ApplicationName:        application,
			ApplicationDescription: desc,
			Endpoints:              ep,
			OfferName:              offerName,
		},
	}
	out := params.ErrorResults{}
	if err := c.facade.FacadeCall("Offer", params.AddApplicationOffers{Offers: offers}, &out); err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results, nil
}

// ListOffers gets all remote applications that have been offered from this Juju model.
// Each returned application satisfies at least one of the the specified filters.
func (c *Client) ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOfferDetailsResult, error) {
	var paramsFilter params.OfferFilters
	for _, f := range filters {
		// TODO(wallyworld) - include allowed users
		filterTerm := params.OfferFilter{
			OfferName:       f.OfferName,
			ApplicationName: f.ApplicationName,
		}
		filterTerm.Endpoints = make([]params.EndpointFilterAttributes, len(f.Endpoints))
		for i, ep := range f.Endpoints {
			filterTerm.Endpoints[i].Name = ep.Name
			filterTerm.Endpoints[i].Interface = ep.Interface
			filterTerm.Endpoints[i].Role = ep.Role
		}
		paramsFilter.Filters = append(paramsFilter.Filters, filterTerm)
	}

	applicationOffers := params.ListApplicationOffersResults{}
	err := c.facade.FacadeCall("ListApplicationOffers", paramsFilter, &applicationOffers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return convertListResultsToModel(applicationOffers.Results), nil
}

func convertListResultsToModel(items []params.ApplicationOfferDetails) []crossmodel.ApplicationOfferDetailsResult {
	result := make([]crossmodel.ApplicationOfferDetailsResult, len(items))
	for i, one := range items {
		eps := make([]charm.Relation, len(one.Endpoints))
		for i, ep := range one.Endpoints {
			eps[i] = charm.Relation{
				Name:      ep.Name,
				Role:      ep.Role,
				Interface: ep.Interface,
				Scope:     ep.Scope,
				Limit:     ep.Limit,
			}
		}
		result[i].Result = &crossmodel.ApplicationOfferDetails{
			ApplicationName: one.ApplicationName,
			OfferName:       one.OfferName,
			CharmName:       one.CharmName,
			ConnectedCount:  one.ConnectedCount,
			OfferURL:        one.OfferURL,
			Endpoints:       eps,
		}
	}
	return result
}

// GrantOffer grants a user access to the specified offers.
func (c *Client) GrantOffer(user, access string, offers ...string) error {
	return c.modifyOfferUser(params.GrantOfferAccess, user, access, offers)
}

// RevokeOffer revokes a user's access to the specified offers.
func (c *Client) RevokeOffer(user, access string, offers ...string) error {
	return c.modifyOfferUser(params.RevokeOfferAccess, user, access, offers)
}

func (c *Client) modifyOfferUser(action params.OfferAction, user, access string, offers []string) error {
	var args params.ModifyOfferAccessRequest

	if !names.IsValidUser(user) {
		return errors.Errorf("invalid username: %q", user)
	}
	userTag := names.NewUserTag(user)

	offerAccess := permission.Access(access)
	if err := permission.ValidateOfferAccess(offerAccess); err != nil {
		return errors.Trace(err)
	}
	for _, offer := range offers {
		args.Changes = append(args.Changes, params.ModifyOfferAccess{
			UserTag:  userTag.String(),
			Action:   action,
			Access:   params.OfferAccessPermission(offerAccess),
			OfferTag: names.NewApplicationOfferTag(offer).String(),
		})
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall("ModifyOfferAccess", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Changes) {
		return errors.Errorf("expected %d results, got %d", len(args.Changes), len(result.Results))
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeAlreadyExists {
			logger.Warningf("offer %q is already shared with %q", offers[i], userTag.Id())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}
