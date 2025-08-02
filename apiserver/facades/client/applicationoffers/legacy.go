// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"

	"github.com/juju/names/v6"

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
func (api *OffersAPIv5) ApplicationOffers(ctx context.Context, urls params.OfferURLs) (params.ApplicationOffersResults, error) {
	return params.ApplicationOffersResults{}, nil

	// TODO - this would be the compatibility code if the functionality weren't temporarily removed
	/*
		user := api.Authorizer.GetAuthTag().(names.UserTag)
		var results params.ApplicationOffersResults
		results.Results = make([]params.ApplicationOfferResult, len(urls.OfferURLs))

		var (
			filters []params.OfferFilter
			// fullURLs contains the URL strings from the url args,
			// with any optional parts like model owner filled in.
			// It is used to process the result offers.
			fullURLs []string
			// urlArgs is the unmodified URLs without any username -> qualifier mapping.
			// This is used to report any errors to the caller.
			// Legacy callers can supply an offer URL with a username in the URL
			// and we want to report back that same URL if there's an error.
			urlArgs []string
		)
		for i, urlStr := range urls.OfferURLs {
			url, err := jujucrossmodel.ParseOfferURL(urlStr)
			if err != nil {
				results.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			if url.ModelQualifier == "" {
				url.ModelQualifier = user.Id()
			}
			urlCopy := *url
			// Older clients may try to reference an offer with a model owner username.
			// Create a URL ensuring a valid model qualifier is always used.
			url.ModelQualifier = model.QualifierFromUserTag(names.NewUserTag(url.ModelQualifier)).String()
			if url.HasEndpoint() {
				results.Results[i].Error = apiservererrors.ServerError(
					errors.Errorf("saas application %q shouldn't include endpoint", urlCopy.String()))
				continue
			}
			if url.Source != "" {
				results.Results[i].Error = apiservererrors.ServerError(
					errors.NotSupportedf("query for non-local application offers"))
				continue
			}
			fullURLs = append(fullURLs, url.String())
			urlArgs = append(urlArgs, urlCopy.String())
			filters = append(filters, api.filterFromURL(url))
		}
		if len(filters) == 0 {
			return results, nil
		}
		offers, err := api.getApplicationOffersDetails(ctx, user, params.OfferFilters{Filters: filters}, permission.ReadAccess)
		if err != nil {
			return results, apiservererrors.ServerError(err)
		}
		offersByURL := make(map[string]params.ApplicationOfferAdminDetailsV5)
		for _, offer := range offers {
			offersByURL[offer.OfferURL] = offer
		}
		for i, urlStr := range fullURLs {
			offer, ok := offersByURL[urlStr]
			if !ok {
				err = errors.NotFoundf("application offer %q", urlArgs[i])
				results.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			results.Results[i].Result = &offer
		}
		return results, nil
	*/
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
