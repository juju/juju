// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	commonerrors "github.com/juju/juju/apiserver/common/errors"
	"github.com/juju/juju/apiserver/params"
)

// StateBackend exposes State methods needed by credential manager.
type StateBackend interface {
	InvalidateModelCredential(reason string) error
}

type CredentialManagerAPI struct {
	backend StateBackend
}

// NewCredentialManagerAPI creates new model credential manager api endpoint.
func NewCredentialManagerAPI(backend StateBackend) *CredentialManagerAPI {
	return &CredentialManagerAPI{backend: backend}
}

// InvalidateModelCredential marks the cloud credential for this model as invalid.
func (api *CredentialManagerAPI) InvalidateModelCredential(args params.InvalidateCredentialArg) (params.ErrorResult, error) {
	err := api.backend.InvalidateModelCredential(args.Reason)
	if err != nil {
		return params.ErrorResult{Error: commonerrors.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}
