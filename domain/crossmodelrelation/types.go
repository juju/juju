// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelation

import (
	"maps"
	"slices"
	"time"

	"gopkg.in/macaroon.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
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

// ConsumeDetails contains details about a offer which is being consumed.
type ConsumeDetails struct {
	OfferUUID string
	Endpoints []OfferEndpoint
}

// OfferDetailWithConnections contains details about an offer and its connections.
type OfferDetailWithConnections struct {
	OfferDetail
	OfferConnections []OfferConnection
}

// OfferConnection holds details about a connection to an offer.
type OfferConnection struct {
	// Username is the name of the user consuming the offer.
	Username string

	// RelationId is the id of the relation for this connection.
	RelationId int

	// Endpoint is the endpoint being connected to.
	Endpoint string

	// Status is the status of the offer connection.
	Status status.Status

	// Message is the status message of the offer connection.
	Message string

	// Since is when the status value was last changed.
	Since *time.Time

	// IngressSubnets is the list of subnets from which traffic will originate.
	IngressSubnets []string
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

	// OfferURL is the URL of this offer, used to located an offered appliction
	// and it's exported endpoints
	OfferURL string

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

	// OfferURL is the URL of this offer, the natural key for the offer to
	// identify the in offer in the offering model.
	OfferURL string

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
	// CharmUUID is the UUID to assign to the synthetic charm representing
	// the remote application, on the consuming model.
	CharmUUID string

	// Charm is the charm representing the remote application, on the consuming
	// model.
	Charm charm.Charm

	// OfferUUID is the UUID of the offer that the remote application is
	// consuming. The offer is in this model, the offering model.
	OfferUUID string

	// OfferEndpointName is the name of the offering application endpoint
	// to be used in the relation.
	OfferEndpointName string

	// ConsumerModelUUID is the UUID of the model that is consuming the
	// application.
	ConsumerModelUUID string

	// RelationUUID is the UUID of the relation created to connect the remote
	// application to a local application, on the consuming model.
	RelationUUID string

	// ConsumerApplicationUUID is the UUID of the consuming application UUID.
	ConsumerApplicationUUID string

	// ConsumerApplicationEndpoint is the relation endpoint name of the
	// consuming application.
	ConsumerApplicationEndpoint string

	// SynthApplicationUUID is the UUID of the synthetic application created
	// to represent the remote application, on the consuming model.
	SynthApplicationUUID string

	// Username is the name of the user making the request.
	Username string
}

// CreateOfferArgs contains parameters used to create an offer.
type CreateOfferArgs struct {
	// UUID is the unique identifier of the new offer.
	UUID offer.UUID

	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// Endpoints is the collection of endpoint names offered.
	Endpoints []string

	// OfferName is the name of the offer.
	OfferName string
}

// MakeCreateOfferArgs returns a CreateOfferArgs from the given
// ApplicationOfferArgs and uuid.
func MakeCreateOfferArgs(in ApplicationOfferArgs, offerUUID offer.UUID) CreateOfferArgs {
	return CreateOfferArgs{
		UUID:            offerUUID,
		ApplicationName: in.ApplicationName,
		// There was an original intention to allow for endpoint aliases,
		// however it was never implemented. Just use the maps keys from
		// here.
		Endpoints: slices.Collect(maps.Keys(in.Endpoints)),
		OfferName: in.OfferName,
	}
}

// OfferFilter is used to query applications offered
// by this model.
type OfferFilter struct {
	// OfferUUIDs is a list of offerUUIDs to find based on the
	// OfferFilter.AllowedConsumers.
	OfferUUIDs []string

	// OfferName is the name of the offer.
	OfferName string

	// ApplicationName is the name of the application to which the offer pertains.
	ApplicationName string

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string

	// Endpoint contains an endpoint filter criteria.
	Endpoints []EndpointFilterTerm
}

// Empty checks to see if the filter has any values. An empty
// filter indicates all offers should be found.
func (f OfferFilter) Empty() bool {
	return f.OfferName == "" &&
		f.ApplicationName == "" &&
		f.ApplicationDescription == "" &&
		len(f.Endpoints) == 0 &&
		len(f.OfferUUIDs) == 0
}

// EmptyModuloEndpoints does the same as Empty, but not including endpoints.
func (f OfferFilter) EmptyModuloEndpoints() bool {
	return f.OfferName == "" &&
		f.ApplicationName == "" &&
		f.ApplicationDescription == ""
}
