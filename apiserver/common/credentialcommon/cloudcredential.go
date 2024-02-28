// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/rpc/params"
)

// StateBackend exposes State methods needed by credential manager.
type StateBackend interface {
	CloudCredentialTag() (names.CloudCredentialTag, bool, error)
	InvalidateModelCredential(reason string) error
}

// CredentialService exposes State methods needed by credential manager.
type CredentialService interface {
	CloudCredential(ctx context.Context, id credential.ID) (cloud.Credential, error)
	InvalidateCredential(ctx context.Context, id credential.ID, reason string) error
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
	err = api.credentialService.InvalidateCredential(ctx, credential.IdFromTag(tag), args.Reason)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	err = api.backend.InvalidateModelCredential(args.Reason)
	if err != nil {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

// CredentialInvalidatorGetter returns a getter for a function used to invalidate the cloud credential
// for the model associated with the facade context.
func CredentialInvalidatorGetter(ctx facade.ModelContext) envcontext.ModelCredentialInvalidatorGetter {
	return ModelCredentialInvalidatorGetter(ctx.ServiceFactory().Credential(), stateShim{State: ctx.State()})
}

// ModelCredentialInvalidatorGetter returns a getter for a function used to invalidate the cloud credential
// for the model associated with the specified state.
func ModelCredentialInvalidatorGetter(credentialService CredentialService, st StateBackend) envcontext.ModelCredentialInvalidatorGetter {
	return func() (envcontext.ModelCredentialInvalidatorFunc, error) {
		idGetter := func() (credential.ID, error) {
			credTag, _, err := st.CloudCredentialTag()
			if err != nil {
				return credential.ID{}, errors.Trace(err)
			}
			return credential.IdFromTag(credTag), nil
		}
		invalidator := envcontext.NewCredentialInvalidator(idGetter, credentialService.InvalidateCredential, st.InvalidateModelCredential)
		return invalidator.InvalidateModelCredential, nil
	}
}
