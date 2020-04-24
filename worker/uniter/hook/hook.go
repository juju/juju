// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package hook provides types that define the hooks known to the Uniter
package hook

import (
	"fmt"

	"github.com/juju/charm/v7/hooks"
	"github.com/juju/names/v4"
)

// TODO(fwereade): move these definitions to juju/charm/hooks.
const (
	LeaderElected         hooks.Kind = "leader-elected"
	LeaderDeposed         hooks.Kind = "leader-deposed"
	LeaderSettingsChanged hooks.Kind = "leader-settings-changed"
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

	// RemoteApplication is always set if either an app or a unit triggers the hook.
	// If the app triggers the hook, then RemoteUnit will be empty
	RemoteApplication string `yaml:"remote-application,omitempty"`

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit. It is only set when RemoteUnit is set.
	ChangeVersion int64 `yaml:"change-version,omitempty"`

	// StorageId is the ID of the storage instance relevant to the hook.
	StorageId string `yaml:"storage-id,omitempty"`

	// DepartingUnit is the name of the unit that goes away. It is only set
	// when Kind indicates a relation-departed hook.
	DepartingUnit string `yaml:"departee,omitempty"`
}

// Validate returns an error if the info is not valid.
func (hi Info) Validate() error {
	switch hi.Kind {
	case hooks.RelationChanged:
		if hi.RemoteUnit == "" {
			if hi.RemoteApplication == "" {
				return fmt.Errorf("%q hook requires a remote unit or application", hi.Kind)
			}
		} else if hi.RemoteApplication == "" {
			return fmt.Errorf("%q hook has a remote unit but no application", hi.Kind)
		}
		return nil
	case hooks.RelationJoined, hooks.RelationDeparted:
		if hi.RemoteUnit == "" {
			return fmt.Errorf("%q hook requires a remote unit", hi.Kind)
		}
		if hi.RemoteApplication == "" {
			return fmt.Errorf("%q hook has a remote unit but no application", hi.Kind)
		}
		return nil
	case hooks.Install, hooks.Remove, hooks.Start, hooks.ConfigChanged, hooks.UpgradeCharm, hooks.Stop, hooks.RelationCreated, hooks.RelationBroken,
		hooks.CollectMetrics, hooks.MeterStatusChanged, hooks.UpdateStatus, hooks.PreSeriesUpgrade, hooks.PostSeriesUpgrade:
		return nil
	case hooks.Action:
		return fmt.Errorf("hooks.Kind Action is deprecated")
	case hooks.StorageAttached, hooks.StorageDetaching:
		if !names.IsValidStorage(hi.StorageId) {
			return fmt.Errorf("invalid storage ID %q", hi.StorageId)
		}
		return nil
	// TODO(fwereade): define these in charm/hooks...
	case LeaderElected, LeaderDeposed, LeaderSettingsChanged:
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
