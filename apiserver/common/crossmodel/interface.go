// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type Backend interface {
	// ModelUUID returns the model UUID for the model
	// controlled by this state instance.
	ModelUUID() string

	// KeyRelation returns the existing relation with the given key (which can
	// be derived unambiguously from the relation's endpoints).
	KeyRelation(string) (Relation, error)

	// Application returns a local application by name.
	Application(string) (Application, error)

	// RemoteApplication returns a remote application by name.
	RemoteApplication(string) (RemoteApplication, error)

	// AddRelation adds a relation between the specified endpoints and returns the relation info.
	AddRelation(...state.Endpoint) (Relation, error)

	// EndpointsRelation returns the existing relation with the given endpoints.
	EndpointsRelation(...state.Endpoint) (Relation, error)

	// AddRemoteApplication creates a new remote application record, having the supplied relation endpoints,
	// with the supplied name (which must be unique across all applications, local and remote).
	AddRemoteApplication(state.AddRemoteApplicationParams) (RemoteApplication, error)

	// GetRemoteEntity returns the tag of the entity associated with the given
	// token and model.
	GetRemoteEntity(names.ModelTag, string) (names.Tag, error)

	// ExportLocalEntity adds an entity to the remote entities collection,
	// returning an opaque token that uniquely identifies the entity within
	// the model.
	ExportLocalEntity(names.Tag) (string, error)

	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(sourceModel names.ModelTag, entity names.Tag, token string) error
}

// Relation provides access a relation in global state.
type Relation interface {
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

	// Endpoints returns the endpoints that constitute the relation.
	Endpoints() []state.Endpoint

	// Unit returns a RelationUnit for the unit with the supplied ID.
	Unit(unitId string) (RelationUnit, error)

	// WatchUnits returns a watcher that notifies of changes to the units of the
	// specified application in the relation.
	WatchUnits(applicationName string) (state.RelationUnitsWatcher, error)
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

	// URL returns the remote application URL, at which it is offered.
	URL() (string, bool)

	// OfferName returns the name the offering side has given to the remote application..
	OfferName() string

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
}
