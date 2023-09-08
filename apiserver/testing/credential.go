// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
)

// ConstCredentialGetter returns a CredentialService which serves a fixed credential.
func ConstCredentialGetter(cred *cloud.Credential) *credentialGetter {
	return &credentialGetter{cred: cred}
}

type credentialGetter struct {
	common.CredentialService
	cred *cloud.Credential
}

func (c credentialGetter) CloudCredential(_ context.Context, tag names.CloudCredentialTag) (cloud.Credential, error) {
	if c.cred == nil {
		return cloud.Credential{}, errors.NotFoundf("credential %q", tag)
	}
	return *c.cred, nil
}
