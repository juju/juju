// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
)

// GrantedSecretsGetter returns the revisions on the given backend for which
// consumers have access with the given role.
type GrantedSecretsGetter func(
	ctx context.Context, backendID string, role coresecrets.SecretRole, consumers ...secretservice.SecretAccessor,
) ([]*coresecrets.SecretRevisionRef, error)

// DrainBackendConfigParams are used to get config for draining a secret backend.
type DrainBackendConfigParams struct {
	GrantedSecretsGetter GrantedSecretsGetter
	LeaderToken          leadership.Token
	Accessor             secretservice.SecretAccessor
	ModelUUID            coremodel.UUID
	BackendID            string
}

// BackendConfigParams are used to get config for reading secrets from a secret backend.
type BackendConfigParams struct {
	GrantedSecretsGetter GrantedSecretsGetter
	LeaderToken          leadership.Token
	Accessor             secretservice.SecretAccessor
	ModelUUID            coremodel.UUID
	BackendIDs           []string
	SameController       bool
}
