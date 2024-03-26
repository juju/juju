// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"fmt"
	"time"

	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
)

// UpsertSecretBackendParams are used to upsert a secret backend.
type UpsertSecretBackendParams struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]string
}

// Validate checks that the parameters are valid.
func (p UpsertSecretBackendParams) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("%w: ID is missing", backenderrors.NotValid)
	}
	for k, v := range p.Config {
		if k == "" {
			return fmt.Errorf(
				"%w: empty config key for %q", backenderrors.NotValid, p.ID)
		}
		if v == "" {
			return fmt.Errorf(
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
	Config map[string]string
}
