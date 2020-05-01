// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner/context"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mock_statetracker.go github.com/juju/juju/worker/uniter/relation RelationStateTracker
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mock_subordinate_destroyer.go github.com/juju/juju/worker/uniter/relation SubordinateDestroyer
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mock_state_tracker_state.go github.com/juju/juju/worker/uniter/relation StateTrackerState
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mock_uniter_api.go github.com/juju/juju/worker/uniter/relation Unit,Application,Relation,RelationUnit

type RelationStateTracker interface {
	// PrepareHook returns the name of the supplied relation hook, or an error
	// if the hook is unknown or invalid given current state.
	PrepareHook(hook.Info) (string, error)

	// CommitHook persists the state change encoded in the supplied relation
	// hook, or returns an error if the hook is unknown or invalid given
	// current relation state.
	CommitHook(hook.Info) error

	// SyncronizeScopes ensures that the locally tracked relation scopes
	// reflect the contents of the remote state snapshot by entering or
	// exiting scopes as required.
	SynchronizeScopes(remotestate.Snapshot) error

	// IsKnown returns true if the relation ID is known by the tracker.
	IsKnown(int) bool

	// IsImplicit returns true if the endpoint for a relation ID is implicit.
	IsImplicit(int) (bool, error)

	// IsPeerRelation returns true if the endpoint for a relation ID has a
	// Peer role.
	IsPeerRelation(int) (bool, error)

	// HasContainerScope returns true if the specified relation ID has a
	// container scope.
	HasContainerScope(int) (bool, error)

	// RelationCreated returns true if a relation created hook has been
	// fired for the specified relation ID.
	RelationCreated(int) bool

	// RemoteApplication returns the remote application name associated
	// with the specified relation ID.
	RemoteApplication(int) string

	// State returns a State instance for accessing the local state
	// for a relation ID.
	State(int) (*State, error)

	// StateFound returns a true if there is a state for the given in
	// in the state manager.
	StateFound(int) bool

	// GetInfo returns information about current relation state.
	GetInfo() map[int]*context.RelationInfo

	// Name returns the name of the relation with the supplied id, or an error
	// if the relation is unknown.
	Name(id int) (string, error)

	// LocalUnitName returns the name for the local unit.
	LocalUnitName() string

	// LocalUnitAndApplicationLife returns the life values for the local
	// unit and application.
	LocalUnitAndApplicationLife() (life.Value, life.Value, error)
}

// SubordinateDestroyer destroys all subordinates of a unit.
type SubordinateDestroyer interface {
	DestroyAllSubordinates() error
}

// StateManager encapsulates methods required to handle relation
// state.
type StateManager interface {
	// KnownIDs returns a slice of relation ids, known to the
	// state manager.
	KnownIDs() []int

	// Relation returns a copy of the relation state for the given id.
	Relation(int) (*State, error)

	// SetRelation persists the given state, overwriting the previous
	// state for a given id or creating state at a new id.
	SetRelation(*State) error

	// RelationFound returns true if the state manager has a
	// state for the given id.
	RelationFound(id int) bool

	// RemoveRelation removes the state for the given id from the
	// manager.
	RemoveRelation(id int) error
}

// UnitStateReadWriter encapsulates the methods from a state.Unit
// required to set and get unit state.
type UnitStateReadWriter interface {
	// SetState sets the state persisted by the charm running in this unit
	// and the state internal to the uniter for this unit.
	SetState(unitState params.SetUnitStateArg) error

	// State returns the state persisted by the charm running in this unit
	// and the state internal to the uniter for this unit.
	State() (params.UnitStateResult, error)
}

// StateTrackerState encapsulates the methods from state
// required by a relationStateTracker.
type StateTrackerState interface {
	// Relation returns the existing relation with the given tag.
	Relation(tag names.RelationTag) (Relation, error)

	// RelationById returns the existing relation with the given id.
	RelationById(int) (Relation, error)
}

