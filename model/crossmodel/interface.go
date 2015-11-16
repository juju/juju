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

	// List offers returns the offers satisfying the specified filter.
	ListOffers(filter ...ServiceOfferFilter) ([]ServiceOffer, error)

	// Remove removes the service offer at the specified URL.
	Remove(url string) error
}
