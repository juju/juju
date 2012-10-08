package jujuc

import "errors"

// ErrNoRemote indicates that a Context is not associated with a remote unit.
var ErrNoRemote = errors.New("remote unit not found")

// ErrNoRelation indicates that some relation is not known to the unit.
var ErrNoRelation = errors.New("relation not found")

// Context is the interface that all hook helper commands
// depend on to interact with the rest of the system.
type Context interface {

	// Unit returns the executing unit's name.
	UnitName() string

	// PublicAddress returns the executing unit's public address.
	PublicAddress() (string, error)

	// PrivateAddress returns the executing unit's private address.
	PrivateAddress() (string, error)

	// OpenPort marks the supplied port for opening when the executing unit's
	// service is exposed.
	OpenPort(protocol string, port int) error

	// ClosePort ensures the supplied port is closed even when the executing
	// unit's service is exposed (unless it is opened separately by a co-
	// located unit).
	ClosePort(protocol string, port int) error

	// Config returns the current service configuration of the executing unit.
	Config() (map[string]interface{}, error)

	// RelationId returns the id of the relation associated with the hook.
	// It returns ErrNoRelation when not running a relation hook.
	RelationId() (int, error)

	// RemoteUnitName returns the name of the remote unit the hook
	// execution is associated with. It returns ErrNoRemote if no
	// remote unit is associated with the hook execution.
	RemoteUnitName() (string, error)

	// RelationIds returns the ids of all relations the executing unit is
	// currently participating in.
	RelationIds() []int

	// Relation returns the relation with the supplied id. It returns
	// ErrNoRelation when the relation is not known.
	Relation(id int) (ContextRelation, error)
}

// ContextRelation expresses the capabilities of a hook with respect to a relation.
type ContextRelation interface {

	// Name returns the name the locally executing charm assigned to this relation.
	Name() string

	// FakeId returns a string of the form "relation-name:123", which uniquely
	// identifies the relation to the hook. In reality, the identification
	// of the relation is the integer following the colon, but the composed
	// name is useful to humans observing it.
	FakeId() string

	// Settings allows read/write access to the local unit's settings in
	// this relation.
	Settings() (Settings, error)

	// UnitNames returns a list of the remote units in the relation.
	UnitNames() []string

	// ReadSettings returns the settings of any remote unit in the relation.
	ReadSettings(unit string) (map[string]interface{}, error)
}

// Settings is implemented by types that manipulate unit settings.
type Settings interface {
	Map() map[string]interface{}
	Get(string) (interface{}, bool)
	Set(string, interface{})
	Delete(string)
}

// envRelation returns the relation name exposed to hooks as JUJU_RELATION.
// If the context does not have a relation, it will return an empty string.
func envRelation(ctx Context) string {
	id, err := ctx.RelationId()
	if err != nil {
		if err != ErrNoRelation {
			panic(err)
		}
		return ""
	}
	r, err := ctx.Relation(id)
	if err != nil {
		panic(err)
	}
	return r.Name()
}

// envRelationId returns the relation id exposed to hooks as JUJU_RELATION_ID.
// If the context does not have a relation, it will return an empty string.
// Otherwise, it will panic if RelationId is not a key in the Relations map.
func (ctx *HookContext) envRelationId() string {
	if ctx.RelationId_ == -1 {
		return ""
	}
	return ctx.Relations[ctx.RelationId_].FakeId()
}
