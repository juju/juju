// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
)

// Snapshot is a snapshot of the remote state of the unit.
type Snapshot struct {
	// Life is the lifecycle state of the unit.
	Life params.Life

	// Relations contains the lifecycle states of
	// each of the service's relations, keyed by
	// relation IDs.
	Relations map[int]params.Life

	// Storage contains the lifecycle states of
	// each of the unit's storage attachments.
	Storage map[names.StorageTag]params.Life

	// CharmURL is the charm URL that the unit is
	// expected to run.
	CharmURL *charm.URL

	// ForceCharmUpgrade reports whether the unit
	// should upgrade even in an error state.
	ForceCharmUpgrade bool

	// ResolvedMode reports the method of resolving
	// hook execution errors.
	ResolvedMode params.ResolvedMode

	// ConfigVersion is the last published version of
	// the unit's config settings.
	ConfigVersion int

	// LeaderSettingsVersion is the last published
	// version of the leader settings for the service.
	LeaderSettingsVersion int
}
