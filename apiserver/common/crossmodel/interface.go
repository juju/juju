// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

type Backend interface {
	// ModelUUID returns the model UUID for the model
	// controlled by this state instance.
	ModelUUID() string

	// ModelTag the tag of the model on which we are operating.
	ModelTag() names.ModelTag

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

	// AddRemoteApplication creates a new remote application record, having the supplied relation endpoints,
	// with the supplied name (which must be unique across all applications, local and remote).
	AddRemoteApplication(state.AddRemoteApplicationParams) (RemoteApplication, error)

	// GetRemoteEntity returns the tag of the entity associated with the given token.
	GetRemoteEntity(string) (names.Tag, error)

	// ExportLocalEntity adds an entity to the remote entities collection,
	// returning an opaque token that uniquely identifies the entity within
	// the model.
	ExportLocalEntity(names.Tag) (string, error)

	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(entity names.Tag, token string) error

	// SaveIngressNetworks stores in state the ingress networks for the relation.
	SaveIngressNetworks(relationKey string, cidrs []string) (state.RelationNetworks, error)

	// Networks returns the networks for the specified relation.
	IngressNetworks(relationKey string) (state.RelationNetworks, error)

	// ApplicationOfferForUUID returns the application offer for the UUID.
	ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error)

	// WatchStatus returns a watcher that notifies of changes to the status
	// of the offer.
	WatchOfferStatus(offerUUID string) (state.NotifyWatcher, error)

	// FirewallRule returns the firewall rule for the specified service.
	FirewallRule(service state.WellKnownServiceType) (*state.FirewallRule, error)
}

// Relation provides access a relation in global state.
type Relation interface {
	status.StatusGetter
	status.StatusSetter
	// Destroy ensures that the relation will be removed at some point; if
	// no units are currently in scope, it will be removed immediately.
	Destroy() error

	// Id returns the integer internal relation key.
	Id() int

	// Life returns the relation's current life state.
	Life() state.Life

	// Tag returns the relation's tag.
	Tag() names.Tag

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

	// CharmURL returns the application's charm URL, and whether units should upgrade
	// to the charm with that URL even if they are in an error state.
	CharmURL() (curl *charm.URL, force bool)

	// EndpointBindings returns the mapping for each endpoint name and the space
	// name it is bound to (or empty if unspecified). When no bindings are stored
	// for the application, defaults are returned.
	EndpointBindings() (map[string]string, error)

	// Status returns the status of the application.
	Status() (status.StatusInfo, error)
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
	// Destroy ensures that this remote application reference and all its relations
	// will be removed at some point; if no relation involving the
	// application has any units in scope, they are all removed immediately.
	Destroy() error

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

	// Life returns the lifecycle state of the application.
	Life() state.Life

	// SetStatus sets the status of the remote application.
	SetStatus(info status.StatusInfo) error
}
