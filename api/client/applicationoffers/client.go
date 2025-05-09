// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

var logger = internallogger.GetLogger("juju.api.applicationoffers")

// Client allows access to the cross model management API end points.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the application offers API.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "ApplicationOffers", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// Offer prepares application's endpoints for consumption.
func (c *Client) Offer(ctx context.Context, modelUUID, application string, endpoints []string, owner, offerName, desc string) ([]params.ErrorResult, error) {
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
			OwnerTag:               names.NewUserTag(owner).String(),
		},
	}
	out := params.ErrorResults{}
	if err := c.facade.FacadeCall(ctx, "Offer", params.AddApplicationOffers{Offers: offers}, &out); err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results, nil
}

func filtersToLegacyFilters(in params.OfferFilters) params.OfferFiltersLegacy {
	out := params.OfferFiltersLegacy{
		Filters: make([]params.OfferFilterLegacy, len(in.Filters)),
	}
	for i, f := range in.Filters {
		out.Filters[i] = params.OfferFilterLegacy{
			OwnerName:              f.OfferName,
			ModelName:              f.ModelName,
			OfferName:              f.OfferName,
			ApplicationName:        f.ApplicationName,
			ApplicationDescription: f.ApplicationDescription,
			ApplicationUser:        f.ApplicationUser,
			Endpoints:              f.Endpoints,
			ConnectedUserTags:      f.ConnectedUserTags,
			AllowedConsumerTags:    f.AllowedConsumerTags,
		}
	}
	return out
}

