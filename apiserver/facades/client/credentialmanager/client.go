// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// CredentialManager defines the methods on credentialmanager API endpoint.
type CredentialManager interface {
	InvalidateModelCredential(params.InvalidateCredentialArg) (params.ErrorResult, error)
}

type CredentialManagerAPI struct {
	*credentialcommon.CredentialManagerAPI

	resources facade.Resources
}

var _ CredentialManager = (*CredentialManagerAPI)(nil)

// NewCredentialManagerAPI creates a new CredentialManager API endpoint on server-side.
func NewCredentialManagerAPI(ctx facade.Context) (*CredentialManagerAPI, error) {
	return internalNewCredentialManagerAPI(newStateShim(ctx.State()), ctx.Resources(), ctx.Auth())
}

func internalNewCredentialManagerAPI(backend credentialcommon.StateBackend, resources facade.Resources, authorizer facade.Authorizer) (*CredentialManagerAPI, error) {
	if authorizer.GetAuthTag() == nil || !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &CredentialManagerAPI{
		resources:            resources,
		CredentialManagerAPI: credentialcommon.NewCredentialManagerAPI(backend),
	}, nil
}
