// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hookcommit

import (
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	domainsecret "github.com/juju/juju/domain/secret"
)

type CommitHookChangesArgs struct {
	// Original approach - replaced by WithLease()
	// LeaderToken leadership.Token

	UnitUUID        coreunit.UUID
	ApplicationUUID coreapplication.ID

	UpdateNetworkInfo    bool
	RelationUnitSettings []RelationUnitSettingsArgs
	OpenPorts            network.GroupedPortRanges
	ClosePorts           network.GroupedPortRanges
	UnitState            *UnitStateArgs
	AddStorage           []StorageAddArgs
	TrackLatest          []string

	AppSecretCreates  []CreateSecretArgs
	UnitSecretCreates []CreateSecretArgs
	SecretUpdates     []domainsecret.UpsertSecretParams
	SecretGrants      []domainsecret.GrantParams
	SecretRevokes     []domainsecret.AccessParams
	SecretDeletes     []DeleteSecretArgs
}

type StorageAddArgs struct {
	StorageName string
	Pool        string
	Size        uint64
	Count       uint64
}

type UnitStateArgs struct {
	// CharmState is key/value pairs for charm attributes.
	CharmState map[string]string
}

type CreateSecretArgs struct {
	domainsecret.UpsertSecretParams
	URI     *secrets.URI
	Version int
}

type RelationUnitSettingsArgs struct {
	Relation            string
	UnitSettings        map[string]string
	ApplicationSettings map[string]string
}

type DeleteSecretArgs struct {
	URI       *secrets.URI
	Revisions []int
}
