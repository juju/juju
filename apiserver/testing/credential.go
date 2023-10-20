// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/domain/credential"
	envcontext "github.com/juju/juju/environs/envcontext"
)

// ConstCredentialGetter returns a CredentialService which serves a fixed credential.
func ConstCredentialGetter(cred *cloud.Credential) *credentialGetter {
	return &credentialGetter{cred: cred}
}

type credentialGetter struct {
	common.CredentialService
	cred *cloud.Credential
}

func (c credentialGetter) CloudCredential(_ context.Context, id credential.ID) (cloud.Credential, error) {
	if c.cred == nil {
		return cloud.Credential{}, errors.NotFoundf("credential %q", id)
	}
	return *c.cred, nil
}

func (c credentialGetter) InvalidateCredential(_ context.Context, _ credential.ID, _ string) error {
	return nil
}

func NoopModelCredentialInvalidatorGetter() (envcontext.ModelCredentialInvalidatorFunc, error) {
	return func(string) error {
		return nil
	}, nil
}
