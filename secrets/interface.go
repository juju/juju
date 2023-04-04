// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
)

const (
	// Version describes the secret format.
	Version = 1
)

// ContentParams represents the content of a secret,
// which is either a secret value or an id used to
// access the content from an external provider like vault.
type ContentParams struct {
	secrets.SecretValue
	BackendId *string
}

// Validate returns an error if the content is invalid.
func (p *ContentParams) Validate() error {
	if p.BackendId == nil && p.SecretValue == nil {
		return errors.NotValidf("secret content without value or provider id")
	}
	return nil
}

// CreateParams are used to create a secret.
type CreateParams struct {
	Version int

	secrets.SecretConfig
	Content ContentParams
	Owner   names.Tag

	LeaderToken leadership.Token
}

// Validate returns an error if params are invalid.
func (p *CreateParams) Validate() error {
	switch p.Owner.Kind() {
	case names.ApplicationTagKind, names.UnitTagKind:
	default:
		return errors.NotValidf("secret owner kind %q", p.Owner.Kind())
	}
	if err := p.Content.Validate(); err != nil {
		return errors.Trace(err)
	}
	return p.SecretConfig.Validate()
}

// UpdateParams are used to update a secret.
type UpdateParams struct {
	secrets.SecretConfig
	Content ContentParams

	LeaderToken leadership.Token
}

// Validate returns an error if params are invalid.
func (p *UpdateParams) Validate() error {
	if err := p.Content.Validate(); err != nil {
		return errors.Trace(err)
	}
	return p.SecretConfig.Validate()
}

type jujuAPIClient interface {
	// GetContentInfo returns info about the content of a secret.
	GetContentInfo(uri *secrets.URI, label string, refresh, peek bool) (*ContentParams, error)
	// GetSecretBackendConfig fetches the config needed to make a secret backend client.
	GetSecretBackendConfig() (*provider.BackendConfig, error)
}

// Backend provides access to a secrets backend.
type Backend interface {
	// GetContent returns the content of a secret, either from an external backend if
	// one is configured, or from Juju.
	GetContent(uri *secrets.URI, label string, refresh, peek bool) (secrets.SecretValue, error)

	// SaveContent saves the content of a secret to an external backend returning the backend id.
	SaveContent(uri *secrets.URI, revision int, value secrets.SecretValue) (string, error)

	// DeleteContent deletes a secret from an external backend.
	DeleteContent(backendId string) error
}
