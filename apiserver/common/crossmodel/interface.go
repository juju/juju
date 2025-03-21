// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/state"
)

type BakeryConfigService interface {
	GetExternalUsersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error)
}

type Backend interface {
	// AllModelUUIDs returns the UUIDs of all models in the controller.
	AllModelUUIDs() ([]string, error)

	// ControllerTag the tag of the controller in which we are operating.
	ControllerTag() names.ControllerTag

	// KeyRelation returns the existing relation with the given key (which can
	// be derived unambiguously from the relation's endpoints).
	KeyRelation(string) (Relation, error)

	// Application returns a local application by name.
	Application(string) (Application, error)

	// RemoteApplication returns a remote application by name.
	RemoteApplication(string) (RemoteApplication, error)

	// AddRelation adds a relation between the specified endpoints and returns the relation info.
	AddRelation(...relation.Endpoint) (Relation, error)

	// EndpointsRelation returns the existing relation with the given endpoints.
	EndpointsRelation(...relation.Endpoint) (Relation, error)

	// OfferConnectionForRelation get the offer connection for a cross model relation.
	OfferConnectionForRelation(string) (OfferConnection, error)

	// AddRemoteApplication creates a new remote application record, having the supplied relation endpoints,
	// with the supplied name (which must be unique across all applications, local and remote).
	AddRemoteApplication(AddRemoteApplicationParams) (RemoteApplication, error)

	// OfferUUIDForRelation gets the uuid of the offer for the
	// specified cross-model relation key.
	OfferUUIDForRelation(string) (string, error)

	// GetRemoteEntity returns the tag of the entity associated with the given token.
	GetRemoteEntity(string) (names.Tag, error)

	// GetToken returns the token associated with the entity with the given tag.
	GetToken(entity names.Tag) (string, error)

	// ExportLocalEntity adds an entity to the remote entities collection,
	// returning an opaque token that uniquely identifies the entity within
	// the model.
	ExportLocalEntity(names.Tag) (string, error)

	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(entity names.Tag, token string) error

	// SaveIngressNetworks stores in state the ingress networks for the relation.
	SaveIngressNetworks(relationKey string, cidrs []string) (RelationNetworks, error)

	// IngressNetworks returns the networks for the specified relation.
	IngressNetworks(relationKey string) (RelationNetworks, error)

	// ApplicationOfferForUUID returns the application offer for the UUID.
	ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error)

	// ApplyOperation applies a model operation to the state.
	ApplyOperation(op state.ModelOperation) error

	// AllRemoteApplications returns a list of all remote applications available in the model.
	AllRemoteApplications() ([]RemoteApplication, error)
}

// OfferConnection provides access to an offer connection in state.
type OfferConnection interface {
	UserName() string
	OfferUUID() string
}

// Relation provides access a relation in global state.
type Relation interface {
	status.StatusGetter
	status.StatusSetter
	// Destroy ensures that the relation will be removed at some point; if
	// no units are currently in scope, it will be removed immediately.
	Destroy(objectstore.ObjectStore) error

	// DestroyWithForce may force the destruction of the relation.
	// In addition, this function also returns all non-fatal operational errors
	// encountered.
	DestroyWithForce(force bool, maxWait time.Duration) ([]error, error)

	// Id returns the integer internal relation key.
	Id() int

	// Life returns the relation's current life state.
	Life() state.Life

	// Tag returns the relation's tag.
	Tag() names.Tag

	// UnitCount is the number of units still in relation scope.
	UnitCount() int

	// RemoteUnit returns a RelationUnit for the remote application unit
	// with the supplied ID.
	RemoteUnit(unitId string) (RelationUnit, error)

	// AllRemoteUnits returns all the RelationUnits for the remote
	// application units for a given application.
	AllRemoteUnits(appName string) ([]RelationUnit, error)

	// Endpoints returns the endpoints that constitute the relation.
	Endpoints() []relation.Endpoint

	// Endpoint returns the endpoint of the relation for the named application.
	Endpoint(appName string) (relation.Endpoint, error)

	// Unit returns a RelationUnit for the unit with the supplied ID.
	Unit(unitId string) (RelationUnit, error)

	// WatchUnits returns a watcher that notifies of changes to the units of the
	// specified application in the relation.
	WatchUnits(applicationName string) (relation.RelationUnitsWatcher, error)

	// WatchLifeSuspendedStatus returns a watcher that notifies of changes to the life
	// or suspended status of the relation.
	WatchLifeSuspendedStatus() state.StringsWatcher

	// Suspended returns the suspended status of the relation.
	Suspended() bool

	// SuspendedReason returns the reason why the relation is suspended.
	SuspendedReason() string

	// SetSuspended sets the suspended status of the relation.
	SetSuspended(bool, string) error

	// ReplaceApplicationSettings replaces the application's settings within the
	// relation.
	ReplaceApplicationSettings(appName string, settings map[string]interface{}) error

	// ApplicationSettings returns the settings for the specified
	// application in the relation.
	ApplicationSettings(appName string) (map[string]interface{}, error)

	// RemoteApplication returns the remote application the relation is crossmodel,
	// or false if it is not
	RemoteApplication() (RemoteApplication, bool, error)

	// RelatedEndpoints returns the endpoints of the relation with which
	// units of the named application will establish relations. If the application
	// is not part of the relation r, an error will be returned.
	RelatedEndpoints(name string) ([]relation.Endpoint, error)
}

