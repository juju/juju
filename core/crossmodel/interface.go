// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// ApplicationOffer holds the details of an application offered
// by this model.
type ApplicationOffer struct {
	// OfferName is the name of the offer.
	OfferName string

	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string

	// Endpoints is the collection of endpoint names offered (internal->published).
	// The map allows for advertised endpoint names to be aliased.
	Endpoints map[string]charm.Relation
}

// AddApplicationOfferArgs contain parameters used to create an application offer.
type AddApplicationOfferArgs struct {
	// OfferName is the name of the offer.
	OfferName string

	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string

	// Endpoints is the collection of endpoint names offered (internal->published).
	// The map allows for advertised endpoint names to be aliased.
	Endpoints map[string]string

	// Icon is an icon to display when browsing the ApplicationOffers, which by default
	// comes from the charm.
	Icon []byte
}

// String returns the offered application name.
func (s *ApplicationOffer) String() string {
	return s.ApplicationName
}

// ApplicationOfferFilter is used to query applications offered
// by this model.
type ApplicationOfferFilter struct {
	// OfferName is the name of the model hosting the offer.
	ModelName string

	// OfferName is the name of the offer.
	OfferName string

	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string

	// Endpoint contains an endpoint filter criteria.
	Endpoints []EndpointFilterTerm

	// AllowedUsers are the users allowed to consume the application.
	AllowedUsers []string
}

// EndpointFilterTerm represents a remote endpoint filter.
type EndpointFilterTerm struct {
	// Name is an endpoint name.
	Name string

	// Interface is an endpoint interface.
	Interface string

	// Role is an endpoint role.
	Role charm.RelationRole
}

// An ApplicationOffers instance holds application offers from a model.
type ApplicationOffers interface {

	// AddOffer adds a new application offer to the directory.
	AddOffer(offer AddApplicationOfferArgs) (*ApplicationOffer, error)

	// UpdateOffer replaces an existing offer at the same URL.
	UpdateOffer(offer AddApplicationOfferArgs) (*ApplicationOffer, error)

	// ListOffers returns the offers satisfying the specified filter.
	ListOffers(filter ...ApplicationOfferFilter) ([]ApplicationOffer, error)

	// Remove removes the application offer at the specified URL.
	Remove(offerName string) error
}

// RemoteApplication represents a remote application.
type RemoteApplication struct {
	ApplicationOffer

	// ConnectedUsers are the users that are consuming the application.
	ConnectedUsers []string
}
