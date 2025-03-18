// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"github.com/juju/errors"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/uuid"
)

// UUID represents a upgrade unique identifier.
type UUID string

// NewUUID returns a new UUID.
func NewUUID() (UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Trace(err)
	}
	return UUID(id.String()), nil
}

// MustNewUUID returns a new UUID or panics.
func MustNewUUID() UUID {
	id, err := NewUUID()
	if err != nil {
		panic(err)
	}
	return id
}

// Validate ensures the consistency of the UUID.
func (u UUID) Validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	if !utils.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}

// String implements the stringer interface for UUID.
func (u UUID) String() string {
	return string(u)
}
