// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
)

const (
	// Version describes the secret format.
	Version = 1
)

// ContentParams represents the content of a secret,
// which is either a secret value or a reference used to
// access the content from an external provider like vault.
type ContentParams struct {
	secrets.SecretValue
	ValueRef *secrets.ValueRef
}

// Validate returns an error if the content is invalid.
func (p *ContentParams) Validate() error {
	if p.ValueRef == nil && p.SecretValue == nil {
		return errors.NotValidf("secret content without value or backend reference")
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

// JujuAPIClient provides access to the SecretsManager facade.
type JujuAPIClient interface {
	// GetContentInfo returns info about the content of a secret and the backend config
	// needed to make a backend client if necessary.
	GetContentInfo(uri *secrets.URI, label string, refresh, peek bool) (*ContentParams, *provider.ModelBackendConfig, bool, error)
	// GetRevisionContentInfo returns info about the content of a secret revision and the backend config
	// needed to make a backend client if necessary.
	// If pendingDelete is true, the revision is marked for deletion.
	GetRevisionContentInfo(uri *secrets.URI, revision int, pendingDelete bool) (*ContentParams, *provider.ModelBackendConfig, bool, error)
	// GetSecretBackendConfig fetches the config needed to make secret backend clients.
	// If backendID is nil, return the current active backend (if any).
	GetSecretBackendConfig(backendID *string) (*provider.ModelBackendConfigInfo, error)

	// GetBackendConfigForDrain fetches the config needed to make a secret backend client for the drain worker.
	GetBackendConfigForDrain(backendID *string) (*provider.ModelBackendConfig, string, error)
}

// BackendsClient provides access to a client which can access secret backends.
type BackendsClient interface {
	// GetContent returns the content of a secret, either from an external backend if
	// one is configured, or from Juju.
	GetContent(uri *secrets.URI, label string, refresh, peek bool) (secrets.SecretValue, error)

	// GetRevisionContent returns the content of a secret revision, either from an external backend if
	// one is configured, or from Juju.
	GetRevisionContent(uri *secrets.URI, revision int) (secrets.SecretValue, error)

	// SaveContent saves the content of a secret to an external backend returning the backend id.
	SaveContent(uri *secrets.URI, revision int, value secrets.SecretValue) (secrets.ValueRef, error)

	// DeleteContent deletes a secret from an external backend
	// if it exists there.
	DeleteContent(uri *secrets.URI, revision int) error

	// DeleteExternalContent deletes a secret from an external backend.
	DeleteExternalContent(ref secrets.ValueRef) error

	// GetBackend returns the secret client for the provided backend ID.
	GetBackend(backendID *string, forDrain bool) (provider.SecretsBackend, string, error)
}
