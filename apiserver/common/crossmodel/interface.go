// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"time"

	"github.com/juju/charm/v13"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type Backend interface {
	// ModelUUID returns the model UUID for the model
	// controlled by this state instance.
	ModelUUID() string

	// ModelTag the tag of the model on which we are operating.
	ModelTag() names.ModelTag

	// ModelConfig returns the complete config for the model
	ModelConfig(context.Context) (*config.Config, error)

	// AllModelUUIDs returns the UUIDs of all models in the controller.
	AllModelUUIDs() ([]string, error)

	// ControllerTag the tag of the controller in which we are operating.
	ControllerTag() names.ControllerTag

	// KeyRelation returns the existing relation with the given key (which can
	// be derived unambiguously from the relation's endpoints).
	KeyRelation(string) (Relation, error)

	// Application returns a local application by name.
	Application(string) (Application, error)

	// GetOfferAccess gets the access permission for the specified user on an offer.
	GetOfferAccess(offerUUID string, user names.UserTag) (permission.Access, error)

	// UserPermission returns the access permission for the passed subject and target.
	UserPermission(subject names.UserTag, target names.Tag) (permission.Access, error)

	// RemoteApplication returns a remote application by name.
	RemoteApplication(string) (RemoteApplication, error)

	// AddRelation adds a relation between the specified endpoints and returns the relation info.
	AddRelation(...state.Endpoint) (Relation, error)

	// EndpointsRelation returns the existing relation with the given endpoints.
	EndpointsRelation(...state.Endpoint) (Relation, error)

	// OfferConnectionForRelation get the offer connection for a cross model relation.
	OfferConnectionForRelation(string) (OfferConnection, error)

	// AddRemoteApplication creates a new remote application record, having the supplied relation endpoints,
	// with the supplied name (which must be unique across all applications, local and remote).
	AddRemoteApplication(state.AddRemoteApplicationParams) (RemoteApplication, error)

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
	SaveIngressNetworks(relationKey string, cidrs []string) (state.RelationNetworks, error)

	// IngressNetworks returns the networks for the specified relation.
	IngressNetworks(relationKey string) (state.RelationNetworks, error)

	// ApplicationOfferForUUID returns the application offer for the UUID.
	ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error)

	// WatchOfferStatus returns a watcher that notifies of changes to the status
	// of the offer.
	WatchOfferStatus(offerUUID string) (state.NotifyWatcher, error)

	// WatchOffer returns a watcher that notifies of changes to the
	// lifecycle of the offer.
	WatchOffer(offerName string) state.NotifyWatcher

	// ApplyOperation applies a model operation to the state.
	ApplyOperation(op state.ModelOperation) error

	// RemoveSecretConsumer removes secret references for the specified consumer.
	RemoveSecretConsumer(consumer names.Tag) error

	// UpdateSecretConsumerOperation returns an operation for updating the latest revision
	// for any consumers of the secret.
	UpdateSecretConsumerOperation(uri *coresecrets.URI, latestRevision int) (state.ModelOperation, error)
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
	Endpoints() []state.Endpoint

	// Endpoint returns the endpoint of the relation for the named application.
	Endpoint(appName string) (state.Endpoint, error)

	// Unit returns a RelationUnit for the unit with the supplied ID.
	Unit(unitId string) (RelationUnit, error)

	// WatchUnits returns a watcher that notifies of changes to the units of the
	// specified application in the relation.
	WatchUnits(applicationName string) (state.RelationUnitsWatcher, error)

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
	Endpoints() ([]state.Endpoint, error)

	// Charm returns the application's charm and whether units should upgrade to that
	// charm even if they are in an error state.
	Charm() (ch Charm, force bool, err error)

	// CharmURL returns a string representation the application's charm URL,
	// and whether units should upgrade to the charm with that URL even if
	// they are in an error state.
	CharmURL() (curl *string, force bool)

	// EndpointBindings returns the Bindings object for this application.
	EndpointBindings() (Bindings, error)

	// Status returns the status of the application.
	Status() (status.StatusInfo, error)

	// AllUnits returns all units of the application.
	AllUnits() ([]Unit, error)
}

// Unit represents the state of a unit hosted in the local model.
type Unit interface {
	// Status returns the status of the unit.
	Status() (status.StatusInfo, error)
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
	SetStatus(info status.StatusInfo, recorder status.StatusHistoryRecorder) error

	// TerminateOperation returns an operation that will set this
	// remote application to terminated and leave it in a state
	// enabling it to be removed cleanly.
	TerminateOperation(string) state.ModelOperation

	// DestroyOperation returns a model operation to destroy remote application.
	DestroyOperation(bool) state.ModelOperation
}
