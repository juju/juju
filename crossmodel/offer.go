// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

// ExportOffer prepares service endpoints for consumption.
func ExportOffer(offer Offer) error {
	// TODO(anastasiamac 2015-11-02) needs the actual implementation - this is a placeholder.
	// actual implementation will coordinate the work:
	// validate entities exist, access the service directory, write to state etc.
	TempPlaceholder[offer.Service()] = offer
	return nil
}

// Search looks through offered services and returns the ones
// that match speified filter.
func Search(filter params.SAASSearchFilter) ([]ServiceDetails, error) {
	// TODO(anastasiamac 2015-11-02) needs the actual implementation - this is a placeholder.

	byURL := make(map[string][]ServiceDetails, len(TempPlaceholder))
	for _, v := range TempPlaceholder {
		//TODO(anastasiamac 2015-11-4) Pull description from the charm metadata
		s := service{baseDetails{v.Service(), v.Endpoints()}, ""}
		byURL[v.URL()] = append(byURL[v.URL()], &s)
	}
	return byURL[filter.URL], nil
}

type baseDetails struct {
	service   names.ServiceTag
	endpoints []string
}

// Service implements BaseDetails.Service.
func (b *baseDetails) Service() names.ServiceTag {
	return b.service
}

// Endpoints implements BaseDetails.Endpoints.
func (b *baseDetails) Endpoints() []string {
	return b.endpoints
}

type anOffer struct {
	baseDetails
	aURL  string
	users []names.UserTag
}

func NewOffer(serviceTag names.ServiceTag, endpoints []string, URL string, users []names.UserTag) Offer {
	offer := anOffer{
		baseDetails{serviceTag, endpoints},
		URL,
		users,
	}
	return &offer
}

// URL implements Offer.URL.
func (o *anOffer) URL() string {
	return o.aURL
}

// Users implements Offer.Users.
func (o *anOffer) Users() []names.UserTag {
	return o.users
}

type service struct {
	baseDetails
	desc string
}

// Description implements ServiceDetails.Description.
func (s *service) Description() string {
	return s.desc
}

// START TEMP IN-MEMORY PLACEHOLDER ///////////////

var TempPlaceholder = make(map[names.ServiceTag]Offer)

// END TEMP IN-MEMORY PLACEHOLDER ///////////////
