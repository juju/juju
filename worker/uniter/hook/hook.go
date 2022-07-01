// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package hook provides types that define the hooks known to the Uniter
package hook

import (
	"github.com/juju/charm/v9/hooks"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v3/core/secrets"
)

// Info holds details required to execute a hook. Not all fields are
// relevant to all Kind values.
type Info struct {
	Kind hooks.Kind `yaml:"kind"`

	// RelationId identifies the relation associated with the hook.
	// It is only set when Kind indicates a relation hook.
	RelationId int `yaml:"relation-id,omitempty"`

	// RemoteUnit is the name of the unit that triggered the hook.
	// It is only set when Kind indicates a relation hook other than
	// relation-created or relation-broken.
	RemoteUnit string `yaml:"remote-unit,omitempty"`

	// RemoteApplication is always set if either an app or a unit triggers
	// the hook. If the app triggers the hook, then RemoteUnit will be empty.
	RemoteApplication string `yaml:"remote-application,omitempty"`

	// ChangeVersion identifies the most recent unit settings change
	// associated with RemoteUnit. It is only set when RemoteUnit is set.
	ChangeVersion int64 `yaml:"change-version,omitempty"`

	// StorageId is the ID of the storage instance relevant to the hook.
	StorageId string `yaml:"storage-id,omitempty"`

	// DepartingUnit is the name of the unit that goes away. It is only set
	// when Kind indicates a relation-departed hook.
	DepartingUnit string `yaml:"departee,omitempty"`

	// WorkloadName is the name of the sidecar container or workload relevant to the hook.
	WorkloadName string `yaml:"workload-name,omitempty"`

	// SeriesUpgradeTarget is the series that the unit's machine is to be
	// updated to when Juju is issued the `upgrade-series` command.
	// It is only set for the pre-series-upgrade hook.
	SeriesUpgradeTarget string `yaml:"series-upgrade-target,omitempty"`

	// SecretURL is the secret URL relevant to the hook.
	SecretURL string `yaml:"secret-url,omitempty"`
}

// Validate returns an error if the info is not valid.
func (hi Info) Validate() error {
	switch hi.Kind {
	case hooks.RelationChanged:
		if hi.RemoteUnit == "" {
			if hi.RemoteApplication == "" {
				return errors.Errorf("%q hook requires a remote unit or application", hi.Kind)
			}
		} else if hi.RemoteApplication == "" {
			return errors.Errorf("%q hook has a remote unit but no application", hi.Kind)
		}
		return nil
	case hooks.RelationJoined, hooks.RelationDeparted:
		if hi.RemoteUnit == "" {
			return errors.Errorf("%q hook requires a remote unit", hi.Kind)
		}
		if hi.RemoteApplication == "" {
			return errors.Errorf("%q hook has a remote unit but no application", hi.Kind)
		}
		return nil
	case hooks.PebbleReady:
		if hi.WorkloadName == "" {
			return errors.Errorf("%q hook requires a workload name", hi.Kind)
		}
		return nil
	case hooks.PreSeriesUpgrade:
		if hi.SeriesUpgradeTarget == "" {
			return errors.Errorf("%q hook requires a target series", hi.Kind)
		}
		return nil
	case hooks.Install, hooks.Remove, hooks.Start, hooks.ConfigChanged, hooks.UpgradeCharm, hooks.Stop,
		hooks.RelationCreated, hooks.RelationBroken, hooks.CollectMetrics, hooks.MeterStatusChanged, hooks.UpdateStatus,
		hooks.PostSeriesUpgrade:
		return nil
	case hooks.Action:
		return errors.Errorf("hooks.Kind Action is deprecated")
	case hooks.StorageAttached, hooks.StorageDetaching:
		if !names.IsValidStorage(hi.StorageId) {
			return errors.Errorf("invalid storage ID %q", hi.StorageId)
		}
		return nil
	case hooks.LeaderElected, hooks.LeaderDeposed, hooks.LeaderSettingsChanged:
		return nil
	case hooks.SecretRotate:
		if hi.SecretURL == "" {
			return errors.Errorf("%q hook requires a secret URL", hi.Kind)
		}
		if _, err := secrets.ParseURL(hi.SecretURL); err != nil {
			return errors.Errorf("invalid secret URL %q", hi.SecretURL)
		}
		return nil
	}
	return errors.Errorf("unknown hook kind %q", hi.Kind)
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
