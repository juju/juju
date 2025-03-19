// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"github.com/juju/errors"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/uuid"
)

// UUID is a unique removal job identifier.
type UUID string

// NewUUID returns a new UUID.
func NewUUID() (UUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Trace(err)
	}
	return UUID(id.String()), nil
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

// String returns the UUID in string form.
func (u UUID) String() string {
	return string(u)
}
