// hooks provides types and constants that define the hooks known to Juju.
package hooks

// Kind enumerates the different kinds of hooks that exist.
type Kind string

const (
	// None of these hooks are ever associated with a relation; each of them
	// represents a change to the state of the unit as a whole. The values
	// themselves are all valid hook names.
	Install       Kind = "install"
	Start         Kind = "start"
	ConfigChanged Kind = "config-changed"
	UpgradeCharm  Kind = "upgrade-charm"
	Stop          Kind = "stop"

	// These hooks require an associated relation, and the name of the relation
	// unit whose change triggered the hook. The hook file names that these
	// kinds represent will be prefixed by the relation name; for example,
	// "db-relation-joined".
	RelationJoined   Kind = "relation-joined"
	RelationChanged  Kind = "relation-changed"
	RelationDeparted Kind = "relation-departed"

	// This hook requires an associated relation. The represented hook file name
	// will be prefixed by the relation name, just like the other Relation* Kind
	// values.
	RelationBroken Kind = "relation-broken"
)

// UnitHooks contains all known unit hook kinds.
var UnitHooks = []Kind{
	Install,
	Start,
	ConfigChanged,
	UpgradeCharm,
	Stop,
}

// RelationBroken contains all known relation hook kinds.
var RelationHooks = []Kind{
	RelationJoined,
	RelationChanged,
	RelationDeparted,
	RelationBroken,
}

// IsRelation will return true if the Kind represents a relation hook.
func (kind Kind) IsRelation() bool {
	switch kind {
	case RelationJoined, RelationChanged, RelationDeparted, RelationBroken:
		return true
	}
	return false
}

// IsUnit will return true if the Kind does represents an unit hook.
func (kind Kind) IsUnit() bool {
	switch kind {
	case Install, Start, ConfigChanged, UpgradeCharm, Stop:
		return true
	}
	return false
}
