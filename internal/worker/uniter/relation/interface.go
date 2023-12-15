// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

type RelationStateTracker interface {
	// PrepareHook returns the name of the supplied relation hook, or an error
	// if the hook is unknown or invalid given current state.
	PrepareHook(hook.Info) (string, error)

	// CommitHook persists the state change encoded in the supplied relation
	// hook, or returns an error if the hook is unknown or invalid given
	// current relation state.
	CommitHook(hook.Info) error

	// SynchronizeScopes ensures that the locally tracked relation scopes
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

	// Report provides information for the engine report.
	Report() map[string]interface{}
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
	RemoveRelation(id int, unitGetter UnitGetter, knownUnits map[string]bool) error
}

// UnitGetter encapsulates methods to get unit info.
type UnitGetter interface {
	// Unit returns the existing unit with the given tag.
	Unit(tag names.UnitTag) (api.Unit, error)
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

// StateTrackerClient encapsulates the uniter client API methods
// required by a relationStateTracker.
type StateTrackerClient interface {
	// Unit returns the existing unit with the given tag.
	Unit(tag names.UnitTag) (api.Unit, error)

	// Relation returns the existing relation with the given tag.
	Relation(tag names.RelationTag) (api.Relation, error)

	// RelationById returns the existing relation with the given id.
	RelationById(int) (api.Relation, error)
}

// Application encapsulates the methods from
// state.Application required by a relationStateTracker.
type Application interface {
	Life() life.Value
}

// Relationer encapsulates the methods from relationer required by a stateTracker.
type Relationer interface {
	// CommitHook persists the fact of the supplied hook's completion.
	CommitHook(hi hook.Info) error

	// ContextInfo returns a representation of the relationer's current state.
	ContextInfo() *context.RelationInfo

	// IsDying returns whether the relation is dying.
	IsDying() bool

	// IsImplicit returns whether the local relation endpoint is implicit.
	IsImplicit() bool

	// Join initializes local state and causes the unit to enter its relation
	// scope, allowing its counterpart units to detect its presence and settings
	// changes.
	Join() error

	// PrepareHook checks that the relation is in a state such that it makes
	// sense to execute the supplied hook, and ensures that the relation context
	// contains the latest relation state as communicated in the hook.Info.
	PrepareHook(hi hook.Info) (string, error)

	// RelationUnit returns the relation unit associated with this relationer instance.
	RelationUnit() api.RelationUnit

	// SetDying informs the relationer that the unit is departing the relation,
	// and that the only hooks it should send henceforth are -departed hooks,
	// until the relation is empty, followed by a -broken hook.
	SetDying() error
}
