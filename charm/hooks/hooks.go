// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package hooks provides types and constants that define the hooks known to Juju.
package hooks

// Kind enumerates the different kinds of hooks that exist.
type Kind string

const (
	// None of these hooks are ever associated with a relation; each of them
	// represents a change to the state of the unit as a whole. The values
	// themselves are all valid hook names.

	Install               Kind = "install"
	Start                 Kind = "start"
	ConfigChanged         Kind = "config-changed"
	UpgradeCharm          Kind = "upgrade-charm"
	Stop                  Kind = "stop"
	Remove                Kind = "remove"
	Action                Kind = "action"
	LeaderElected         Kind = "leader-elected"
	LeaderDeposed         Kind = "leader-deposed"
	LeaderSettingsChanged Kind = "leader-settings-changed"
	UpdateStatus          Kind = "update-status"
	PreSeriesUpgrade      Kind = "pre-series-upgrade"
	PostSeriesUpgrade     Kind = "post-series-upgrade"
	SecretChanged         Kind = "secret-changed"
	SecretExpired         Kind = "secret-expired"
	SecretRemove          Kind = "secret-remove"
	SecretRotate          Kind = "secret-rotate"

	// These hooks require an associated relation, and the name of the relation
	// unit whose change triggered the hook. The hook file names that these
	// kinds represent will be prefixed by the relation name; for example,
	// "db-relation-joined".

	RelationCreated  Kind = "relation-created"
	RelationJoined   Kind = "relation-joined"
	RelationChanged  Kind = "relation-changed"
	RelationDeparted Kind = "relation-departed"

	// This hook requires an associated relation. The represented hook file name
	// will be prefixed by the relation name, just like the other Relation* Kind
	// values.

	RelationBroken Kind = "relation-broken"

	// These hooks require an associated storage. The hook file names that these
	// kinds represent will be prefixed by the storage name; for example,
	// "shared-fs-storage-attached".

	StorageAttached  Kind = "storage-attached"
	StorageDetaching Kind = "storage-detaching"

	// These hooks require an associated workload/container, and the name of the workload/container
	// whose change triggered the hook. The hook file names that these
	// kinds represent will be prefixed by the workload/container name; for example,
	// "mycontainer-pebble-ready".

	PebbleChangeUpdated Kind = "pebble-change-updated"
	PebbleCustomNotice  Kind = "pebble-custom-notice"
	PebbleReady         Kind = "pebble-ready"
)

var unitHooks = []Kind{
	Install,
	Start,
	ConfigChanged,
	UpgradeCharm,
	Stop,
	Remove,
	LeaderElected,
	LeaderDeposed,
	LeaderSettingsChanged,
	UpdateStatus,
	PreSeriesUpgrade,
	PostSeriesUpgrade,
}

// UnitHooks returns all known unit hook kinds.
func UnitHooks() []Kind {
	hooks := make([]Kind, len(unitHooks))
	copy(hooks, unitHooks)
	return hooks
}

var relationHooks = []Kind{
	RelationCreated,
	RelationJoined,
	RelationChanged,
	RelationDeparted,
	RelationBroken,
}

// RelationHooks returns all known relation hook kinds.
func RelationHooks() []Kind {
	hooks := make([]Kind, len(relationHooks))
	copy(hooks, relationHooks)
	return hooks
}

var storageHooks = []Kind{
	StorageAttached,
	StorageDetaching,
}

// StorageHooks returns all known storage hook kinds.
func StorageHooks() []Kind {
	hooks := make([]Kind, len(storageHooks))
	copy(hooks, storageHooks)
	return hooks
}

var workloadHooks = []Kind{
	PebbleChangeUpdated,
	PebbleCustomNotice,
	PebbleReady,
}

// WorkloadHooks returns all known container hook kinds.
func WorkloadHooks() []Kind {
	hooks := make([]Kind, len(workloadHooks))
	copy(hooks, workloadHooks)
	return hooks
}

// IsRelation returns whether the Kind represents a relation hook.
func (kind Kind) IsRelation() bool {
	switch kind {
	case RelationCreated, RelationJoined, RelationChanged, RelationDeparted, RelationBroken:
		return true
	}
	return false
}

// IsStorage returns whether the Kind represents a storage hook.
func (kind Kind) IsStorage() bool {
	switch kind {
	case StorageAttached, StorageDetaching:
		return true
	}
	return false
}

// IsWorkload returns whether the Kind represents a workload hook.
func (kind Kind) IsWorkload() bool {
	switch kind {
	case PebbleChangeUpdated, PebbleCustomNotice, PebbleReady:
		return true
	}
	return false
}

var secretHooks = []Kind{
	SecretChanged, SecretExpired, SecretRemove, SecretRotate,
}

// SecretHooks returns all secret hook kinds.
func SecretHooks() []Kind {
	hooks := make([]Kind, len(secretHooks))
	copy(hooks, secretHooks)
	return hooks
}

// IsSecret returns whether the Kind represents a secret hook.
func (kind Kind) IsSecret() bool {
	switch kind {
	case SecretChanged, SecretExpired, SecretRemove, SecretRotate:
		return true
	}
	return false
}
