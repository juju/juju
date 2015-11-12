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

func ParseOffer(p params.CrossModelOffer) (crossmodel.Offer, error) {
	serviceTag, err := names.ParseServiceTag(p.Service)
	if err != nil {
		return crossmodel.Offer{}, errors.Annotatef(err, "cannot parse service tag %q", p.Service)
	}

	users := make([]names.UserTag, len(p.Users))
	for i, user := range p.Users {
		users[i], err = names.ParseUserTag(user)
		if err != nil {
			return crossmodel.Offer{}, errors.Annotatef(err, "cannot parse user tag %q", user)
		}
	}

	return crossmodel.Offer{
		Service:   serviceTag,
		Endpoints: p.Endpoints,
		URL:       p.URL,
		Users:     users,
	}, nil
}
