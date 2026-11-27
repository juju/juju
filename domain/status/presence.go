// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// PresenceToken identifies the agent login that recorded a unit or machine
// presence.
type PresenceToken string

// NewPresenceToken returns a new PresenceToken.
func NewPresenceToken() (PresenceToken, error) {
	id, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}
	return PresenceToken(id.String()), nil
}

// Validate ensures the consistency of the PresenceToken. If the token is
// invalid an error satisfying [coreerrors.NotValid] is returned.
func (t PresenceToken) Validate() error {
	if t == "" {
		return errors.Errorf("presence token cannot be empty").Add(coreerrors.NotValid)
	}
	if !uuid.IsValidUUIDString(string(t)) {
		return errors.Errorf("presence token %q: %w", t, coreerrors.NotValid)
	}
	return nil
}

// String returns the PresenceToken in string form.
func (t PresenceToken) String() string {
	return string(t)
}

// IsEmpty returns whether the PresenceToken is empty.
func (t PresenceToken) IsEmpty() bool {
	return t == ""
}
