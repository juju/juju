package jujuc

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

	// HasHookRelation returns whether the executing hook has an associated
	// relation.
	HasHookRelation() bool

	// HookRelation returns the ContextRelation associated with the executing
	// hook. It panics if no relation is associated.
	HookRelation() ContextRelation

	// HasRemoteUnit returns whether the executing hook has an associated
	// remote unit.
	HasRemoteUnit() bool

	// RemoteUnitName returns the name of the remote unit the hook execution
	// is associated with. It panics if there is no remote unit associated
	// with the hook execution.
	RemoteUnitName() string

	// HasRelation returns whether the executing unit is participating in
	// the relation with the supplied id.
	HasRelation(id int) bool

	// Relation returns the relation with the supplied id. It panics if the
	// executing unit is not participating in a relation with that id.
	Relation(id int) ContextRelation

	// RelationIds returns the ids of all relations the executing unit is
	// currently participating in.
	RelationIds() []int
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
