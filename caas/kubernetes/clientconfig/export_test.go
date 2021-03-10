// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

var (
	NewK8sClientSet               = newK8sClientSet
	EnsureJujuAdminServiceAccount = ensureJujuAdminServiceAccount
	GetOrCreateClusterRole        = getOrCreateClusterRole
	GetOrCreateServiceAccount     = getOrCreateServiceAccount
	GetOrCreateClusterRoleBinding = getOrCreateClusterRoleBinding
	GetServiceAccountSecret       = getServiceAccountSecret
	RemoveJujuAdminServiceAccount = removeJujuAdminServiceAccount
)
