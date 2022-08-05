// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// ContextSecrets is a test double for jujuc.ContextSecrets.
type ContextSecrets struct {
	contextBase

	SecretValue secrets.SecretValue
}

// GetSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) GetSecret(uri string) (secrets.SecretValue, error) {
	c.stub.AddCall("GetSecret", uri)
	return c.SecretValue, nil
}

// CreateSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) CreateSecret(args *jujuc.SecretUpsertArgs) (string, error) {
	c.stub.AddCall("CreateSecret", args)
	return "secret:9m4e2mr0ui3e8a215n4g", nil
}

// UpdateSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) UpdateSecret(uri string, args *jujuc.SecretUpsertArgs) error {
	c.stub.AddCall("UpdateSecret", uri, args)
	return nil
}

// GrantSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) GrantSecret(uri string, args *jujuc.SecretGrantRevokeArgs) error {
	c.stub.AddCall("GrantSecret", uri, args)
	return nil
}

// RevokeSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) RevokeSecret(uri string, args *jujuc.SecretGrantRevokeArgs) error {
	c.stub.AddCall("RevokeSecret", uri, args)
	return nil
}
