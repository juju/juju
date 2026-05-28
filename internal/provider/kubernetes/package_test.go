// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"testing"

	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

func (k *kubernetesClient) EnsureRoleBinding(rb *rbacv1.RoleBinding) (*rbacv1.RoleBinding, []func(), error) {
	return k.ensureRoleBinding(rb)
}

func (k *kubernetesClient) EnsureServiceAccount(sa *core.ServiceAccount) (*core.ServiceAccount, []func(), error) {
	return k.ensureServiceAccount(sa)
}

func (k *kubernetesClient) EnsureRole(role *rbacv1.Role) (*rbacv1.Role, []func(), error) {
	return k.ensureRole(role)
}
