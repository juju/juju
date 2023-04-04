// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

// SecretRole is an access role on a secret.
type SecretRole string

const (
	RoleNone   = SecretRole("")
	RoleView   = SecretRole("view")
	RoleRotate = SecretRole("rotate")
	RoleManage = SecretRole("manage")
)

// IsValid returns true if r is a valid secret role.
func (r SecretRole) IsValid() bool {
	switch r {
	case RoleNone, RoleView, RoleRotate, RoleManage:
		return true
	}
	return false
}

func (r SecretRole) value() int {
	switch r {
	case RoleView:
		return 1
	case RoleRotate:
		return 2
	case RoleManage:
		return 3
	default:
		return -1
	}
}

func (r SecretRole) Allowed(wanted SecretRole) bool {
	v1, v2 := r.value(), wanted.value()
	if v1 < 0 || v2 < 0 {
		return false
	}
	return v1 >= v2
}
