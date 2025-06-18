// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/juju/juju/rpc/params"
)

// OffersAPIv5 implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPIv5 struct{}

// createAPI returns a new application offers OffersAPI facade.
func createOffersAPI() (*OffersAPIv5, error) {
	api := &OffersAPIv5{}
	return api, nil
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *OffersAPIv5) Offer(ctx context.Context, all params.AddApplicationOffers) (params.ErrorResults, error) {
	// Although this API is offering adding offers in bulk, we only want to
	// support adding one offer at a time. This is because we're jumping into
	// other models using the state pool, in the context of a model facade.
	// There is no limit, nor pagination, on the number of offers that can be
	// added in one call, so any nefarious user could add a large number of
	// offers in one call, and potentially exhaust the state pool. This becomes
	// more of a problem when we move to dqlite (4.0 and beyond), as each
	// model is within a different database. By limiting the number of offers
	// we force the clients to make multiple calls and if required we can
	// enforce rate limiting.
	// This API will be deprecated in the future and replaced once we refactor
	// the API (5.0 and beyond).
	return params.ErrorResults{}, nil
}

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
func (api *OffersAPIv5) ListApplicationOffers(ctx context.Context, filters params.OfferFilters) (params.QueryApplicationOffersResultsV5, error) {
	return params.QueryApplicationOffersResultsV5{}, nil
}

// ModifyOfferAccess changes the application offer access granted to users.
func (api *OffersAPIv5) ModifyOfferAccess(ctx context.Context, args params.ModifyOfferAccessRequest) (result params.ErrorResults, _ error) {
	return params.ErrorResults{}, nil
}

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *OffersAPIv5) ApplicationOffers(ctx context.Context, urls params.OfferURLs) (params.ApplicationOffersResults, error) {
	return params.ApplicationOffersResults{}, nil
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *OffersAPIv5) FindApplicationOffers(ctx context.Context, filters params.OfferFilters) (params.QueryApplicationOffersResultsV5, error) {
	return params.QueryApplicationOffersResultsV5{}, nil
}

// GetConsumeDetails returns the details necessary to pass to another model
// to allow the specified args user to consume the offers represented by the args URLs.
func (api *OffersAPIv5) GetConsumeDetails(ctx context.Context, args params.ConsumeOfferDetailsArg) (params.ConsumeOfferDetailsResults, error) {
	return params.ConsumeOfferDetailsResults{}, nil
}

// RemoteApplicationInfo returns information about the requested remote application.
// This call currently has no client side API, only there for the Dashboard at this stage.
func (api *OffersAPIv5) RemoteApplicationInfo(ctx context.Context, args params.OfferURLs) (params.RemoteApplicationInfoResults, error) {
	return params.RemoteApplicationInfoResults{}, nil
}

// DestroyOffers removes the offers specified by the given URLs, forcing if necessary.
func (api *OffersAPIv5) DestroyOffers(ctx context.Context, args params.DestroyApplicationOffers) (params.ErrorResults, error) {
	return params.ErrorResults{}, nil
}
