// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	crossmodelapi "github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

func init() {
	common.RegisterStandardFacade("ServiceDirectory", 1, newServiceDirectoryAPI)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type ServiceDirectoryAPI struct {
	authorizer       common.Authorizer
	serviceDirectory crossmodel.ServiceDirectory
}

// createServiceDirectoryAPI returns a new cross model API facade.
func createServiceDirectoryAPI(
	serviceDirectory crossmodel.ServiceDirectory,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ServiceDirectoryAPI, error) {
	if !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}

	return &ServiceDirectoryAPI{
		authorizer:       authorizer,
		serviceDirectory: serviceDirectory,
	}, nil
}

func newServiceDirectoryAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ServiceDirectoryAPI, error) {
	return createServiceDirectoryAPI(NewEmbeddedServiceDirectory(st), resources, authorizer)
}

// ListOffers returns offers matching the filter from a service directory.
func (api *ServiceDirectoryAPI) ListOffers(filters params.OfferFilters) ([]params.ServiceOffer, error) {
	offerFilters, err := makeOfferFilterFromParams(filters.Filters)
	if err != nil {
		return nil, err
	}
	offers, err := api.serviceDirectory.ListOffers(offerFilters...)
	if err != nil {
		return nil, err
	}
	result := make([]params.ServiceOffer, len(offers))
	for i, offer := range offers {
		result[i] = crossmodelapi.MakeParamsFromOffer(offer)
	}
	return result, nil
}

func makeOfferFilterFromParams(filters []params.OfferFilter) ([]crossmodel.ServiceOfferFilter, error) {
	offerFilters := make([]crossmodel.ServiceOfferFilter, len(filters))
	for i, filter := range filters {
		envTag, err := names.ParseEnvironTag(filter.SourceEnvUUIDTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		offerFilters[i] = crossmodel.ServiceOfferFilter{
			ServiceOffer: crossmodel.ServiceOffer{
				ServiceURL:         filter.ServiceURL,
				ServiceName:        filter.ServiceName,
				ServiceDescription: filter.ServiceDescription,
				SourceLabel:        filter.SourceLabel,
				SourceEnvUUID:      envTag.Id(),
			},
		}
	}
	return offerFilters, nil
}

// AddOffers adds new service offerings to a directory, able to be consumed by
// the specified users.
func (api *ServiceDirectoryAPI) AddOffers(offers params.AddServiceOffers) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(offers.Offers)),
	}
	if len(offers.Offers) == 0 {
		return result, nil
	}

	for i, offerParams := range offers.Offers {
		offer, err := crossmodelapi.MakeOfferFromParams(offerParams.ServiceOffer)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := api.serviceDirectory.AddOffer(offer); err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	// TODO(wallyworld) - write ACLs with supplied users once we support that
	return result, nil
}

// NewEmbeddedServiceDirectory creates a service directory used by a Juju controller.
func NewEmbeddedServiceDirectory(st *state.State) crossmodel.ServiceDirectory {
	return &controllerServiceDirectory{st}
}

type controllerServiceDirectory struct {
	st *state.State
}

func (s *controllerServiceDirectory) AddOffer(offer crossmodel.ServiceOffer) error {
	// TODO(wallyworld) - implement
	return errors.NewNotImplemented(nil, "add offer")
}

func (s *controllerServiceDirectory) UpdateOffer(offer crossmodel.ServiceOffer) error {
	// TODO(wallyworld) - implement
	return errors.NewNotImplemented(nil, "update offer")
}

func (s *controllerServiceDirectory) ListOffers(filters ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
	// TODO(wallyworld) - implement
	return nil, errors.NewNotImplemented(nil, "list offers")
}

func (s *controllerServiceDirectory) Remove(url string) error {
	// TODO(wallyworld) - implement
	return errors.NewNotImplemented(nil, "delete offer")
}