// ListOffers gets all remote applications that have been offered from this Juju model.
// Each returned application satisfies at least one of the the specified filters.
func (c *Client) ListOffers(ctx context.Context, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
	var (
		filtersArg   any
		paramsFilter params.OfferFilters
	)
	for _, f := range filters {
		filterTerm := params.OfferFilter{
			Namespace:           f.Namespace,
			ModelName:           f.ModelName,
			OfferName:           f.OfferName,
			ApplicationName:     f.ApplicationName,
			Endpoints:           make([]params.EndpointFilterAttributes, len(f.Endpoints)),
			AllowedConsumerTags: make([]string, len(f.AllowedConsumers)),
			ConnectedUserTags:   make([]string, len(f.ConnectedUsers)),
		}
		for i, ep := range f.Endpoints {
			filterTerm.Endpoints[i].Name = ep.Name
			filterTerm.Endpoints[i].Interface = ep.Interface
			filterTerm.Endpoints[i].Role = ep.Role
		}
		for i, u := range f.AllowedConsumers {
			filterTerm.AllowedConsumerTags[i] = names.NewUserTag(u).String()
		}
		for i, u := range f.ConnectedUsers {
			filterTerm.ConnectedUserTags[i] = names.NewUserTag(u).String()
		}
		paramsFilter.Filters = append(paramsFilter.Filters, filterTerm)
	}
	filtersArg = paramsFilter
	if c.BestAPIVersion() < 6 {
		filtersArg = filtersToLegacyFilters(paramsFilter)
	}

	offers := params.QueryApplicationOffersResultsV5{}
	err := c.facade.FacadeCall(ctx, "ListApplicationOffers", filtersArg, &offers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return convertOffersResultsToModel(offers.Results)
}

func convertOffersResultsToModel(items []params.ApplicationOfferAdminDetailsV5) ([]*crossmodel.ApplicationOfferDetails, error) {
	result := make([]*crossmodel.ApplicationOfferDetails, len(items))
	var err error
	for i, one := range items {
		if result[i], err = offerParamsToDetails(one); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}

func offerParamsToDetails(offer params.ApplicationOfferAdminDetailsV5) (*crossmodel.ApplicationOfferDetails, error) {
	eps := make([]charm.Relation, len(offer.Endpoints))
	for i, ep := range offer.Endpoints {
		eps[i] = charm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
		}
	}
	result := &crossmodel.ApplicationOfferDetails{
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.ApplicationDescription,
		OfferName:              offer.OfferName,
		CharmURL:               offer.CharmURL,
		OfferURL:               offer.OfferURL,
		Endpoints:              eps,
	}
	for _, oc := range offer.Connections {
		modelTag, err := names.ParseModelTag(oc.SourceModelTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Connections = append(result.Connections, crossmodel.OfferConnection{
			SourceModelUUID: modelTag.Id(),
			Username:        oc.Username,
			Endpoint:        oc.Endpoint,
			RelationId:      oc.RelationId,
			Status:          relation.Status(oc.Status.Status),
			Message:         oc.Status.Info,
			Since:           oc.Status.Since,
			IngressSubnets:  oc.IngressSubnets,
		})
	}
	for _, u := range offer.Users {
		result.Users = append(result.Users, crossmodel.OfferUserDetails{
			UserName:    u.UserName,
			DisplayName: u.DisplayName,
			Access:      permission.Access(u.Access),
		})
	}
	return result, nil
}

// GrantOffer grants a user access to the specified offers.
func (c *Client) GrantOffer(ctx context.Context, user, access string, offerURLs ...string) error {
	return c.modifyOfferUser(ctx, params.GrantOfferAccess, user, access, offerURLs)
}

// RevokeOffer revokes a user's access to the specified offers.
func (c *Client) RevokeOffer(ctx context.Context, user, access string, offerURLs ...string) error {
	return c.modifyOfferUser(ctx, params.RevokeOfferAccess, user, access, offerURLs)
}

func (c *Client) modifyOfferUser(ctx context.Context, action params.OfferAction, user, access string, offerURLs []string) error {
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
	err := c.facade.FacadeCall(ctx, "ModifyOfferAccess", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Changes) {
		return errors.Errorf("expected %d results, got %d", len(args.Changes), len(result.Results))
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeAlreadyExists {
			logger.Warningf(context.TODO(), "offer %q is already shared with %q", offerURLs[i], userTag.Id())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}

// ApplicationOffer returns offered remote application details for a given URL.
func (c *Client) ApplicationOffer(ctx context.Context, urlStr string) (*crossmodel.ApplicationOfferDetails, error) {

	url, err := crossmodel.ParseOfferURL(urlStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if url.Source != "" {
		return nil, errors.NotSupportedf("query for non-local application offers")
	}

	found := params.ApplicationOffersResults{}

	err = c.facade.FacadeCall(ctx, "ApplicationOffers", params.OfferURLs{[]string{urlStr}, bakery.LatestVersion}, &found)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := found.Results
	if len(result) != 1 {
		return nil, errors.Errorf("expected to find one result for url %q but found %d", url, len(result))
	}

	theOne := result[0]
	if theOne.Error != nil {
		return nil, errors.Trace(theOne.Error)
	}
	return offerParamsToDetails(*theOne.Result)
}

// FindApplicationOffers returns all application offers matching the supplied filter.
func (c *Client) FindApplicationOffers(ctx context.Context, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error) {
	// We need at least one filter. The default filter will list all local applications.
	if len(filters) == 0 {
		return nil, errors.New("at least one filter must be specified")
	}

	var (
		filtersArg   any
		paramsFilter params.OfferFilters
	)
	for _, f := range filters {
		filterTerm := params.OfferFilter{
			OfferName: f.OfferName,
			ModelName: f.ModelName,
			Namespace: f.Namespace,
		}
		filterTerm.Endpoints = make([]params.EndpointFilterAttributes, len(f.Endpoints))
		for i, ep := range f.Endpoints {
			filterTerm.Endpoints[i].Name = ep.Name
			filterTerm.Endpoints[i].Interface = ep.Interface
			filterTerm.Endpoints[i].Role = ep.Role
		}
		paramsFilter.Filters = append(paramsFilter.Filters, filterTerm)
	}
	filtersArg = paramsFilter
	if c.BestAPIVersion() < 6 {
		filtersArg = filtersToLegacyFilters(paramsFilter)
	}

	offers := params.QueryApplicationOffersResultsV5{}
	err := c.facade.FacadeCall(ctx, "FindApplicationOffers", filtersArg, &offers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return convertOffersResultsToModel(offers.Results)
}

// GetConsumeDetails returns details necessary to consume an offer at a given URL.
func (c *Client) GetConsumeDetails(ctx context.Context, urlStr string) (params.ConsumeOfferDetails, error) {

	url, err := crossmodel.ParseOfferURL(urlStr)
	if err != nil {
		return params.ConsumeOfferDetails{}, errors.Trace(err)
	}
	if url.Source != "" {
		return params.ConsumeOfferDetails{}, errors.NotSupportedf("query for application offers on another controller")
	}

	found := params.ConsumeOfferDetailsResults{}

	offerURLs := params.OfferURLs{[]string{urlStr}, bakery.LatestVersion}
	args := params.ConsumeOfferDetailsArg{
		OfferURLs: offerURLs,
	}

	err = c.facade.FacadeCall(ctx, "GetConsumeDetails", args, &found)
	if err != nil {
		return params.ConsumeOfferDetails{}, errors.Trace(err)
	}

	result := found.Results
	if len(result) != 1 {
		return params.ConsumeOfferDetails{}, errors.Errorf("expected to find one result for url %q but found %d", url, len(result))
	}

	theOne := result[0]
	if theOne.Error != nil {
		return params.ConsumeOfferDetails{}, errors.Trace(theOne.Error)
	}
	return params.ConsumeOfferDetails{
		Offer:          theOne.Offer,
		Macaroon:       theOne.Macaroon,
		ControllerInfo: theOne.ControllerInfo,
	}, nil
}

// DestroyOffers removes the specified application offers.
func (c *Client) DestroyOffers(ctx context.Context, force bool, offerURLs ...string) error {
	if len(offerURLs) == 0 {
		return nil
	}
	args := params.DestroyApplicationOffers{
		Force:     force,
		OfferURLs: make([]string, len(offerURLs)),
	}
	for i, url := range offerURLs {
		if _, err := crossmodel.ParseOfferURL(url); err != nil {
			return errors.Trace(err)
		}
		args.OfferURLs[i] = url
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall(ctx, "DestroyOffers", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.OfferURLs) {
		return errors.Errorf("expected %d results, got %d", len(args.OfferURLs), len(result.Results))
	}
	return result.Combine()
}