// RelationNetworks instances describe the ingress or egress
// networks required for a cross model relation.
type RelationNetworks interface {
	Id() string
	RelationKey() string
	CIDRS() []string
}

// RelationUnit provides access to the settings of a single unit in a relation,
// and methods for modifying the unit's involvement in the relation.
type RelationUnit interface {
	// EnterScope ensures that the unit has entered its scope in the
	// relation. When the unit has already entered its scope, EnterScope
	// will report success but make no changes to state.
	EnterScope(settings map[string]interface{}) error

	// InScope returns whether the relation unit has entered scope and
	// not left it.
	InScope() (bool, error)

	// LeaveScope signals that the unit has left its scope in the relation.
	// After the unit has left its relation scope, it is no longer a member
	// of the relation; if the relation is dying when its last member unit
	// leaves, it is removed immediately. It is not an error to leave a
	// scope that the unit is not, or never was, a member of.
	LeaveScope() error

	// Settings returns the relation unit's settings within the relation.
	Settings() (map[string]interface{}, error)

	// ReplaceSettings replaces the relation unit's settings within the
	// relation.
	ReplaceSettings(map[string]interface{}) error
}

// Application represents the state of a application hosted in the local model.
type Application interface {
	// Name is the name of the application.
	Name() string

	// Life returns the lifecycle state of the application.
	Life() state.Life

	// Endpoints returns the application's currently available relation endpoints.
	Endpoints() ([]relation.Endpoint, error)

	// CharmURL returns a string representation the application's charm URL,
	// and whether units should upgrade to the charm with that URL even if
	// they are in an error state.
	CharmURL() (curl *string, force bool)

	// EndpointBindings returns the Bindings object for this application.
	EndpointBindings() (Bindings, error)
}

// Bindings defines a subset of the functionality provided by the
// state.Bindings type, as required by the application facade. For
// details on the methods, see the methods on state.Bindings with
// the same names.
type Bindings interface {
	MapWithSpaceNames(network.SpaceInfos) (map[string]string, error)
}

type Charm interface {
	// Meta returns the metadata of the charm.
	Meta() *charm.Meta

	// StoragePath returns the storage path of the charm bundle.
	StoragePath() string
}

// RemoteApplication represents the state of an application hosted in an external
// (remote) model.
type RemoteApplication interface {
	// DestroyWithForce in addition to doing what Destroy() does,
	// when force is passed in as 'true', forces th destruction of remote application,
	// ignoring errors.
	DestroyWithForce(force bool, maxWait time.Duration) (opErrs []error, err error)

	// Name returns the name of the remote application.
	Name() string

	// Tag returns the remote applications's tag.
	Tag() names.Tag

	// URL returns the offer URL, at which the application is offered.
	URL() (string, bool)

	// OfferUUID returns the UUID of the offer.
	OfferUUID() string

	// SourceModel returns the tag of the model hosting the remote application.
	SourceModel() names.ModelTag

	// Macaroon returns the macaroon used for authentication.
	Macaroon() (*macaroon.Macaroon, error)

	// Status returns the status of the remote application.
	Status() (status.StatusInfo, error)

	// IsConsumerProxy returns whether application is created
	// from a registration operation by a consuming model.
	IsConsumerProxy() bool

	// ConsumeVersion increments each time a new saas proxy
	// for the same offer is created.
	ConsumeVersion() int

	// Life returns the lifecycle state of the application.
	Life() state.Life

	// SetStatus sets the status of the remote application.
	SetStatus(info status.StatusInfo) error

	// TerminateOperation returns an operation that will set this
	// remote application to terminated and leave it in a state
	// enabling it to be removed cleanly.
	TerminateOperation(string) state.ModelOperation

	// DestroyOperation returns a model operation to destroy remote application.
	DestroyOperation(bool) state.ModelOperation

	// SetSourceController updates the source controller attribute.
	SetSourceController(uuid string) error

	// SourceController returns the UUID of the source controller associated
	// with the remote application.
	SourceController() string

	// Endpoints returns the application's currently available relation endpoints.
	Endpoints() ([]relation.Endpoint, error)

	// AddEndpoints adds the specified endpoints to the remote application.
	// If an endpoint with the same name already exists, an error is returned.
	// If the endpoints change during the update, the operation is retried.
	AddEndpoints(eps []charm.Relation) error

	// Destroy ensures that this remote application reference and all its relations
	// will be removed at some point; if no relation involving the
	// application has any units in scope, they are all removed immediately.
	Destroy() error
}

// AccessService provides information about users and permissions.
type AccessService interface {
	// ReadUserAccessLevelForTarget returns the subject's (user) access level
	// for the given user on the given target.
	// If the access level of a user cannot be found then
	// accesserrors.AccessNotFound is returned.
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target permission.ID) (permission.Access, error)
}
