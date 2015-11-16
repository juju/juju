// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
)

// serviceDirectory allows access to a locally hosted service directory.
type serviceOffersAPI struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// A ServiceDirectoryAPI provides access to service offerings from external environments.
type ServiceOffersAPI interface {

	// AddOffer adds a new service offering to a service directory.
	// The offer's service URL scheme determines the directory to
	// which the offer is added.
	AddOffer(offer crossmodel.ServiceOffer, users []string) error

	// List offers returns the offers contained in the specified directory,
	// satisfying the specified filter. The directory is the same as the service
	// URL scheme and is used to determine which backend to query.
	ListOffers(directory string, filters ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error)
}

// NewServiceOffers creates a new client for accessing a controller service directory API.
func NewServiceOffers(st base.APICallCloser) ServiceOffersAPI {
	frontend, backend := base.NewClientFacade(st, "ServiceDirectory")
	return &serviceOffersAPI{ClientFacade: frontend, facade: backend}
}

// TODO(wallyworld) - add Remove() and Update()

// AddOffer adds a new service offering to the directory, able to be consumed by
// the specified users.
func (s *serviceOffersAPI) AddOffer(offer crossmodel.ServiceOffer, users []string) error {
	addOffer := params.AddServiceOffer{
		ServiceOffer: MakeParamsFromOffer(offer),
	}
	addOffer.UserTags = make([]string, len(users))
	for i, user := range users {
		if !names.IsValidUserName(user) {
			return errors.NotValidf("user name %q", user)
		}
		addOffer.UserTags[i] = names.NewUserTag(user).String()
	}
	offers := []params.AddServiceOffer{addOffer}

	results := new(params.ErrorResults)
	if err := s.facade.FacadeCall("AddOffers", params.AddServiceOffers{Offers: offers}, results); err != nil {
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

// MakeParamsFromOffer creates api parameters from a ServiceOffer.
func MakeParamsFromOffer(offer crossmodel.ServiceOffer) params.ServiceOffer {
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
	return params.ServiceOffer{
		ServiceURL:         offer.ServiceURL,
		ServiceName:        offer.ServiceName,
		ServiceDescription: offer.ServiceDescription,
		SourceEnvironTag:   names.NewEnvironTag(offer.SourceEnvUUID).String(),
		SourceLabel:        offer.SourceLabel,
		Endpoints:          eps,
	}
}

// List offers returns the offers satisfying the specified filter for the specified directory.
func (s *serviceOffersAPI) ListOffers(directory string, filters ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
	if directory == "" {
		return nil, errors.New("service directory must be specified")
	}
	var filterParams params.OfferFilters
	filterParams.Directory = directory
	filterParams.Filters = make([]params.OfferFilter, len(filters))
	// TODO(wallyworld) - support or remove params.ServiceOfferFilter.ServiceUser
	for i, filter := range filters {
		eps := make([]params.EndpointFilterAttributes, len(filter.Endpoints))
		for j, ep := range filter.Endpoints {
			eps[j] = params.EndpointFilterAttributes{
				Interface: ep.Interface,
				Role:      ep.Role,
			}
		}
		users := make([]string, len(filter.AllowedUsers))
		for j, user := range filter.AllowedUsers {
			users[j] = names.NewUserTag(user).String()
		}
		filterParams.Filters[i] = params.OfferFilter{
			ServiceURL:         filter.ServiceURL,
			ServiceName:        filter.ServiceName,
			ServiceDescription: filter.ServiceDescription,
			SourceLabel:        filter.SourceLabel,
			SourceEnvUUIDTag:   names.NewEnvironTag(filter.SourceEnvUUID).String(),
			Endpoints:          eps,
			AllowedUserTags:    users,
		}
	}
	results := new(params.ServiceOfferResults)
	if err := s.facade.FacadeCall("ListOffers", filterParams, results); err != nil {
		return nil, errors.Trace(err)
	}
	if results.Error != nil {
		return nil, results.Error
	}
	offers := make([]crossmodel.ServiceOffer, len(results.Offers))
	for i, offer := range results.Offers {
		var err error
		if offers[i], err = MakeOfferFromParams(offer); err != nil {
			return nil, errors.Annotatef(err, "invalid environment UUID for offer %v", offer)
		}
	}
	return offers, nil
}

// MakeOfferFromParams creates a ServiceOffer from api parameters.
func MakeOfferFromParams(offer params.ServiceOffer) (crossmodel.ServiceOffer, error) {
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
	envTag, err := names.ParseEnvironTag(offer.SourceEnvironTag)
	if err != nil {
		return crossmodel.ServiceOffer{}, errors.Trace(err)
	}
	return crossmodel.ServiceOffer{
		ServiceURL:         offer.ServiceURL,
		ServiceName:        offer.ServiceName,
		ServiceDescription: offer.ServiceDescription,
		SourceEnvUUID:      envTag.Id(),
		SourceLabel:        offer.SourceLabel,
		Endpoints:          eps,
	}, nil
}
