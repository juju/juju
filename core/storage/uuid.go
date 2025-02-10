// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/internal/uuid"
)

// UUID represents a storage unique identifier.
type UUID string

// NewUUID is a convenience function for generating a new storage uuid.
func NewUUID() (UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}
	return UUID(id.String()), nil
}
