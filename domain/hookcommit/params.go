// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hookcommit

import (
	"time"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	// TODO - move structs from service to top level domain package
	domainsecret "github.com/juju/juju/domain/secret/service"
)

type CommitHookChangesParams struct {
	LeaderToken leadership.Token

	UpdateNetworkInfo    bool
	RelationUnitSettings []RelationUnitSettings
	OpenPorts            network.GroupedPortRanges
	ClosePorts           network.GroupedPortRanges
	UnitState            *UnitState
	AddStorage           []StorageAddParams
	SecretCreates        []CreateCharmSecretParams
	TrackLatest          []string
	SecretUpdates        []UpdateCharmSecretParams
	SecretGrants         []SecretAccessParams
	SecretRevokes        []SecretAccessParams
	SecretDeletes        []DeleteSecretParams
}

type StorageAddParams struct {
	StorageName string
	Pool        string
	Size        uint64
	Count       uint64
}

// UnitState represents the state of the world according to a unit agent at
// hook commit time.
type UnitState struct {
	// CharmState is key/value pairs for charm attributes.
	CharmState map[string]string
}

type RelationUnitSettings struct {
	Relation            string
	UnitSettings        map[string]string
	ApplicationSettings map[string]string
}

// CreateCharmSecretParams are used to create charm a secret.
type CreateCharmSecretParams struct {
	UpdateCharmSecretParams
	Version int

	CharmOwner domainsecret.CharmSecretOwner
}

// UpdateCharmSecretParams are used to update a charm secret.
type UpdateCharmSecretParams struct {
	URI *secrets.URI

	RotatePolicy *secrets.RotatePolicy
	ExpireTime   *time.Time
	Description  *string
	Label        *string
	Params       map[string]interface{}
	Data         secrets.SecretData
	ValueRef     *secrets.ValueRef
	Checksum     string
}

// DeleteSecretParams are used to delete a secret.
type DeleteSecretParams struct {
	URI *secrets.URI

	Revisions []int
}

// SecretAccessParams are used to define access to a secret.
type SecretAccessParams struct {
	URI *secrets.URI

	Scope   domainsecret.SecretAccessScope
	Subject domainsecret.SecretAccessor
	Role    secrets.SecretRole
}
