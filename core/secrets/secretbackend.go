// Copyright 2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"fmt"
	"time"

	"github.com/juju/utils/v4"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// IsInternalSecretBackendID returns true if the supplied backend ID is the internal backend ID.
func IsInternalSecretBackendID(backendID string) bool {
	// TODO: Fix me!!! This is not correct anymore because secret backend IDs now are all UUIDs.
	return utils.IsValidUUIDString(backendID)
}

// SecretBackend defines a secrets backend.
type SecretBackend struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	Config              map[string]interface{}
}

// ValueRef represents a reference to a secret
// content value stored in a backend.
type ValueRef struct {
	BackendID  string
	RevisionID string
}

func (r *ValueRef) String() string {
	return fmt.Sprintf("%s:%s", r.BackendID, r.RevisionID)
}

// NextBackendRotateTime returns the next time a token rotate is due,
// given the supplied rotate interval.
func NextBackendRotateTime(now time.Time, rotateInterval time.Duration) (*time.Time, error) {
	if rotateInterval > 0 && rotateInterval < time.Hour {
		return nil, errors.Errorf("token rotate interval %q less than 1h %w", rotateInterval, coreerrors.NotValid)
	}
	// Rotate a reasonable time before the token is due to expire.
	const maxInterval = 24 * time.Hour
	nextInterval := time.Duration(0.75*rotateInterval.Seconds()) * time.Second
	if nextInterval > maxInterval {
		nextInterval = maxInterval
	}
	when := now.Add(nextInterval)
	return &when, nil
}
