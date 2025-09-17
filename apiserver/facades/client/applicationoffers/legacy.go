// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/juju/names/v6"

	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// legacyFiltersToFilters converts params which use a model owner name
// to params which use a model qualifier.
func legacyFiltersToFilters(in params.OfferFiltersLegacy) params.OfferFilters {
	out := params.OfferFilters{
		Filters: make([]params.OfferFilter, len(in.Filters)),
	}
	for i, f := range in.Filters {
		out.Filters[i] = params.OfferFilter{
			ModelName:              f.ModelName,
			OfferName:              f.OfferName,
			ApplicationName:        f.ApplicationName,
			ApplicationDescription: f.ApplicationDescription,
			ApplicationUser:        f.ApplicationUser,
			Endpoints:              f.Endpoints,
			ConnectedUserTags:      f.ConnectedUserTags,
			AllowedConsumerTags:    f.AllowedConsumerTags,
		}
		if f.OwnerName != "" {
			out.Filters[i].ModelQualifier = model.QualifierFromUserTag(names.NewUserTag(f.OwnerName)).String()
		}
	}
	return out
}

// ApplicationOffers gets details about remote applications that match given URLs.
// It converts incoming URLs to adapt model owner name to qualifier.
func (api *OffersAPIv5) ApplicationOffers(ctx context.Context, args params.OfferURLs) (params.ApplicationOffersResults, error) {
	args.OfferURLs = transformOfferURLs(args.OfferURLs)

	return api.OffersAPI.ApplicationOffers(ctx, args)
}

func transformOfferURLs(in []string) []string {
	updatedURLs := make([]string, len(in))

	// Update any model owners values in the offer URLs to be in
	// the model qualifier form used currently. All other concerns of
	// parsing the URL or handling errors will be done by the newer
	// version of ApplicationOffers.
	for i, urlStr := range in {
		url, err := jujucrossmodel.ParseOfferURL(urlStr)
		if err != nil {
			// We know this will fail, however to allow for errors to
			// be properly returned, handle in the newer version of
			// ApplicationOffers.
			updatedURLs[i] = urlStr
			continue
		}
		if url.ModelQualifier == "" {
			updatedURLs[i] = urlStr
			continue
		}
		// Older clients may try to reference an offer with a model owner username.
		// Create a URL ensuring a valid model qualifier is always used.
		url.ModelQualifier = model.QualifierFromUserTag(names.NewUserTag(url.ModelQualifier)).String()
		updatedURLs[i] = url.String()
	}
	return updatedURLs
}

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
// It converts incoming filters which contain a model owner to use a model qualifier.
func (api *OffersAPIv5) ListApplicationOffers(ctx context.Context, legacyFilters params.OfferFiltersLegacy) (params.QueryApplicationOffersResultsV5, error) {
	filters := legacyFiltersToFilters(legacyFilters)
	return api.OffersAPI.ListApplicationOffers(ctx, filters)
}

// FindApplicationOffers gets details about remote applications that match given filter.
// It converts incoming filters which contain a model owner to use a model qualifier.
func (api *OffersAPIv5) FindApplicationOffers(ctx context.Context, legacyFilters params.OfferFiltersLegacy) (params.QueryApplicationOffersResultsV5, error) {
	filters := legacyFiltersToFilters(legacyFilters)
	return api.OffersAPI.FindApplicationOffers(ctx, filters)
}
