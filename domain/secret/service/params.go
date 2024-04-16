// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/juju/core/leadership"
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
	Scope       SecretAccessScope
	Subject     SecretAccessor
	Role        secrets.SecretRole
}

// ChangeSecretBackendParams are used to change the backend of a secret.
type ChangeSecretBackendParams struct {
	LeaderToken leadership.Token
	ValueRef    *secrets.ValueRef
	Data        secrets.SecretData
}

// SecretAccessorKind represents the kind of an entity which can access a secret.
type SecretAccessorKind string

// These represent the kinds of secret accessor.
const (
	ApplicationAccessor SecretAccessorKind = "application"
	UnitAccessor        SecretAccessorKind = "unit"
	ModelAccessor       SecretAccessorKind = "model"
)

// SecretAccessor represents an entity that can access a secret.
type SecretAccessor struct {
	Kind SecretAccessorKind
	ID   string
}

// SecretAccessScopeKind represents the kind of an access scope for a secret permission.
type SecretAccessScopeKind string

// These represent the kinds of secret accessor.
const (
	ApplicationAccessScope SecretAccessScopeKind = "application"
	UnitAccessScope        SecretAccessScopeKind = "unit"
	RelationAccessScope    SecretAccessScopeKind = "relation"
	ModelAccessScope       SecretAccessScopeKind = "model"
)

// SecretAccessScope represents the scope of a secret permission.
type SecretAccessScope struct {
	Kind SecretAccessScopeKind
	ID   string
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
