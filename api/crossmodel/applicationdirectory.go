// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
)

// applicationOffersAPI allows access to a locally hosted application directory.
type applicationOffersAPI struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// ApplicationOffersAPI instances provides access to application offerings from external environments.
type ApplicationOffersAPI interface {

	// AddOffer adds a new application offering to a application directory.
	// The offer's application URL scheme determines the directory to
	// which the offer is added.
	AddOffer(offer crossmodel.ApplicationOffer, users []string) error

	// List offers returns the offers contained in the specified directory,
	// satisfying the specified filter. The directory is the same as the service
	// URL scheme and is used to determine which backend to query.
	ListOffers(directory string, filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error)
}

// NewApplicationOffers creates a new client for accessing a controller application directory API.
func NewApplicationOffers(st base.APICallCloser) ApplicationOffersAPI {
	frontend, backend := base.NewClientFacade(st, "ApplicationOffers")
	return &applicationOffersAPI{ClientFacade: frontend, facade: backend}
}

// TODO(wallyworld) - add Remove() and Update()

// AddOffer adds a new application offering to the directory, able to be consumed by
// the specified users.
func (s *applicationOffersAPI) AddOffer(offer crossmodel.ApplicationOffer, users []string) error {
	addOffer := params.AddApplicationOffer{
		ApplicationOffer: MakeParamsFromOffer(offer),
	}
	addOffer.UserTags = make([]string, len(users))
	for i, user := range users {
		if !names.IsValidUserName(user) {
			return errors.NotValidf("user name %q", user)
		}
		addOffer.UserTags[i] = names.NewUserTag(user).String()
	}
	offers := []params.AddApplicationOffer{addOffer}

	results := new(params.ErrorResults)
	if err := s.facade.FacadeCall("AddOffers", params.AddApplicationOffers{Offers: offers}, results); err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		err := errors.Errorf("expected 1 result, got %d", len(results.Results))
		return errors.Trace(err)
	}
	if results.Results[0].Error == nil {
		return nil
	}
	return results.Results[0].Error
}

// MakeParamsFromOffer creates api parameters from a ApplicationOffer.
func MakeParamsFromOffer(offer crossmodel.ApplicationOffer) params.ApplicationOffer {
	eps := make([]params.RemoteEndpoint, len(offer.Endpoints))
	for i, ep := range offer.Endpoints {
		eps[i] = params.RemoteEndpoint{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	return params.ApplicationOffer{
		ApplicationURL:         offer.ApplicationURL,
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.ApplicationDescription,
		SourceModelTag:         names.NewModelTag(offer.SourceModelUUID).String(),
		SourceLabel:            offer.SourceLabel,
		Endpoints:              eps,
	}
}

// List offers returns the offers satisfying the specified filter for the specified directory.
func (s *applicationOffersAPI) ListOffers(directory string, filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	if directory == "" {
		return nil, errors.New("application directory must be specified")
	}
	var filterParams params.OfferFilters
	filterParams.Directory = directory
	filterParams.Filters = make([]params.OfferFilter, len(filters))
	// TODO(wallyworld) - support or remove params.ApplicationOfferFilter.ServiceUser
	for i, filter := range filters {
		eps := make([]params.EndpointFilterAttributes, len(filter.Endpoints))
		for j, ep := range filter.Endpoints {
			eps[j] = params.EndpointFilterAttributes{
				Name:      ep.Name,
				Interface: ep.Interface,
				Role:      ep.Role,
			}
		}
		users := make([]string, len(filter.AllowedUsers))
		for j, user := range filter.AllowedUsers {
			users[j] = names.NewUserTag(user).String()
		}
		filterParams.Filters[i] = params.OfferFilter{
			ApplicationURL:         filter.ApplicationURL,
			ApplicationName:        filter.ApplicationName,
			ApplicationDescription: filter.ApplicationDescription,
			SourceLabel:            filter.SourceLabel,
			SourceModelUUIDTag:     names.NewModelTag(filter.SourceModelUUID).String(),
			Endpoints:              eps,
			AllowedUserTags:        users,
		}
	}
	results := new(params.ApplicationOfferResults)
	if err := s.facade.FacadeCall("ListOffers", filterParams, results); err != nil {
		return nil, errors.Trace(err)
	}
	if results.Error != nil {
		return nil, results.Error
	}
	offers := make([]crossmodel.ApplicationOffer, len(results.Offers))
	for i, offer := range results.Offers {
		var err error
		if offers[i], err = MakeOfferFromParams(offer); err != nil {
			return nil, errors.Annotatef(err, "invalid environment UUID for offer %v", offer)
		}
	}
	return offers, nil
}

// MakeOfferFromParams creates a ApplicationOffer from api parameters.
func MakeOfferFromParams(offer params.ApplicationOffer) (crossmodel.ApplicationOffer, error) {
	eps := make([]charm.Relation, len(offer.Endpoints))
	for i, ep := range offer.Endpoints {
		eps[i] = charm.Relation{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	envTag, err := names.ParseModelTag(offer.SourceModelTag)
	if err != nil {
		return crossmodel.ApplicationOffer{}, errors.Trace(err)
	}
	return crossmodel.ApplicationOffer{
		ApplicationURL:         offer.ApplicationURL,
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.ApplicationDescription,
		SourceModelUUID:        envTag.Id(),
		SourceLabel:            offer.SourceLabel,
		Endpoints:              eps,
	}, nil
}
