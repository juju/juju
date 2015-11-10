// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/crossmodel"
)

// serviceDirectory allows access to a locally hosted service directory.
type serviceDirectoryAPI struct {
	base.ClientFacade
	facade base.FacadeCaller
}

var _ jujucrossmodel.ServiceDirectoryProvider = (*serviceDirectoryAPI)(nil)

// NewServiceDirectory creates a new client for accessing a controller service directory API.
func NewServiceDirectory(st base.APICallCloser) *jujucrossmodel.ServiceDirectory {
	frontend, backend := base.NewClientFacade(st, "ServiceDirectory")
	return &jujucrossmodel.ServiceDirectory{
		&serviceDirectoryAPI{ClientFacade: frontend, facade: backend}}
}

// List offers returns the offers satisfying the specified filter.
func (s *serviceDirectoryAPI) ListOffers(filter ...params.OfferFilter) ([]params.ServiceOffer, error) {
	filters := params.OfferFilters{filter}
	results := new(params.ServiceOfferResults)
	if err := s.facade.FacadeCall("ListOffers", filters, results); err != nil {
		return nil, errors.Trace(err)
	}
	if results.Error != nil {
		return nil, results.Error
	}
	return results.Offers, nil
}

// AddOffer adds a new service offering to the directory, able to be consumed by
// the specified users.
func (s *serviceDirectoryAPI) AddOffer(url string, offerDetails params.ServiceOfferDetails, users []names.UserTag) error {
	offers := []params.AddServiceOffer{{url, offerDetails, users}}
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
