// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/secrets"
)

const (
	// Version describes the secret format.
	Version = 1
)

// CreateParams are used to create a secret.
type CreateParams struct {
	UpsertParams

	Version int
	Owner   string
}

// Validate returns an error if params are invalid.
func (p *CreateParams) Validate() error {
	tag, err := names.ParseTag(p.Owner)
	if err != nil {
		return errors.Trace(err)
	}
	switch tag.Kind() {
	case names.ApplicationTagKind, names.UnitTagKind:
	default:
		return errors.NotValidf("secret owner kind %q", tag.Kind())
	}
	return p.UpsertParams.Validate()
}

// UpsertParams are used to update a secret.
type UpsertParams struct {
	LeaderToken    leadership.Token
	Description    *string
	Label          *string
	RotatePolicy   *secrets.RotatePolicy
	NextRotateTime *time.Time
	ExpireTime     *time.Time
	Params         map[string]interface{}
	Data           map[string]string
}

// Validate returns an error if params are invalid.
func (p *UpsertParams) Validate() error {
	if p.RotatePolicy != nil && !p.RotatePolicy.IsValid() {
		return errors.NotValidf("secret rotate policy %q", p.RotatePolicy)
	}
	if p.RotatePolicy.WillRotate() && p.NextRotateTime == nil {
		return errors.New("cannot specify a secret rotate policy without a next rotate time")
	}
	if !p.RotatePolicy.WillRotate() && p.NextRotateTime != nil {
		return errors.New("cannot specify a secret rotate time without a rotate policy")
	}
	return nil
}

// Filter is used when querying secrets.
type Filter struct {
	URI      *secrets.URI
	Revision *int
	OwnerTag *string
}

// SecretsService instances provide a backend for storing secrets values.
type SecretsService interface {
	// CreateSecret creates a new secret with the given URI.
	CreateSecret(context.Context, *secrets.URI, CreateParams) (*secrets.SecretMetadata, error)

	// UpdateSecret updates a given secret with a new secret value.
	UpdateSecret(context.Context, *secrets.URI, UpsertParams) (*secrets.SecretMetadata, error)

	// DeleteSecret deletes the specified secret.
	DeleteSecret(context.Context, *secrets.URI) error

	// GetSecret returns the metadata for the specified secret.
	GetSecret(context.Context, *secrets.URI) (*secrets.SecretMetadata, error)

	// GetSecretValue returns the value of the specified secret.
	GetSecretValue(context.Context, *secrets.URI, int) (secrets.SecretValue, error)

	// ListSecrets returns secret metadata using the specified filter.
	ListSecrets(context.Context, Filter) ([]*secrets.SecretMetadata, map[string][]*secrets.SecretRevisionMetadata, error)
}

// ProviderConfig is used when constructing a secrets provider.
// TODO(wallyworld) - use a schema
type ProviderConfig map[string]interface{}
