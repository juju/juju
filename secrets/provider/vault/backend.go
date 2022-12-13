// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	vault "github.com/mittwald/vaultgo"

	"github.com/juju/juju/core/secrets"
)

type vaultBackend struct {
	modelUUID string
	client    *vault.Client
}

// GetContent implements SecretsBackend.
func (k vaultBackend) GetContent(ctx context.Context, backendId string) (_ secrets.SecretValue, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	s, err := k.client.KVv1(k.modelUUID).Get(ctx, backendId)
	if err != nil {
		return nil, errors.Annotatef(err, "getting secret %q", backendId)
	}
	val := make(map[string]string)
	for k, v := range s.Data {
		val[k] = fmt.Sprintf("%s", v)
	}
	return secrets.NewSecretValue(val), nil
}

// DeleteContent implements SecretsBackend.
func (k vaultBackend) DeleteContent(ctx context.Context, backendId string) (err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	err = k.client.KVv1(k.modelUUID).Delete(ctx, backendId)
	if isNotFound(err) {
		return nil
	}
	return err
}

// SaveContent implements SecretsBackend.
func (k vaultBackend) SaveContent(ctx context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (_ string, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	path := uri.Name(revision)
	val := make(map[string]interface{})
	for k, v := range value.EncodedValues() {
		val[k] = v
	}
	err = k.client.KVv1(k.modelUUID).Put(ctx, path, val)
	return path, errors.Annotatef(err, "saving secret content for %q", uri)
}

// Ping implements SecretsBackend.
func (k vaultBackend) Ping() error {
	h, err := k.client.Sys().Health()
	if err != nil {
		return errors.Annotate(err, "backend not reachable")
	}
	if !h.Initialized {
		return errors.New("vault is not initialised")
	}
	if h.Sealed {
		return errors.New("vault is sealed")
	}
	return nil
}
