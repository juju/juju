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
	Config              map[string]interface{}
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
		if v.(string) == "" {
			return fmt.Errorf(
				"%w: empty config value for %q", backenderrors.NotValid, p.ID)
		}
	}
	return nil
}
