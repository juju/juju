// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("CrossModelRelations", 1, NewAPI)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type API struct {
	authorizer common.Authorizer
}

// createAPI returns a new cross model API facade.
func createAPI(
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		authorizer: authorizer,
	}, nil
}

// NewAPI returns a new cross model API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	return createAPI(resources, authorizer)
}

// Offer makes service endpoints available for consumption.
func (api *API) Offer(all params.CrossModelOffers) (params.ErrorResults, error) {
	// export service offers
	offers := make([]params.ErrorResult, len(all.Offers))
	for i, one := range all.Offers {
		offer, err := ParseOffer(one)
		if err != nil {
			offers[i].Error = common.ServerError(err)
			continue
		}

		if err := crossmodel.ExportOffer(offer); err != nil {
			offers[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: offers}, nil
}

// Show gets SAAS endpoints details matching to provided filter.
func (api *API) Show(filter params.SAASSearchFilter) (params.SAASDetailsResult, error) {
	noResult := params.SAASDetailsResult{}

	found, err := crossmodel.Search(filter)
	if err != nil {
		return noResult, errors.Trace(err)
	}
	if len(found) == 0 {
		return noResult, errors.NotFoundf("endpoints with url %q", filter.URL)
	}
	if len(found) > 1 {
		return noResult, errors.Errorf("expected to find one result for url %q but found %d", filter.URL, len(found))
	}

	return ConvertSAASDetails(found[0]), nil
}

// ParseOffer is a helper function that translates from params
// structure into internal service layer one.
func ParseOffer(p params.CrossModelOffer) (crossmodel.Offer, error) {
	serviceTag, err := names.ParseServiceTag(p.Service)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot parse service tag %q", p.Service)
	}

	users := make([]names.UserTag, len(p.Users))
	for i, user := range p.Users {
		users[i], err = names.ParseUserTag(user)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot parse user tag %q", user)
		}
	}

	return crossmodel.NewOffer(serviceTag, p.Endpoints, p.URL, users), nil
}

// ParseOffer is a helper function that translates from params
// structure into internal service layer one.
func ConvertSAASDetails(c crossmodel.ServiceDetails) params.SAASDetailsResult {
	return params.SAASDetailsResult{
		Service:     c.Service().String(),
		Endpoints:   c.Endpoints(),
		Description: c.Description(),
	}
}
