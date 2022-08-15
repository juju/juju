// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

// SecretRole is an access role on a secret.
type SecretRole string

const (
	RoleView   = SecretRole("view")
	RoleRotate = SecretRole("rotate")
	RoleManage = SecretRole("manage")
)

// IsValid returns true if r is a valid secret role.
func (r SecretRole) IsValid() bool {
	switch r {
	case RoleView, RoleRotate, RoleManage:
		return true
	}
	return false
}
