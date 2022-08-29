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
func (c *ContextSecrets) GetSecret(uri *secrets.URI, label string, update, peek bool) (secrets.SecretValue, error) {
	c.stub.AddCall("GetSecret", uri.String(), label, update, peek)
	return c.SecretValue, nil
}

// CreateSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) CreateSecret(args *jujuc.SecretUpsertArgs) (*secrets.URI, error) {
	c.stub.AddCall("CreateSecret", args)
	uri, _ := secrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	return uri, nil
}

// UpdateSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) UpdateSecret(uri *secrets.URI, args *jujuc.SecretUpsertArgs) error {
	c.stub.AddCall("UpdateSecret", uri.String(), args)
	return nil
}

// RemoveSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) RemoveSecret(uri *secrets.URI) error {
	c.stub.AddCall("RemoveSecret", uri.String())
	return nil
}

// SecretMetadata gets the metadata for secrets created by the charm.
func (c *ContextSecrets) SecretMetadata() (map[string]jujuc.SecretMetadata, error) {
	c.stub.AddCall("SecretMetadata")
	return map[string]jujuc.SecretMetadata{
		"9m4e2mr0ui3e8a215n4g": {
			LatestRevision: 666,
			Label:          "label",
			Description:    "description",
			RotatePolicy:   secrets.RotateHourly,
		},
	}, nil
}

// GrantSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) GrantSecret(uri *secrets.URI, args *jujuc.SecretGrantRevokeArgs) error {
	c.stub.AddCall("GrantSecret", uri.String(), args)
	return nil
}

// RevokeSecret implements jujuc.ContextSecrets.
func (c *ContextSecrets) RevokeSecret(uri *secrets.URI, args *jujuc.SecretGrantRevokeArgs) error {
	c.stub.AddCall("RevokeSecret", uri.String(), args)
	return nil
}
