// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"time"

	"github.com/juju/utils/v3"
)

type User struct {
	// UUID is the unique identifier for the user.
	UUID UUID

	// CreatedAt is the time that the user was created at.
	CreatedAt time.Time

	// DisplayName is a user-friendly name represent the user as.
	DisplayName string

	// Name is the username of the user.
	Name string

	// CreatorUUID is the associated user that created this user.
	CreatorUUID UUID
}

type UUID string

func NewUUID() (UUID, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(uuid.String()), nil
}

func (u UUID) Validate() error {
	if u == "" {
		return fmt.Errorf("empty uuid")
	}
	if !utils.IsValidUUIDString(string(u)) {
		return fmt.Errorf("invalid uuid: %q", u)
	}
	return nil
}

func (u UUID) String() string {
	return string(u)
}
