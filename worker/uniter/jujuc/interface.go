package jujuc

import "errors"

// Context expresses the capabilities of a hook.
type Context interface {

	// Unit returns the executing unit's name.
	Unit() string

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
	// If the hook is not a relation hook, it returns -1.
	RelationId() int

	// CounterpartUnit returns the name of the counterpart unit associated with
	// the hook. If the hook is not a relation hook, or if no counterpart unit
	// is associated, it returns "".
	CounterpartUnit() string

	// RelationIds returns the ids of all relations the executing unit is
	// currently participating in.
	RelationIds() []int

	// Relation returns the relation with the supplied id. If the relation is
	// not known, it returns ErrRelationNotFound.
	Relation(id int) (Relation, error)
}

// ErrRelationNotFound indicates that some relation is not known to the unit.
var ErrRelationNotFound = errors.New("relation not found")

// Relation expresses the capabilities of a hook with respect to a relation.
type Relation interface {

	// Name returns a string of the form "relation-name", which identifies
	// the kind of relation only.
	Name() string

	// FakeId returns a string of the form "relation-name:123", which uniquely
	// identifies the relation to the hook.
	FakeId() string

	// Units returns a list of the counterpart units in the relation.
	Units() []string

	// Settings allows read/write access to the unit's settings in this relation.
	Settings() (MapChanger, error)

	// ReadSettings returns the settings of any counterpart unit in the
	// relation.
	ReadSettings(unit string) (map[string]interface{}, error)
}

// MapChanger exposes map-like functionality.
type MapChanger interface {
	Map() map[string]interface{}
	Get(string) (interface{}, bool)
	Set(string, interface{})
	Delete(string)
}
