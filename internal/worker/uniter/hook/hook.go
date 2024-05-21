// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hook

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/secrets"
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

	// NoticeID is the Pebble notice ID associated with the hook.
	NoticeID string `yaml:"notice-id,omitempty"`

	// NoticeType is the Pebble notice type associated with the hook.
	NoticeType string `yaml:"notice-type,omitempty"`

	// NoticeKey is the Pebble notice key associated with the hook.
	NoticeKey string `yaml:"notice-key,omitempty"`

	// MachineUpgradeTarget is the base that the unit's machine is to be
	// updated to when Juju is issued the `upgrade-machine` command.
	// It is only set for the pre-series-upgrade hook.
	MachineUpgradeTarget string `yaml:"series-upgrade-target,omitempty"`

	// SecretURI is the secret URI relevant to the hook.
	SecretURI string `yaml:"secret-uri,omitempty"`

	// SecretRevision is the secret revision relevant to the hook.
	SecretRevision int `yaml:"secret-revision,omitempty"`

	// SecretLabel is the secret label to expose to the hook.
	SecretLabel string `yaml:"secret-label,omitempty"`
}

// SecretHookRequiresRevision returns true if the hook context needs a secret revision.
func SecretHookRequiresRevision(kind hooks.Kind) bool {
	return kind == hooks.SecretRemove || kind == hooks.SecretExpired
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
	case hooks.PebbleCustomNotice:
		if hi.WorkloadName == "" {
			return errors.Errorf("%q hook requires a workload name", hi.Kind)
		}
		if hi.NoticeID == "" || hi.NoticeType == "" || hi.NoticeKey == "" {
			return errors.Errorf("%q hook requires a notice ID, type, and key", hi.Kind)
		}
		return nil
	case hooks.PebbleReady:
		if hi.WorkloadName == "" {
			return errors.Errorf("%q hook requires a workload name", hi.Kind)
		}
		return nil
	case hooks.PreSeriesUpgrade:
		if hi.MachineUpgradeTarget == "" {
			return errors.Errorf("%q hook requires a target base", hi.Kind)
		}
		return nil
	case hooks.Install, hooks.Remove, hooks.Start, hooks.ConfigChanged, hooks.UpgradeCharm, hooks.Stop,
		hooks.RelationCreated, hooks.RelationBroken, hooks.UpdateStatus,
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
	case hooks.SecretRotate, hooks.SecretChanged, hooks.SecretExpired, hooks.SecretRemove:
		if hi.SecretURI == "" {
			return errors.Errorf("%q hook requires a secret URI", hi.Kind)
		}
		if _, err := secrets.ParseURI(hi.SecretURI); err != nil {
			return errors.Errorf("invalid secret URI %q", hi.SecretURI)
		}
		if SecretHookRequiresRevision(hi.Kind) && hi.SecretRevision <= 0 {
			return errors.Errorf("%q hook requires a secret revision", hi.Kind)
		}
		return nil
	}
	return errors.Errorf("unknown hook kind %q", hi.Kind)
}

// Committer is an interface that may be used to convey the fact that the
// specified hook has been successfully executed, and committed.
type Committer interface {
	CommitHook(context.Context, Info) error
}

// Validator is an interface that may be used to validate a hook execution
// request prior to executing it.
type Validator interface {
	ValidateHook(Info) error
}
