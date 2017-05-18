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
func (c *Client) Offer(modelUUID, application string, endpoints []string, offerName string, desc string) ([]params.ErrorResult, error) {
	// TODO(wallyworld) - support endpoint aliases
	ep := make(map[string]string)
	for _, name := range endpoints {
		ep[name] = name
	}
	offers := []params.AddApplicationOffer{
		{
			ModelTag:               names.NewModelTag(modelUUID).String(),
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
			ModelName:       f.ModelName,
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
func (c *Client) GrantOffer(user, access string, offerURLs ...string) error {
	return c.modifyOfferUser(params.GrantOfferAccess, user, access, offerURLs)
}

// RevokeOffer revokes a user's access to the specified offers.
func (c *Client) RevokeOffer(user, access string, offerURLs ...string) error {
	return c.modifyOfferUser(params.RevokeOfferAccess, user, access, offerURLs)
}

func (c *Client) modifyOfferUser(action params.OfferAction, user, access string, offerURLs []string) error {
	var args params.ModifyOfferAccessRequest

	if !names.IsValidUser(user) {
		return errors.Errorf("invalid username: %q", user)
	}
	userTag := names.NewUserTag(user)

	offerAccess := permission.Access(access)
	if err := permission.ValidateOfferAccess(offerAccess); err != nil {
		return errors.Trace(err)
	}
	for _, offerURL := range offerURLs {
		args.Changes = append(args.Changes, params.ModifyOfferAccess{
			UserTag:  userTag.String(),
			Action:   action,
			Access:   params.OfferAccessPermission(offerAccess),
			OfferURL: offerURL,
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
			logger.Warningf("offer %q is already shared with %q", offerURLs[i], userTag.Id())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}

// ApplicationOffer returns offered remote application details for a given URL.
func (c *Client) ApplicationOffer(urlStr string) (params.ApplicationOffer, error) {

	url, err := crossmodel.ParseApplicationURL(urlStr)
	if err != nil {
		return params.ApplicationOffer{}, errors.Trace(err)
	}
	if url.Source != "" {
		return params.ApplicationOffer{}, errors.NotSupportedf("query for non-local application offers")
	}

	found := params.ApplicationOffersResults{}

	err = c.facade.FacadeCall("ApplicationOffers", params.ApplicationURLs{[]string{urlStr}}, &found)
	if err != nil {
		return params.ApplicationOffer{}, errors.Trace(err)
	}

	result := found.Results
	if len(result) != 1 {
		return params.ApplicationOffer{}, errors.Errorf("expected to find one result for url %q but found %d", url, len(result))
	}

	theOne := result[0]
	if theOne.Error != nil {
		return params.ApplicationOffer{}, errors.Trace(theOne.Error)
	}
	return theOne.Result, nil
}

// FindApplicationOffers returns all application offers matching the supplied filter.
func (c *Client) FindApplicationOffers(filters ...crossmodel.ApplicationOfferFilter) ([]params.ApplicationOffer, error) {
	// We need at least one filter. The default filter will list all local applications.
	if len(filters) == 0 {
		return nil, errors.New("at least one filter must be specified")
	}
	var paramsFilter params.OfferFilters
	for _, f := range filters {
		filterTerm := params.OfferFilter{
			OfferName: f.OfferName,
			ModelName: f.ModelName,
			OwnerName: f.OwnerName,
		}
		filterTerm.Endpoints = make([]params.EndpointFilterAttributes, len(f.Endpoints))
		for i, ep := range f.Endpoints {
			filterTerm.Endpoints[i].Name = ep.Name
			filterTerm.Endpoints[i].Interface = ep.Interface
			filterTerm.Endpoints[i].Role = ep.Role
		}
		paramsFilter.Filters = append(paramsFilter.Filters, filterTerm)
	}

	out := params.FindApplicationOffersResults{}
	err := c.facade.FacadeCall("FindApplicationOffers", paramsFilter, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results, nil
}

// Consume adds a remote application to the specified target model.
func (c *Client) Consume(targetModelTag names.ModelTag, offer params.ApplicationOffer, alias string) (string, error) {
	var consumeRes params.ConsumeApplicationResults
	args := params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationAlias: alias,
			TargetModelTag:   targetModelTag.String(),
			ApplicationOffer: offer,
		}},
	}
	err := c.facade.FacadeCall("Consume", args, &consumeRes)
	if err != nil {
		return "", errors.Trace(err)
	}
	if resultLen := len(consumeRes.Results); resultLen != 1 {
		return "", errors.Errorf("expected 1 result, got %d", resultLen)
	}
	if err := consumeRes.Results[0].Error; err != nil {
		return "", errors.Trace(err)
	}
	return consumeRes.Results[0].LocalName, nil
}
