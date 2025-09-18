// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelation

import (
	"gopkg.in/macaroon.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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

// OfferDetail contains details about an offer.
type OfferDetail struct {
	// OfferUUID is the UUID of the offer.
	OfferUUID string

	// OfferName is the name of the offer.
	OfferName string

	// ApplicationName is the name of the application.
	ApplicationName string

	// ApplicationDescription is a description of the application's
	// functionality, typically copied from the charm metadata.
	ApplicationDescription string

	// CharmLocator represents the parts of a charm that are needed to
	// locate it in the same way as a charm URL.
	CharmLocator charm.CharmLocator

	// Endpoints is a slice of charm endpoints encompassed by this offer.
	Endpoints []OfferEndpoint

	// AllowedConsumers includes user which have admin or consume access
	// to the offer.
	OfferUsers []OfferUser

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

// OfferUser contains details of users with access to the offer.
type OfferUser struct {
	Name        string
	DisplayName string
	Access      permission.Access
}

// OfferImport contains details to import an offer during migration.
type OfferImport struct {
	UUID            uuid.UUID
	Name            string
	ApplicationName string
	Endpoints       []string
}

// RemoteApplicationConsumer represents a remote application
// that is consuming an offer from this model.
type RemoteApplicationConsumer struct {
	// ApplicationName is the name of the remote application.
	ApplicationName string

	// Life is the lifecycle state of the remote application.
	Life life.Life

	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID string

	// ConsumeVersion is the version of the offer that the remote application is
	// consuming.
	ConsumeVersion int

	// OffererModelUUID is the UUID of the model that is offering the
	// application.
	OffererModelUUID string

	// Macaroon is the macaroon that the remote application uses to
	// authenticate with the offerer model.
	Macaroon *macaroon.Macaroon
}

// RemoteApplicationOfferer represents a remote application
type RemoteApplicationOfferer struct {
	// ApplicationName is the name of the remote application.
	ApplicationName string

	// Life is the lifecycle state of the remote application.
	Life life.Life

	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID string

	// ConsumeVersion is the version of the offer that the remote application is
	// consuming.
	ConsumeVersion int
}

// AddRemoteApplicationOffererArgs contains the parameters required to add a new
// remote application offerer.
type AddRemoteApplicationOffererArgs struct {
	// Charm is the charm representing the remote application.
	Charm charm.Charm

	// OfferUUID is the UUID of the offer that the remote application is
	// consuming.
	OfferUUID string

	// OffererControllerUUID is the UUID of the controller that the remote
	// application is in.
	OffererControllerUUID *string

	// OffererModelUUID is the UUID of the model that is offering the
	// application.
	OffererModelUUID string

	// EncodedMacaroon is the encoded macaroon that the remote application uses
	// to authenticate with the offerer model.
	EncodedMacaroon []byte
}
