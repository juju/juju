// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package hooks

// Kind enumerates the different kinds of hooks that exist.
type Kind string

const (
	// None of these hooks are ever associated with a relation; each of them
	// represents a change to the state of the unit as a whole. The values
	// themselves are all valid hook names.

	// In normal operation, a unit will run at least the `install`, `start`, `config-changed`
	// and `stop` hooks over the course of its lifetime.

	// The `install` hook always runs once, and only once, before any other hook.
	Install Kind = "install"

	// The `start` hook always runs once immediately after the first config-changed
	// hook. Also, on kubernetes charms, whenever a unit’s pod churns, `start` will
	// be fired again on that unit.
	Start Kind = "start"

	// The `config-changed` hook always runs once immediately after the install hook,
	// and likewise after the upgrade-charm hook. It also runs whenever the service
	// configuration changes, and when recovering from transient unit agent errors.
	ConfigChanged Kind = "config-changed"

	// The `upgrade-charm` hook always runs once immediately after the charm directory
	// contents have been changed by an unforced charm upgrade operation, and *may* do
	// so after a forced upgrade; but will *not* be run after a forced upgrade from an
	// existing error state. (Consequently, neither will the config-changed hook that
	// would ordinarily follow the upgrade-charm.)
	UpgradeCharm Kind = "upgrade-charm"

	// The `stop` hook is the last hook to be run before the unit is destroyed. In the
	// future, it may be called in other situations.
	Stop Kind = "stop"

	Remove Kind = "remove"
	Action Kind = "action"

	LeaderElected Kind = "leader-elected"
	LeaderDeposed Kind = "leader-deposed"

	UpdateStatus Kind = "update-status"

	// These hooks require an associated secret.
	SecretChanged Kind = "secret-changed"
	SecretExpired Kind = "secret-expired"
	SecretRemove  Kind = "secret-remove"
	SecretRotate  Kind = "secret-rotate"

	// These 5 hooks require an associated relation, and the name of the relation
	// unit whose change triggered the hook. The hook file names that these
	// kinds represent will be prefixed by the relation name; for example,
	// "db-relation-joined".

	RelationCreated Kind = "relation-created"

	// The "relation-joined" hook always runs once when a related unit is first seen.
	RelationJoined Kind = "relation-joined"

	// The "relation-changed" hook for a given unit always runs once immediately
	// following the relation-joined hook for that unit, and subsequently whenever
	// the related unit changes its settings (by calling relation-set and exiting
	// without error). Note that "immediately" only applies within the context of
	// this particular runtime relation -- that is, when "foo-relation-joined" is
	// run for unit "bar/99" in relation id "foo:123", the only guarantee is that
	// the next hook to be run *in relation id "foo:123"* will be "foo-relation-changed"
	// for "bar/99". Unit hooks may intervene, as may hooks for other relations,
	// and even for other "foo" relations.
	RelationChanged Kind = "relation-changed"

	// The "relation-departed" hook for a given unit always runs once when a related
	// unit is no longer related. After the "relation-departed" hook has run, no
	// further notifications will be received from that unit; however, its settings
	// will remain accessible via relation-get for the complete lifetime of the
	// relation.
	RelationDeparted Kind = "relation-departed"

	// The "relation-broken" hook is not specific to any unit, and always runs once
	// when the local unit is ready to depart the relation itself. Before this hook
	// is run, a relation-departed hook will be executed for every unit known to be
	// related; it will never run while the relation appears to have members, but it
	// may be the first and only hook to run for a given relation. The stop hook will
	// not run while relations remain to be broken.
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
	PebbleCustomNotice   Kind = "pebble-custom-notice"
	PebbleReady          Kind = "pebble-ready"
	PebbleCheckFailed    Kind = "pebble-check-failed"
	PebbleCheckRecovered Kind = "pebble-check-recovered"
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
	UpdateStatus,
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
	PebbleCheckFailed,
	PebbleCheckRecovered,
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
	case PebbleCheckFailed, PebbleCheckRecovered, PebbleCustomNotice, PebbleReady:
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
