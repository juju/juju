// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import "github.com/juju/juju/apiserver/params"

var (
	CreateAPI                   = createAPI
	CreateApplicationOffersAPI  = createApplicationOffersAPI
	CreateOfferedApplicationAPI = createOfferedApplicationAPI
	NewServiceAPIFactory        = newServiceAPIFactory
	GetStateAccess              = getStateAccess
)

func MakeOfferedApplicationParams(api *API, p params.ApplicationOfferParams) (params.ApplicationOffer, error) {
	return api.makeOfferedApplicationParams(p)
}
