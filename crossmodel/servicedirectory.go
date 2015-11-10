// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// A ServiceDirectory holds service offerings from external environments
// and provides helper methods to access the offerings.
type ServiceDirectory struct {
	ServiceDirectoryProvider
}

// ServiceForURL returns a service offer for the specified URL
// so long as the specified user has been granted access to use that offer.
func (s ServiceDirectory) ServiceForURL(url string, user names.UserTag) (params.ServiceOfferDetails, error) {
	if _, err := ParseServiceURL(url); err != nil {
		return params.ServiceOfferDetails{}, err
	}
	results, err := s.ListOffers(
		params.OfferFilter{
			ServiceURL:      url,
			AllowedUserTags: []string{user.String()},
		},
	)
	if err != nil {
		return params.ServiceOfferDetails{}, errors.Trace(err)
	}
	if len(results) != 1 {
		err := errors.Errorf("expected 1 result, got %d", len(results))
		return params.ServiceOfferDetails{}, err
	}
	return results[0].ServiceOfferDetails, nil
}

// NewEmbeddedServiceDirectory creates a service directory used by a Juju controller.
func NewEmbeddedServiceDirectory(st *state.State) ServiceDirectoryProvider {
	return &controllerServiceDirectory{st}
}

type controllerServiceDirectory struct {
	st *state.State
}

func (s *controllerServiceDirectory) AddOffer(url string, offerDetails params.ServiceOfferDetails, users []names.UserTag) error {
	// TODO(wallyworld) - implement
	return errors.NewNotImplemented(nil, "add offer")
}

func (s *controllerServiceDirectory) ListOffers(filters ...params.OfferFilter) ([]params.ServiceOffer, error) {
	// TODO(wallyworld) - implement
	return nil, errors.NewNotImplemented(nil, "list offers")
}
