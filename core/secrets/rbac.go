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
