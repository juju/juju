// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func (k *kubernetesClient) EnsureRoleBinding(ctx context.Context, rb *rbacv1.RoleBinding) (*rbacv1.RoleBinding, []func(), error) {
	return k.ensureRoleBinding(ctx, rb)
}

func (k *kubernetesClient) EnsureServiceAccount(ctx context.Context, sa *core.ServiceAccount) (*core.ServiceAccount, []func(), error) {
	return k.ensureServiceAccount(ctx, sa)
}

func (k *kubernetesClient) EnsureRole(ctx context.Context, role *rbacv1.Role) (*rbacv1.Role, []func(), error) {
	return k.ensureRole(ctx, role)
}
