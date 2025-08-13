// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package offer

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/errors"
)

// ApplicationOfferArgs contains parameters used to create or update
// an application offer.
type ApplicationOfferArgs struct {
	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// Endpoints is the collection of endpoint names offered.
	// The map allows for advertised endpoint names to be aliased.
	Endpoints map[string]string

	// OfferName is the name of the offer.
	OfferName string

	// OwnerName is the name of the owner of the offer.
	OwnerName user.Name
}

func (a ApplicationOfferArgs) Validate() error {
	if a.ApplicationName == "" {
		return errors.Errorf("application name cannot be empty").Add(coreerrors.NotValid)
	}
	if a.OwnerName.Name() == "" {
		return errors.Errorf("owner name cannot be empty").Add(coreerrors.NotValid)
	}
	if len(a.Endpoints) == 0 {
		return errors.Errorf("endpoints cannot be empty").Add(coreerrors.NotValid)
	}
	return nil
}

// OfferFilter is used to query applications offered
// by this model.
type OfferFilter struct {
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

// OfferDetails contains details about an offer.
type OfferDetails struct {
	// OfferUUID is the UUID of the offer.
	OfferUUID string

	// OfferName is the name of the offer.
	OfferName string

	// ApplicationName is the name of the application.
	ApplicationName string

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string

	// CharmLocator represents the parts of a charm that are needed to locate it
	// in the same way as a charm URL.
	CharmLocator charm.CharmLocator

	// Endpoints is a slice of charm endpoints encompassed by this offer.
	Endpoints []OfferEndpoint

	// TODO (cmr)
	// Add []OfferConnections.
}

// OfferEndpoint contains details of charm endpoints as needed for offer
// details.
type OfferEndpoint struct {
	Name      string
	Role      charm.RelationRole
	Interface string
	Limit     int
}
