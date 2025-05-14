// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/juju/juju/rpc/params"
)

// OffersAPIv5 implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPIv5 struct {
	*OffersAPI
}

func legacyFiltersToFilters(in params.OfferFiltersLegacy) params.OfferFilters {
	out := params.OfferFilters{
		Filters: make([]params.OfferFilter, len(in.Filters)),
	}
	for i, f := range in.Filters {
		out.Filters[i] = params.OfferFilter{
			ModelNamespace:         f.OwnerName,
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

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
func (api *OffersAPIv5) ListApplicationOffers(ctx context.Context, legacyFilters params.OfferFiltersLegacy) (params.QueryApplicationOffersResultsV5, error) {
	filters := legacyFiltersToFilters(legacyFilters)
	return api.OffersAPI.ListApplicationOffers(ctx, filters)
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *OffersAPIv5) FindApplicationOffers(ctx context.Context, legacyFilters params.OfferFiltersLegacy) (params.QueryApplicationOffersResultsV5, error) {
	filters := legacyFiltersToFilters(legacyFilters)
	return api.OffersAPI.FindApplicationOffers(ctx, filters)
}
