// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// hook provides types that define the hooks known to the Uniter
package hook

import (
	"fmt"

	"launchpad.net/juju-core/charm/hooks"
)

// Info holds details required to execute a hook. Not all fields are
// relevant to all Kind values.
type Info struct {
	Kind hooks.Kind

	// RelationId identifies the relation associated with the hook. It is
	// only set when Kind indicates a relation hook.
	RelationId int `yaml:"relation-id,omitempty"`

	// RemoteUnit is the name of the unit that triggered the hook. It is only
	// set when Kind inicates a relation hook other than relation-broken.
	RemoteUnit string `yaml:"remote-unit,omitempty"`

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit. It is only set when RemoteUnit is set.
	ChangeVersion int64 `yaml:"change-version,omitempty"`
}

// Validate returns an error if the info is not valid.
func (hi Info) Validate() error {
	switch hi.Kind {
	case hooks.RelationJoined, hooks.RelationChanged, hooks.RelationDeparted:
		if hi.RemoteUnit == "" {
			return fmt.Errorf("%q hook requires a remote unit", hi.Kind)
		}
		fallthrough
	case hooks.Install, hooks.Start, hooks.ConfigChanged, hooks.UpgradeCharm, hooks.Stop, hooks.RelationBroken:
		return nil
	}
	return fmt.Errorf("unknown hook kind %q", hi.Kind)
}
