// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

var (
	NewK8sClientSet               = newK8sClientSet
	EnsureJujuAdminServiceAccount = ensureJujuAdminServiceAccount
	GetOrCreateClusterRole        = getOrCreateClusterRole
	GetOrCreateServiceAccount     = getOrCreateServiceAccount
	GetOrCreateClusterRoleBinding = getOrCreateClusterRoleBinding
	RemoveJujuAdminServiceAccount = removeJujuAdminServiceAccount
)