// Unit encapsulates the methods from state.Unit
// required by a relationStateTracker.
type Unit interface {
	UnitStateReadWriter

	// Tag returns the tag for this unit.
	Tag() names.UnitTag

	// ApplicationTag returns the tag for this unit's application.
	ApplicationTag() names.ApplicationTag

	// RelationsStatus returns the tags of the relations the unit has joined
	// and entered scope, or the relation is suspended.
	RelationsStatus() ([]uniter.RelationStatus, error)

	// Watch returns a watcher for observing changes to the unit.
	Watch() (watcher.NotifyWatcher, error)

	// Destroy, when called on a Alive unit, advances its lifecycle as far as
	// possible; it otherwise has no effect. In most situations, the unit's
	// life is just set to Dying; but if a principal unit that is not assigned
	// to a provisioned machine is Destroyed, it will be removed from state
	// directly.
	Destroy() error

	// Name returns the name of the unit.
	Name() string

	// Refresh updates the cached local copy of the unit's data.
	Refresh() error

	// Application returns the unit's application.
	Application() (Application, error)

	// Life returns the unit's lifecycle value.
	Life() life.Value
}

// Application encapsulates the methods from
// state.Application required by a relationStateTracker.
type Application interface {
	Life() life.Value
}

// Relation encapsulates the methods from
// state.Relation required by a relationStateTracker.
type Relation interface {
	// Endpoint returns the endpoint of the relation for the application the
	// uniter's managed unit belongs to.
	Endpoint() (*uniter.Endpoint, error)

	// Id returns the integer internal relation key. This is exposed
	// because the unit agent needs to expose a value derived from this
	// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
	// between relations with different applications.
	Id() int

	// Life returns the relation's current life state.
	Life() life.Value

	// OtherApplication returns the name of the application on the other
	// end of the relation (from this unit's perspective).
	OtherApplication() string

	// Refresh refreshes the contents of the relation from the underlying
	// state. It returns an error that satisfies errors.IsNotFound if the
	// relation has been removed.
	Refresh() error

	// SetStatus updates the status of the relation.
	SetStatus(relation.Status) error

	// String returns the relation as a string.
	String() string

	// Suspended returns the relation's current suspended status.
	Suspended() bool

	// Tag returns the relation tag.
	Tag() names.RelationTag

	// Unit returns a uniter.RelationUnit for the supplied unit.
	Unit(names.UnitTag) (RelationUnit, error)

	// UpdateSuspended updates the in memory value of the
	// relation's suspended attribute.
	UpdateSuspended(bool)
}

// RelationUnit encapsulates the methods from
// state.RelationUnit required by a relationer.
type RelationUnit interface {
	// ApplicationSettings returns a Settings which allows access to this
	// unit's application settings within the relation. This can only be
	// used from the leader unit.
	ApplicationSettings() (*uniter.Settings, error)

	// Endpoint returns the endpoint of the relation for the application the
	// uniter's managed unit belongs to.
	Endpoint() uniter.Endpoint

	// EnterScope ensures that the unit has entered its scope in the relation.
	// When the unit has already entered its relation scope, EnterScope will
	// report success but make no changes to state.
	EnterScope() error

	// LeaveScope signals that the unit has left its scope in the relation.
	// After the unit has left its relation scope, it is no longer a member
	// of the relation.
	LeaveScope() error

	// Relation returns the relation associated with the unit.
	Relation() Relation

	// ReadSettings returns a map holding the settings of the unit with the
	// supplied name within this relation.
	ReadSettings(name string) (params.Settings, error)

	// Settings returns a Settings which allows access to the unit's settings
	// within the relation.
	Settings() (*uniter.Settings, error)

	// UpdateRelationSettings is used to record any changes to settings for
	// this unit and application. It is only valid to update application
	// settings if this unit is the leader.
	UpdateRelationSettings(unit, application params.Settings) error
}
