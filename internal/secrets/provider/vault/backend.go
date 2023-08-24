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
	mountPath string
	client    *vault.Client
}

// GetContent implements SecretsBackend.
func (k vaultBackend) GetContent(ctx context.Context, revisionId string) (_ secrets.SecretValue, err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	s, err := k.client.KVv1(k.mountPath).Get(ctx, revisionId)
	if isNotFound(err) {
		return nil, errors.NotFoundf("secret revision %q", revisionId)
	} else if err != nil {
		return nil, errors.Annotatef(err, "getting secret %q", revisionId)
	}
	val := make(map[string]string)
	for k, v := range s.Data {
		val[k] = fmt.Sprintf("%s", v)
	}
	return secrets.NewSecretValue(val), nil
}

// DeleteContent implements SecretsBackend.
func (k vaultBackend) DeleteContent(ctx context.Context, revisionId string) (err error) {
	defer func() {
		err = maybePermissionDenied(err)
	}()

	// Read the content first so we can return a not found error
	// if it doesn't exist.
	_, err = k.client.KVv1(k.mountPath).Get(ctx, revisionId)
	if isNotFound(err) {
		return errors.NotFoundf("secret revision %q", revisionId)
	}
	return k.client.KVv1(k.mountPath).Delete(ctx, revisionId)
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
	err = k.client.KVv1(k.mountPath).Put(ctx, path, val)
	if err != nil {
		return "", errors.Annotatef(err, "saving secret content for %q", path)
	}
	return path, nil
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
	_, err = k.client.Sys().KeyStatus()
	if err == nil {
		return nil
	}
	if isPermissionDenied(err) {
		return errors.New("auth token invalid: permission denied")
	}
	return errors.Annotatef(err, "cannot access backend")
}
