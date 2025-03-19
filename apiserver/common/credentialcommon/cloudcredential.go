// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/rpc/params"
)

// StateBackend exposes State methods needed by credential manager.
type StateBackend interface {
	CloudCredentialTag() (names.CloudCredentialTag, bool, error)
	InvalidateModelCredential(reason string) error
}

// CredentialService exposes State methods needed by credential manager.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	InvalidateCredential(ctx context.Context, key credential.Key, reason string) error
}

type CredentialManagerAPI struct {
	backend           StateBackend
	credentialService CredentialService
}

// NewCredentialManagerAPI creates new model credential manager api endpoint.
func NewCredentialManagerAPI(backend StateBackend, credentialService CredentialService) *CredentialManagerAPI {
	return &CredentialManagerAPI{
		backend:           backend,
		credentialService: credentialService,
	}
}

// InvalidateModelCredential marks the cloud credential for this model as invalid.
func (api *CredentialManagerAPI) InvalidateModelCredential(ctx context.Context, args params.InvalidateCredentialArg) (params.ErrorResult, error) {
	tag, ok, err := api.backend.CloudCredentialTag()
	if err != nil {
		return params.ErrorResult{}, errors.Trace(err)
	}
	if !ok {
		return params.ErrorResult{}, nil
	}
	err = api.credentialService.InvalidateCredential(ctx, credential.KeyFromTag(tag), args.Reason)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	err = api.backend.InvalidateModelCredential(args.Reason)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}
