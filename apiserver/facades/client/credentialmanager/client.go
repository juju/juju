// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"context"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// CredentialManager defines the methods on credentialmanager API endpoint.
type CredentialManager interface {
	InvalidateModelCredential(context.Context, params.InvalidateCredentialArg) (params.ErrorResult, error)
}

type CredentialManagerAPI struct {
	*credentialcommon.CredentialManagerAPI

	resources facade.Resources
}

var _ CredentialManager = (*CredentialManagerAPI)(nil)

func internalNewCredentialManagerAPI(backend credentialcommon.StateBackend, credentialService credentialcommon.CredentialService, resources facade.Resources, authorizer facade.Authorizer) (*CredentialManagerAPI, error) {
	return &CredentialManagerAPI{
		resources:            resources,
		CredentialManagerAPI: credentialcommon.NewCredentialManagerAPI(backend, credentialService),
	}, nil
}
