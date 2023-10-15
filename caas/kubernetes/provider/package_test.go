// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"testing"

	gc "gopkg.in/check.v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

func (k *kubernetesClient) EnsureRoleBinding(rb *rbacv1.RoleBinding) (*rbacv1.RoleBinding, []func(), error) {
	return k.ensureRoleBinding(rb)
}
