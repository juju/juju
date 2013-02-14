// hook provides types that define the hooks known to the Uniter
package hook

import (
	"fmt"
	chook "launchpad.net/juju-core/charm/hook"
)

// Info holds details required to execute a hook. Not all fields are
// relevant to all Kind values.
type Info struct {
	Kind chook.Kind

	// RelationId identifies the relation associated with the hook. It is
	// only set when Kind indicates a relation hook.
	RelationId int `yaml:"relation-id,omitempty"`

	// RemoteUnit is the name of the unit that triggered the hook. It is only
	// set when Kind inicates a relation hook other than relation-broken.
	RemoteUnit string `yaml:"remote-unit,omitempty"`

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit. It is only set when RemoteUnit is set.
	ChangeVersion int64 `yaml:"change-version,omitempty"`

	// Members may contain settings for units that are members of the relation,
	// keyed on unit name. If a unit is present in members, it is always a
	// member of the relation; if a unit is not present, no inferences about
	// its state can be drawn.
	Members map[string]map[string]interface{} `yaml:"members,omitempty"`
}

// Validate returns an error if the info is not valid.
func (hi Info) Validate() error {
	switch hi.Kind {
	case chook.RelationJoined, chook.RelationChanged, chook.RelationDeparted:
		if hi.RemoteUnit == "" {
			return fmt.Errorf("%q hook requires a remote unit", hi.Kind)
		}
		fallthrough
	case chook.Install, chook.Start, chook.ConfigChanged, chook.UpgradeCharm, chook.Stop, chook.RelationBroken:
		return nil
	}
	return fmt.Errorf("unknown hook kind %q", hi.Kind)
}
