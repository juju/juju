// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/watcher"
)

// ConstCredentialGetter returns a CredentialService which serves a fixed credential.
func ConstCredentialGetter(cred *cloud.Credential) *credentialGetter {
	return &credentialGetter{cred: cred}
}

type credentialGetter struct {
	cred *cloud.Credential
}

func (c credentialGetter) CloudCredential(_ context.Context, key credential.Key) (cloud.Credential, error) {
	if c.cred == nil {
		return cloud.Credential{}, errors.NotFoundf("credential %q", key)
	}
	return *c.cred, nil
}

func (credentialGetter) InvalidateCredential(_ context.Context, _ credential.Key, _ string) error {
	return nil
}

func (credentialGetter) WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error) {
	return nil, nil
}
