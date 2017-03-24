// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodelcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
)

// OffersAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPI struct {
	crossmodelcommon.BaseAPI
}

// createAPI returns a new application offers OffersAPI facade.
func createOffersAPI(
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers,
	backend crossmodelcommon.Backend,
	statePool crossmodelcommon.StatePool,
	authorizer facade.Authorizer,
) (*OffersAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	api := &OffersAPI{
		BaseAPI: crossmodelcommon.BaseAPI{
			Authorizer:           authorizer,
			GetApplicationOffers: getApplicationOffers,
			Backend:              backend,
			StatePool:            statePool,
		}}
	return api, nil
}

// NewOffersAPI returns a new application offers OffersAPI facade.
func NewOffersAPI(ctx facade.Context) (*OffersAPI, error) {
	return createOffersAPI(
		crossmodelcommon.GetApplicationOffers, crossmodelcommon.GetStateAccess(ctx.State()),
		crossmodelcommon.GetStatePool(ctx.StatePool()), ctx.Auth())
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *OffersAPI) Offer(all params.AddApplicationOffers) (params.ErrorResults, error) {
	if err := api.CheckPermission(api.Backend, permission.AdminAccess); err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}

	result := make([]params.ErrorResult, len(all.Offers))

	for i, one := range all.Offers {
		applicationOfferParams, err := api.makeAddOfferArgsFromParams(one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		_, err = api.GetApplicationOffers(api.Backend).AddOffer(applicationOfferParams)
		result[i].Error = common.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}

func (api *OffersAPI) makeAddOfferArgsFromParams(addOfferParams params.AddApplicationOffer) (jujucrossmodel.AddApplicationOfferArgs, error) {
	result := jujucrossmodel.AddApplicationOfferArgs{
		OfferName:              addOfferParams.OfferName,
		ApplicationName:        addOfferParams.ApplicationName,
		ApplicationDescription: addOfferParams.ApplicationDescription,
		Endpoints:              addOfferParams.Endpoints,
	}
	if result.OfferName == "" {
		result.OfferName = result.ApplicationName
	}
	application, err := api.Backend.Application(addOfferParams.ApplicationName)
	if err != nil {
		return result, errors.Annotatef(err, "getting offered application %v", addOfferParams.ApplicationName)
	}

	if result.ApplicationDescription == "" {
		ch, _, err := application.Charm()
		if err != nil {
			return result,
				errors.Annotatef(err, "getting charm for application %v", addOfferParams.ApplicationName)
		}
		result.ApplicationDescription = ch.Meta().Description
	}
	return result, nil
}

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
func (api *OffersAPI) ListApplicationOffers(filters params.OfferFilters) (params.ListApplicationOffersResults, error) {
	var result params.ListApplicationOffersResults
	offers, err := api.GetApplicationOffersDetails(filters, permission.AdminAccess)
	if err != nil {
		return result, err
	}
	result.Results = offers
	return result, nil
}
