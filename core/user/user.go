// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"time"

	"github.com/juju/utils/v3"
)

// User represents a user in the system.
type User struct {
	// UUID is the unique identifier for the user.
	UUID UUID

	// Name is the username of the user.
	Name string

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string

	// CreatorUUID is the associated user that created this user.
	CreatorUUID UUID

	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time

	// LastLogin is the last time the user logged in.
	LastLogin time.Time

	// Disabled is true if the user is disabled.
	Disabled bool
}

// UUID is a unique identifier for a user.
type UUID string

// NewUUID returns a new UUID.
func NewUUID() (UUID, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(uuid.String()), nil
}

// Validate returns an error if the UUID is invalid.
func (u UUID) Validate() error {
	if u == "" {
		return fmt.Errorf("empty uuid")
	}
	if !utils.IsValidUUIDString(string(u)) {
		return fmt.Errorf("invalid uuid: %q", u)
	}
	return nil
}

// String returns the UUID as a string.
func (u UUID) String() string {
	return string(u)
}
