// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
)

// CreateSecretParams are used to create a secret.
type CreateSecretParams struct {
	UpdateSecretParams
	Version int

	// Either a charm secret owner is needed, or a user secret is needed.
	CharmOwner *CharmSecretOwner
	UserSecret bool
}

// UpdateSecretParams are used to update a secret.
type UpdateSecretParams struct {
	LeaderToken  leadership.Token
	RotatePolicy *secrets.RotatePolicy
	ExpireTime   *time.Time
	Description  *string
	Label        *string
	Params       map[string]interface{}
	Data         secrets.SecretData
	ValueRef     *secrets.ValueRef
	AutoPrune    *bool
}

// SecretAccessParams are used to define access to a secret.
type SecretAccessParams struct {
	LeaderToken leadership.Token
	Scope       permission.ID
	Subject     permission.ID
	Role        secrets.SecretRole
}

// ChangeSecretBackendParams are used to change the backend of a secret.
type ChangeSecretBackendParams struct {
	LeaderToken leadership.Token
	ValueRef    *secrets.ValueRef
	Data        secrets.SecretData
}

// SecretConsumer represents the consumer of a secret.
// Exactly one of ApplicationName or UnitName must be non nil.
type SecretConsumer struct {
	ApplicationName *string
	UnitName        *string
}

// SecretAccessor represents an entity that can access a secret.
// Exactly one of ApplicationName, UnitName, or ModelUUID must be non nil.
type SecretAccessor struct {
	ApplicationName *string
	UnitName        *string
	ModelUUID       *string
}

// CharmSecretOwnerKind represents the kind of a charm secret owner entity.
type CharmSecretOwnerKind string

// These represent the kinds of charm secret owner.
const (
	ApplicationOwner CharmSecretOwnerKind = "application"
	UnitOwner        CharmSecretOwnerKind = "unit"
)

// CharmSecretOwner is the owner of a secret.
// This is used to query or watch secrets for specified owners.
type CharmSecretOwner struct {
	Kind CharmSecretOwnerKind
	ID   string
}
