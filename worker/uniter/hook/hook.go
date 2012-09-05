// hook provides types and constants that define the hooks known to Juju.
package hook

import "fmt"

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

// IsRelation will return true if the Kind represents a relation hook.
func (kind Kind) IsRelation() bool {
	switch kind {
	case RelationJoined, RelationChanged, RelationDeparted, RelationBroken:
		return true
	}
	return false
}

// Info holds details required to execute a hook. Not all fields are
// relevant to all Kind values.
type Info struct {
	Kind Kind

	// RelationId identifies the relation associated with the hook. It is
	// only set when Kind indicates a relation hook.
	RelationId int `yaml:"relation-id,omitempty"`

	// RemoteUnit is the name of the unit that triggered the hook. It is only
	// set when Kind inicates a relation hook other than relation-broken.
	RemoteUnit string `yaml:"remote-unit,omitempty"`

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit. It is only set when RemoteUnit is set.
	ChangeVersion int `yaml:"change-version,omitempty"`

	// Members may contain member unit relation settings, keyed on unit name.
	// If a unit is present in members, it is always a member of the relation;
	// if a unit is not present, no inferences about its state can be drawn.
	Members map[string]map[string]interface{} `yaml:"members,omitempty"`
}

// Validate returns an error if the info is not valid.
func (hi Info) Validate() error {
	switch hi.Kind {
	case RelationJoined, RelationChanged, RelationDeparted:
		if hi.RemoteUnit == "" {
			return fmt.Errorf("%q hook requires a remote unit", hi.Kind)
		}
		fallthrough
	case Install, Start, ConfigChanged, UpgradeCharm, Stop, RelationBroken:
		return nil
	}
	return fmt.Errorf("unknown hook kind %q", hi.Kind)
}
