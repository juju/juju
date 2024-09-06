// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/leadership"
	coremodel "github.com/juju/juju/core/model"
	secretservice "github.com/juju/juju/domain/secret/service"
)

// DrainBackendConfigParams are used to get config for draining a secret backend.
type DrainBackendConfigParams struct {
	GrantedSecretsGetter secretservice.GrantedSecretsGetter
	LeaderToken          leadership.Token
	Accessor             secretservice.SecretAccessor
	ModelUUID            coremodel.UUID
	BackendID            string
}

// BackendConfigParams are used to get config for reading secrets from a secret backend.
type BackendConfigParams struct {
	GrantedSecretsGetter secretservice.GrantedSecretsGetter
	LeaderToken          leadership.Token
	Accessor             secretservice.SecretAccessor
	ModelUUID            coremodel.UUID
	BackendIDs           []string
	SameController       bool
}
