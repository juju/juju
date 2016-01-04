// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"

	"gopkg.in/juju/charm.v6-unstable"
)

// ServiceOffer represents the state of a service hosted
// in an external (remote) environment.
type ServiceOffer struct {
	// ServiceURL is the URL used to locate the offer in a directory.
	ServiceURL string

	// ServiceName is the name of the service.
	ServiceName string

	// ServiceDescription is a description of the service's functionality,
	// typically copied from the charm metadata.
	ServiceDescription string

	// Endpoints are the charm endpoints supported by the service.
	Endpoints []charm.Relation

	// SourceEnvUUID is the UUID of the environment hosting the service.
	SourceEnvUUID string

	// SourceLabel is a user friendly name for the source environment.
	SourceLabel string
}

// String returns the directory record name.
func (s *ServiceOffer) String() string {
	return fmt.Sprintf("%s-%s", s.SourceEnvUUID, s.ServiceName)
}

// ServiceOfferFilter is used to query offers in a service directory.
// We allow filtering on any of the service offer attributes plus
// users allowed to consume the service.
type ServiceOfferFilter struct {
	ServiceOffer

	// AllowedUsers are the users allowed to consume the service.
	AllowedUsers []string
}

// A ServiceDirectory holds service offers from external environments.
type ServiceDirectory interface {

	// AddOffer adds a new service offer to the directory.
	AddOffer(offer ServiceOffer) error

	// UpdateOffer replaces an existing offer at the same URL.
	UpdateOffer(offer ServiceOffer) error

	// ListOffers returns the offers satisfying the specified filter.
	ListOffers(filter ...ServiceOfferFilter) ([]ServiceOffer, error)

	// Remove removes the service offer at the specified URL.
	Remove(url string) error
}

// OfferedService holds the details of services offered
// by this environment.
type OfferedService struct {
	// ServiceName is the service name.
	ServiceName string

	// ServiceURL is the URL where the service can be located.
	ServiceURL string

	// CharmName is the name of the charm used to deploy the offered service.
	CharmName string

	// Endpoints is the collection of endpoint names offered (internal->published).
	// The map allows for advertised endpoint names to be aliased.
	Endpoints map[string]string

	// Description is a description of the service, which by default comes
	// from the charm metadata.
	Description string

	// Icon is an icon to display when browsing the service, which by default
	// comes from the charm.
	Icon []byte

	// Registered is true if this offer is to be registered with
	// the relevant service directory.
	Registered bool
}

// OfferedServiceFilter is used to query services offered
// by this environment.
type OfferedServiceFilter struct {
	// ServiceName is the service name.
	ServiceName string

	// CharmName is the name of the charm of the service.
	CharmName string

	// ServiceURL is the URL where the service can be located.
	ServiceURL string

	// Registered, if non-nil, returns only the offered services
	// that are registered or not.
	Registered *bool

	// Endpoint contains an endpoint filter criteria.
	Endpoint EndpointFilterTerm
}

// RegisteredFilter is a helper function for creating an offered service filter.
func RegisteredFilter(registered bool) OfferedServiceFilter {
	var filter OfferedServiceFilter
	filter.Registered = &registered
	return filter
}

// OfferedServices instances hold service offers from this environment.
type OfferedServices interface {

	// AddOffer adds a new service offer.
	AddOffer(offer OfferedService) error

	// UpdateOffer updates an existing service offer.
	UpdateOffer(url string, endpoints map[string]string) error

	// ListOffers returns the offers satisfying the specified filter.
	ListOffers(filter ...OfferedServiceFilter) ([]OfferedService, error)

	// SetOfferRegistered marks a previously saved offer as registered or not.
	SetOfferRegistered(url string, registered bool) error

	// RemoveOffer removes the service offer at the specified URL.
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

// RemoteService represents a remote service.
type RemoteService struct {
	ServiceOffer

	// ConnectedUsers are the users that are consuming the service.
	ConnectedUsers []string
}
