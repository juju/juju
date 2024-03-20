// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import "fmt"

// UserPermissionAccess represents the access a user has to a permission.
type UserPermissionAccess struct {
	// Access is the level of access the user has to the permission.
	Access Access
	// ID represents the unique identifier for the permission.
	// This is used to identify the permission in the system.
	ID ID
}

// Validate returns an error if the UserPermissionAccess is invalid.
func (a UserPermissionAccess) Validate() error {
	if err := a.Access.Validate(); err != nil {
		return fmt.Errorf("invalid access: %w", err)
	}
	if err := a.ID.Validate(); err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	return nil
}
