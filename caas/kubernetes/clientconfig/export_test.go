// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

var (
	NewK8sClientSet                               = newK8sClientSet
	EnsureJujuAdminRBACResources                  = ensureJujuAdminRBACResources
	EnsureClusterRole                             = ensureClusterRole
	EnsureServiceAccount                          = ensureServiceAccount
	EnsureClusterRoleBinding                      = ensureClusterRoleBinding
	GetServiceAccountSecret                       = getServiceAccountSecret
	ReplaceAuthProviderWithServiceAccountAuthData = replaceAuthProviderWithServiceAccountAuthData
)
