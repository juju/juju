// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/juju/rpc/params"
)

// filtersToLegacyFilters converts params which use a model qualifier
// to legacy params which use a model owner name.
func filtersToLegacyFilters(in params.OfferFilters) params.OfferFiltersLegacy {
	out := params.OfferFiltersLegacy{
		Filters: make([]params.OfferFilterLegacy, len(in.Filters)),
	}
	for i, f := range in.Filters {
		out.Filters[i] = params.OfferFilterLegacy{
			OwnerName:              f.ModelQualifier,
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
