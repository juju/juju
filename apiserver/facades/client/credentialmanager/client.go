// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"context"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
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

func internalNewCredentialManagerAPI(backend credentialcommon.StateBackend, resources facade.Resources, authorizer facade.Authorizer) (*CredentialManagerAPI, error) {
	if authorizer.GetAuthTag() == nil || !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &CredentialManagerAPI{
		resources:            resources,
		CredentialManagerAPI: credentialcommon.NewCredentialManagerAPI(backend),
	}, nil
}
