// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// UUID represents a relation unique identifier.
type UUID string

// NewUUID is a convenience function for generating a new relation uuid.
func NewUUID() (UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(id.String()), nil
}

// ParseUUID returns a new UUID from the given string. If the string is not a
// valid uuid an error satisfying [errors.NotValid] will be returned.
func ParseUUID(value string) (UUID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", errors.Errorf("parsing relation uuid %q: %w", value, coreerrors.NotValid)
	}
	return UUID(value), nil
}

// String implements the stringer interface for UUID.
func (u UUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u UUID) Validate() error {
	if u == "" {
		return errors.Errorf("relation uuid cannot be empty").Add(coreerrors.NotValid)
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("relation uuid %q: %w", u, coreerrors.NotValid)
	}
	return nil
}

// UnitUUID represents a relation unit unique identifier.
type UnitUUID string

// NewUnitUUID is a convenience function for generating a new relation unit uuid.
func NewUnitUUID() (UnitUUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return UnitUUID(""), err
	}
	return UnitUUID(id.String()), nil
}

// ParseUnitUUID returns a new UUID from the given string. If the string is not a
// valid uuid an error satisfying [errors.NotValid] will be returned.
func ParseUnitUUID(value string) (UnitUUID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", errors.Errorf("parsing relation unit uuid %q: %w", value, coreerrors.NotValid)
	}
	return UnitUUID(value), nil
}

// String implements the stringer interface for UUID.
func (u UnitUUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u UnitUUID) Validate() error {
	if u == "" {
		return errors.Errorf("relation unit uuid cannot be empty").Add(coreerrors.NotValid)
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("relation unit uuid %q: %w", u, coreerrors.NotValid)
	}
	return nil
}

// EndpointUUID represents a relation endpoint unique identifier.
type EndpointUUID string

// NewEndpointUUID is a convenience function for generating a new relation endpoint uuid.
func NewEndpointUUID() (EndpointUUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return EndpointUUID(""), err
	}
	return EndpointUUID(id.String()), nil
}

// ParseEndpointUUID returns a new UUID from the given string. If the string is not a
// valid uuid an error satisfying [errors.NotValid] will be returned.
func ParseEndpointUUID(value string) (EndpointUUID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", errors.Errorf("parsing endpoint uuid %q: %w", value, coreerrors.NotValid)
	}
	return EndpointUUID(value), nil
}

// String implements the stringer interface for UUID.
func (u EndpointUUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u EndpointUUID) Validate() error {
	if u == "" {
		return errors.Errorf("endpoint uuid cannot be empty").Add(coreerrors.NotValid)
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("endpoint uuid %q: %w", u, coreerrors.NotValid)
	}
	return nil
}
