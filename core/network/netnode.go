package network

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// NetNodeUUID represents a net node unique identifier.
type NetNodeUUID string

// NewNetNodeUUID is a convenience function for generating a new
// net node uuid.
func NewNetNodeUUID() (NetNodeUUID, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return NetNodeUUID(""), err
	}
	return NetNodeUUID(id.String()), nil
}

// ParseNetNodeUUID returns a new UUID from the given string. If the string is not a
// valid uuid an error satisfying [errors.NotValid] will be returned.
func ParseNetNodeUUID(value string) (NetNodeUUID, error) {
	if !uuid.IsValidUUIDString(value) {
		return "", errors.Errorf("parsing relation uuid %q: %w", value, coreerrors.NotValid)
	}
	return NetNodeUUID(value), nil
}

// String implements the stringer interface for UUID.
func (u NetNodeUUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u NetNodeUUID) Validate() error {
	if u == "" {
		return errors.Errorf("relation uuid cannot be empty").Add(coreerrors.NotValid)
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("relation uuid %q: %w", u, coreerrors.NotValid)
	}
	return nil
}
