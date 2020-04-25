// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/charm/v7"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/names/v4"
)

// Snapshot is a snapshot of the remote state of the unit.
type Snapshot struct {
	// Life is the lifecycle state of the unit.
	Life life.Value

	// Relations contains the lifecycle states of
	// each of the application's relations, keyed by
	// relation IDs.
	Relations map[int]RelationSnapshot

	// Storage contains the lifecycle and attached
	// states of each of the unit's storage attachments.
	Storage map[names.StorageTag]StorageSnapshot

	// CharmModifiedVersion is increased whenever the application's charm was
	// changed in some way.
	CharmModifiedVersion int

	// CharmURL is the charm URL that the unit is
	// expected to run.
	CharmURL *charm.URL

	// ForceCharmUpgrade reports whether the unit
	// should upgrade even in an error state.
	ForceCharmUpgrade bool

	// ResolvedMode reports the method of resolving
	// hook execution errors.
	ResolvedMode params.ResolvedMode

	// ProviderID is the cloud container's provider ID.
	ProviderID string

	// RetryHookVersion increments each time a failed
	// hook is meant to be retried if ResolvedMode is
	// set to ResolvedNone.
	RetryHookVersion int

	// ConfigHash is a hash of the last published version of the
	// unit's config settings.
	ConfigHash string

	// TrustHash is a hash of the last published version of the unit's
	// trust settings.
	TrustHash string

	// AddressesHash is a hash of the last published addresses for the
	// unit's machine/container.
	AddressesHash string

	// Leader indicates whether or not the unit is the
	// elected leader.
	Leader bool

	// LeaderSettingsVersion is the last published
	// version of the leader settings for the application.
	LeaderSettingsVersion int

	// UpdateStatusVersion increments each time an
	// update-status hook is supposed to run.
	UpdateStatusVersion int

	// ActionsPending is the list of pending actions to
	// be performed by this unit.
	ActionsPending []string

	// ActionChanged contains a monotonically incrementing
	// integer to signify changes in the Action's remote state.
	ActionChanged map[string]int

	// ActionsBlocked is true on CAAS when actions cannot be run due to
	// pod initialization.
	ActionsBlocked bool

	// Commands is the list of IDs of commands to be
	// executed by this unit.
	Commands []string

	// UpgradeSeriesStatus is the preparation status of any currently running
	// series upgrade
	UpgradeSeriesStatus model.UpgradeSeriesStatus

	// ContainerRunningStatus is set on CAAS models for remote init/upgrade of charm.
	ContainerRunningStatus *ContainerRunningStatus
}

// RelationSnapshot tracks the state of a relationship from the viewpoint of the local unit.
type RelationSnapshot struct {
	// Life indicates whether this relation is active, stopping or dead
	Life life.Value

	// Suspended is used by cross-model relations to indicate the offer has
	// disabled the relation, but has not removed it entirely.
	Suspended bool

	// Members tracks the Change version of each member's data bag
	Members map[string]int64

	// ApplicationMembers tracks the Change version of each member's application data bag
	ApplicationMembers map[string]int64
}

// StorageSnapshot has information relating to a storage
// instance belonging to a unit.
type StorageSnapshot struct {
	Kind     params.StorageKind
	Life     life.Value
	Attached bool
	Location string
}
