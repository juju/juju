// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// hook provides types that define the hooks known to the operator
package hook

import (
	"fmt"

	"gopkg.in/juju/charm.v6/hooks"
)

// Info holds details required to execute a hook. Not all fields are
// relevant to all Kind values.
type Info struct {
	Kind hooks.Kind `yaml:"kind"`

	// RelationId identifies the relation associated with the hook. It is
	// only set when Kind indicates a relation hook.
	RelationId int `yaml:"relation-id,omitempty"`

	// RemoteUnit is the name of the unit that triggered the hook. It is only
	// set when Kind indicates a relation hook other than relation-broken.
	RemoteUnit string `yaml:"remote-unit,omitempty"`

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit. It is only set when RemoteUnit is set.
	ChangeVersion int64 `yaml:"change-version,omitempty"`
}

// Validate returns an error if the info is not valid.
func (hi Info) Validate() error {
	switch hi.Kind {
	case hooks.RelationChanged:
		if hi.RemoteUnit == "" {
			return fmt.Errorf("%q hook requires a remote unit", hi.Kind)
		}
		fallthrough
	case hooks.ConfigChanged:
		return nil
	}
	return fmt.Errorf("unknown hook kind %q", hi.Kind)
}

// Committer is an interface that may be used to convey the fact that the
// specified hook has been successfully executed, and committed.
type Committer interface {
	CommitHook(Info) error
}

// Validator is an interface that may be used to validate a hook execution
// request prior to executing it.
type Validator interface {
	ValidateHook(Info) error
}
