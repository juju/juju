// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/secret"
	secretservice "github.com/juju/juju/domain/secret/service"
)

// DrainBackendConfigParams are used to get config for draining a secret backend.
type DrainBackendConfigParams struct {
	GrantedSecretsGetter secretservice.GrantedSecretsGetter
	LeaderToken          leadership.Token
	Accessor             secret.SecretAccessor
	ModelUUID            coremodel.UUID
	BackendID            string
}

// BackendConfigParams are used to get config for reading secrets from a secret backend.
type BackendConfigParams struct {
	GrantedSecretsGetter secretservice.GrantedSecretsGetter
	LeaderToken          leadership.Token
	Accessor             secret.SecretAccessor
	ModelUUID            coremodel.UUID
	BackendIDs           []string
	SameController       bool
	// ReservedSecretIDs contains IDs of secrets that have been reserved by
	// the accessor but not yet persisted to state. The K8s backend cannot
	// restrict the "create" verb by resource name, so it pre-creates an
	// empty placeholder and grants the restricted service account "patch"
	// access to it by name. Without the reserved secret ID in the owned
	// list, policyRulesForSecretAccess omits the secrets rule entirely
	// (the "if len(owned) > 0" guard is intentional to avoid granting
	// access to all secrets), and SaveContent will be forbidden.
	// These IDs are derived server-side from the accessor's reservations,
	// not supplied by the agent.
	ReservedSecretIDs []string
}
