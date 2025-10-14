// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelation

import (
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/relation"
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

	// TotalConnections is the total number of remote connections connected to
	// the offer.
	TotalConnections int

	// TotalActiveConnections is the total number of remote connections
	// connected to the offer that are currently active.
	TotalActiveConnections int
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
	// ApplicationUUID is the UUID of the synthetic application
	// representing the remote application.
	ApplicationUUID string

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

// RemoteApplicationOfferer represents a remote application
type RemoteApplicationOfferer struct {
	// ApplicationUUID is the UUID of the synthetic application
	// representing the remote application.
	ApplicationUUID string

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

// AddRemoteApplicationOffererArgs contains the parameters required to add a new
// remote application offerer.
type AddRemoteApplicationOffererArgs struct {
	AddRemoteApplicationArgs

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

// AddRemoteApplicationConsumerArgs contains the parameters required to add a
// new remote application consumer.
type AddRemoteApplicationConsumerArgs struct {
	AddRemoteApplicationArgs

	// RelationUUID is the UUID of the relation created to connect the remote
	// application to a local application, on the consuming model.
	RelationUUID string
}

// AddRemoteApplicationArgs contains the parameters required to add a new remote
// application, on any of the offering or the consuming models.
type AddRemoteApplicationArgs struct {
	// ApplicationUUID is the UUID to assign to the synthetic application
	// representing the remote application, on the consuming model.
	ApplicationUUID string

	// CharmUUID is the UUID to assign to the synthetic charm representing
	// the remote application, on the consuming model.
	CharmUUID string

	// RemoteApplicationUUID is the UUID of the remote application, on the
	// consuming model.
	RemoteApplicationUUID string

	// Charm is the charm representing the remote application, on the consuming
	// model.
	Charm charm.Charm

	// OfferUUID is the UUID of the offer that the remote application is
	// consuming. The offer is in this model, the offering model.
	OfferUUID string

	// ConsumerModelUUID is the UUID of the model that is consuming the
	// application.
	ConsumerModelUUID string
}

// RemoteRelationChangedArgs contains the parameters required to process a
// remote relation change event.
type RemoteRelationChangedArgs struct {
	// RelationUUID is used to identify the relation that has changed.
	RelationUUID relation.UUID
	// ApplicationUUID is used to identify the remote application that
	// is connected to the relation.
	ApplicationUUID application.UUID
	// Suspended indicates whether the remote application is suspended.
	Suspended bool
	// SuspendedReason provides a reason for the suspension, if applicable.
	SuspendedReason string
}

// ApplicationRemoteRelation represents a remote relation mapping between the
// synthetic relation (relation_uuid) created in the offering model and the
// original consumer relation UUID (consumer_relation_uuid) provided by the
// consuming model. This is returned by service/state queries that look up
// remote relations via the consumer relation UUID.
type ApplicationRemoteRelation struct {
	// RelationUUID is the UUID of the synthetic relation created in the
	// offering model that mirrors the consumer model's relation.
	RelationUUID string

	// ConsumerRelationUUID is the UUID of the relation as it exists in the
	// consuming model (the original relation UUID provided to the offering
	// model when registering the remote relation).
	ConsumerRelationUUID string
}
