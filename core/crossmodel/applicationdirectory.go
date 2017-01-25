// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
)

type ApplicationOfferLister interface {
	// ListOffers returns the offers from the specified directory satisfying the specified filter.
	ListOffers(directory string, filter ...ApplicationOfferFilter) ([]ApplicationOffer, error)
}

// ApplicationOfferForURL returns a application offer for the specified URL
// so long as the specified user has been granted access to use that offer.
func ApplicationOfferForURL(offers ApplicationOfferLister, urlStr string, user string) (ApplicationOffer, error) {
	url, err := ParseApplicationURL(urlStr)
	if err != nil {
		return ApplicationOffer{}, err
	}
	results, err := offers.ListOffers(
		url.Directory,
		ApplicationOfferFilter{
			ApplicationOffer: ApplicationOffer{
				ApplicationURL: urlStr,
			},
			AllowedUsers: []string{user},
		},
	)
	if err != nil {
		return ApplicationOffer{}, errors.Trace(err)
	}
	if len(results) == 0 {
		return ApplicationOffer{}, errors.NotFoundf("application offer at %q", url)
	}
	if len(results) != 1 {
		return ApplicationOffer{}, errors.Errorf("expected 1 result, got %d", len(results))
	}
	return results[0], nil
}
