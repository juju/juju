// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"time"

	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
)

// BackendIdentifier is used to identify a secret backend.
type BackendIdentifier struct {
	ID   string
	Name string
}

// String returns the identifier as a string.
func (i BackendIdentifier) String() string {
	if i.Name != "" {
		return i.Name
	}
	return i.ID
}

// CreateSecretBackendParams are used to create a secret backend.
type CreateSecretBackendParams struct {
	BackendIdentifier
	BackendType         string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]string
}

// Validate checks that the parameters are valid.
func (p CreateSecretBackendParams) Validate() error {
	if p.ID == "" {
		return errors.Errorf("%w: ID is missing", backenderrors.NotValid)
	}
	if p.Name == "" {
		return errors.Errorf("%w: name is missing", backenderrors.NotValid)
	}
	if p.BackendType == "" {
		return errors.Errorf("%w: type is missing", backenderrors.NotValid)
	}
	for k, v := range p.Config {
		if k == "" {
			return errors.Errorf(
				"%w: empty config key for %q", backenderrors.NotValid, p.Name)

		}
		if v == "" {
			return errors.Errorf(
				"%w: empty config value for %q", backenderrors.NotValid, p.Name)

		}
	}
	return nil
}

// UpdateSecretBackendParams are used to update a secret backend.
type UpdateSecretBackendParams struct {
	BackendIdentifier
	NewName             *string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]string
}

// Validate checks that the parameters are valid.
func (p UpdateSecretBackendParams) Validate() error {
	if p.ID == "" && p.Name == "" {
		// We need at least one identifier.
		// So we can identify the secret backend to update by ID or name.
		return errors.Errorf("%w: both ID and name are missing", backenderrors.NotValid)
	}
	if p.ID != "" && p.Name != "" {
		// We just need one identifier.
		return errors.Errorf("%w: both ID and name are set", backenderrors.NotValid)
	}
	if p.NewName != nil && *p.NewName == "" {
		return errors.Errorf("%w: name cannot be set to empty", backenderrors.NotValid)
	}
	for k, v := range p.Config {
		if k == "" {
			return errors.Errorf(
				"%w: empty config key for %q", backenderrors.NotValid, p.ID)

		}
		if v == "" {
			return errors.Errorf(
				"%w: empty config value for %q", backenderrors.NotValid, p.ID)

		}
	}
	return nil
}

// SecretBackend represents a secret backend instance from state.
type SecretBackend struct {
	// ID is the unique identifier of the secret backend.
	ID string
	// Name is the human-readable name of the secret backend.
	Name string
	// BackendType is the type of the secret backend.
	BackendType string
	// TokenRotateInterval is the interval at which the token should be rotated.
	TokenRotateInterval *time.Duration
	// NextRotateTime is the time at which the next token rotation should occur.
	NextRotateTime *time.Time
	// Config is the configuration of the secret backend.
	Config map[string]any
	// NumSecrets is the number of secrets stored in the secret backend.
	NumSecrets int
}
