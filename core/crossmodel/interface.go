// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"

	"gopkg.in/juju/charm.v6-unstable"
)

// ApplicationOffer represents the state of an application hosted
// in an external (remote) model.
type ApplicationOffer struct {
	// ApplicationURL is the URL used to locate the offer in a directory.
	ApplicationURL string

	// ApplicationName is the name of the application.
	ApplicationName string

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string

	// Endpoints are the charm endpoints supported by the application.
	Endpoints []charm.Relation

	// SourceModelUUID is the UUID of the model hosting the application.
	SourceModelUUID string

	// SourceLabel is a user friendly name for the source model.
	SourceLabel string
}

// String returns the directory record name.
func (s *ApplicationOffer) String() string {
	return fmt.Sprintf("%s-%s", s.SourceModelUUID, s.ApplicationName)
}

// ApplicationOfferFilter is used to query offers in a application directory.
// We allow filtering on any of the application offer attributes plus
// users allowed to consume the application.
type ApplicationOfferFilter struct {
	ApplicationOffer

	// AllowedUsers are the users allowed to consume the application.
	AllowedUsers []string
}

// An ApplicationDirectory holds application offers from external models.
type ApplicationDirectory interface {

	// AddOffer adds a new application offer to the directory.
	AddOffer(offer ApplicationOffer) error

	// UpdateOffer replaces an existing offer at the same URL.
	UpdateOffer(offer ApplicationOffer) error

	// ListOffers returns the offers satisfying the specified filter.
	ListOffers(filter ...ApplicationOfferFilter) ([]ApplicationOffer, error)

	// Remove removes the application offer at the specified URL.
	Remove(url string) error
}

// OfferedApplication holds the details of applications offered
// by this model.
type OfferedApplication struct {
	// ApplicationName is the application name.
	ApplicationName string

	// ApplicationURL is the URL where the application can be located.
	ApplicationURL string

	// CharmName is the name of the charm used to deploy the offered application.
	CharmName string

	// Endpoints is the collection of endpoint names offered (internal->published).
	// The map allows for advertised endpoint names to be aliased.
	Endpoints map[string]string

	// Description is a description of the application, which by default comes
	// from the charm metadata.
	Description string

	// Icon is an icon to display when browsing the ApplicationDirectory, which by default
	// comes from the charm.
	Icon []byte

	// Registered is true if this offer is to be registered with
	// the relevant application directory.
	Registered bool
}

// OfferedApplicationFilter is used to query applications offered
// by this model.
type OfferedApplicationFilter struct {
	// ApplicationName is the application name.
	ApplicationName string

	// CharmName is the name of the charm of the application.
	CharmName string

	// ApplicationURL is the URL where the application can be located.
	ApplicationURL string

	// Registered, if non-nil, returns only the offered applications
	// that are registered or not.
	Registered *bool

	// Endpoint contains an endpoint filter criteria.
	Endpoint EndpointFilterTerm
}

// RegisteredFilter is a helper function for creating an offered application filter.
func RegisteredFilter(registered bool) OfferedApplicationFilter {
	var filter OfferedApplicationFilter
	filter.Registered = &registered
	return filter
}

// OfferedApplications instances hold application offers from this model.
type OfferedApplications interface {

	// AddOffer adds a new application offer.
	AddOffer(offer OfferedApplication) error

	// UpdateOffer updates an existing application offer.
	UpdateOffer(url string, endpoints map[string]string) error

	// ListOffers returns the offers satisfying the specified filter.
	ListOffers(filter ...OfferedApplicationFilter) ([]OfferedApplication, error)

	// SetOfferRegistered marks a previously saved offer as registered or not.
	SetOfferRegistered(url string, registered bool) error

	// RemoveOffer removes the application offer at the specified URL.
	RemoveOffer(url string) error
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

// RemoteApplication represents a remote application.
type RemoteApplication struct {
	ApplicationOffer

	// ConnectedUsers are the users that are consuming the application.
	ConnectedUsers []string
}
