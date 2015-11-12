// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
)

type ServiceOfficeLister interface {
	// List offers returns the offers satisfying the specified filter.
	ListOffers(filter ...ServiceOfferFilter) ([]ServiceOffer, error)
}

// A ServiceDirectoryWrapper holds service offerings from external environments
// and provides helper methods to access the offerings.
type ServiceDirectoryWrapper struct {
	ServiceOfficeLister
}

// ServiceForURL returns a service offer for the specified URL
// so long as the specified user has been granted access to use that offer.
func (s ServiceDirectoryWrapper) ServiceForURL(url string, user string) (ServiceOffer, error) {
	if _, err := ParseServiceURL(url); err != nil {
		return ServiceOffer{}, err
	}
	results, err := s.ListOffers(
		ServiceOfferFilter{
			ServiceOffer: ServiceOffer{
				ServiceURL: url,
			},
			AllowedUsers: []string{user},
		},
	)
	if err != nil {
		return ServiceOffer{}, errors.Trace(err)
	}
	if len(results) == 0 {
		return ServiceOffer{}, errors.NotFoundf("service offer at %q", url)
	}
	if len(results) != 1 {
		return ServiceOffer{}, errors.Errorf("expected 1 result, got %d", len(results))
	}
	return results[0], nil
}
