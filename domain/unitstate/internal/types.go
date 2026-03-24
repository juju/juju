// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate"
)

// RelationNetworkInfo contains data to include the relation's network
// info in the unit settings when updated.
type RelationNetworkInfo struct {
	// RelationUUID is the UUID of the relation.
	RelationUUID relation.UUID

	// IngressAddress is the ingress address of the relation.
	IngressAddress string

	// EgressSubnets is a comma separated list of egress subnets
	// of the relation.
	EgressSubnets string
}

// RelationSettings holds a relation uuid and local unit and
// app-level settings, represented by scalar types.
type RelationSettings struct {
	// RelationUUID is the UUID of the relation.
	RelationUUID relation.UUID

	// Settings represent the settings of the unit.
	Settings unitstate.Settings

	// ApplicationSettings represent the settings of the unit.
	ApplicationSettings unitstate.Settings
}

// CommitHookChangesArg contains data needed to commit a hook change
// represented by scalar types.
type CommitHookChangesArg struct {
	// UnitUUID is the uuid of the unit these changes pertain to.
	UnitUUID unit.UUID

	// RelationUnitSettings settings for the relation unit and application
	// which need to be updated.
	RelationSettings []RelationSettings

	// OpenPorts are GroupedPortRanges with ports to be opened.
	// PortRanges are grouped by relation endpoint name.
	OpenPorts network.GroupedPortRanges

	// ClosePorts are GroupedPortRanges with ports to be closed.
	// PortRanges are grouped by relation endpoint name.
	ClosePorts network.GroupedPortRanges

	// CharmState is key/value pairs for charm attributes.
	CharmState *map[string]string

	// SecretCreates contains charm secrets to create.
	SecretCreates []unitstate.CreateSecretArg

	// TrackLatestSecrets is a slice of URIs for which the latest revision should
	// be tracked.
	TrackLatestSecrets []string

	// SecretUpdates contains charm secrets to update.
	SecretUpdates []unitstate.UpdateSecretArg

	// SecretGrants contains charm secrets  to grant access on.
	SecretGrants []unitstate.GrantRevokeSecretArg

	// SecretRevokes contains charm secrets to revoke access on.
	SecretRevokes []unitstate.GrantRevokeSecretArg

	// SecretDeletes contains charm secrets to delete.
	SecretDeletes []unitstate.DeleteSecretArg

	// TODO: (hml) 10-Dec-2025
	// Implement storage
}

// TransformCommitHookChangesArg takes a domain package CommitHookChangesArg
// struct and return an internal package CommitHookChangesArg struct. Does not
// include RelationSettings
func TransformCommitHookChangesArg(in unitstate.CommitHookChangesArg, unitUUID unit.UUID) CommitHookChangesArg {
	return CommitHookChangesArg{
		UnitUUID:           unitUUID,
		OpenPorts:          in.OpenPorts,
		ClosePorts:         in.ClosePorts,
		CharmState:         in.CharmState,
		SecretCreates:      in.SecretCreates,
		TrackLatestSecrets: in.TrackLatestSecrets,
		SecretUpdates:      in.SecretUpdates,
		SecretGrants:       in.SecretGrants,
		SecretRevokes:      in.SecretRevokes,
		SecretDeletes:      in.SecretDeletes,
	}
}
