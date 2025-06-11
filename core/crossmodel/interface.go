// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// ApplicationOffer holds the details of an application offered
// by this model.
type ApplicationOffer struct {
	// OfferUUID is the UUID of the offer.
	OfferUUID string

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

// AddApplicationOfferArgs contains parameters used to create an application offer.
type AddApplicationOfferArgs struct {
	// OfferName is the name of the offer.
	OfferName string

	// Owner is the user name who owns the offer.
	Owner string

	// HasRead are the user names who can see the offer exists.
	HasRead []string

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

// ConsumeApplicationArgs contains parameters used to consume an offer.
type ConsumeApplicationArgs struct {
	// Offer is the offer to be consumed.
	Offer params.ApplicationOfferDetailsV5

	// Macaroon is used for authentication.
	Macaroon *macaroon.Macaroon

	// ControllerInfo contains connection details to the controller
	// hosting the offer.
	ControllerInfo *ControllerInfo

	// ApplicationAlias is the name of the alias to use for the application name.
	ApplicationAlias string
}

// String returns the offered application name.
func (s *ApplicationOffer) String() string {
	return s.ApplicationName
}

// ApplicationOfferFilter is used to query applications offered
// by this model.
type ApplicationOfferFilter struct {
	// ModelQualifier disambiguates the name of the model hosting the offer.
	ModelQualifier model.Qualifier

	// ModelName is the name of the model hosting the offer.
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

	// AllowedConsumers are the users allowed to consume the offer.
	AllowedConsumers []string

	// ConnectedUsers are the users currently related to the offer.
	ConnectedUsers []string
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

	// ApplicationOffer returns the named application offer.
	ApplicationOffer(offerName string) (*ApplicationOffer, error)

	// ApplicationOfferForUUID returns the application offer with the UUID.
	ApplicationOfferForUUID(offerUUID string) (*ApplicationOffer, error)

	// ListOffers returns the offers satisfying the specified filter.
	ListOffers(filter ...ApplicationOfferFilter) ([]ApplicationOffer, error)

	// Remove removes the application offer at the specified URL.
	Remove(offerName string, force bool) error

	// AllApplicationOffers returns all application offers in the model.
	AllApplicationOffers() (offers []*ApplicationOffer, _ error)
}

// RemoteApplication represents a remote application.
type RemoteApplication struct {
	ApplicationOffer

	// ConnectedUsers are the users that are consuming the application.
	ConnectedUsers []string
}
