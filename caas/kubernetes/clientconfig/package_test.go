// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

var (
	NewK8sClientSet               = newK8sClientSet
	EnsureJujuAdminServiceAccount = ensureJujuAdminServiceAccount
	GetOrCreateClusterRole        = getOrCreateClusterRole
	GetOrCreateServiceAccount     = getOrCreateServiceAccount
	GetOrCreateClusterRoleBinding = getOrCreateClusterRoleBinding
	RemoveJujuAdminServiceAccount = removeJujuAdminServiceAccount
)
