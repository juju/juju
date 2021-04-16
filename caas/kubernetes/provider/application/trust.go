// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var defaultApplicationRoles = []rbacv1.PolicyRule{
	{
		APIGroups: []string{""},
		Resources: []string{"pods", "services"},
		Verbs: []string{
			"get",
			"list",
			"patch",
		},
	},
	{
		APIGroups: []string{""},
		Resources: []string{"pods/exec"},
		Verbs: []string{
			"create",
		},
	},
}

var fullAccessApplicationRoles = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbacv1.APIGroupAll},
		Resources: []string{rbacv1.ResourceAll},
		Verbs:     []string{rbacv1.VerbAll},
	},
}

// Trust updates application roles to give full access to the cluster
// by patching the role used for the application pod service account.
func (a *app) Trust(trust bool) error {
	logger.Debugf("application %q, trust %v", a.name, trust)

	rules := defaultApplicationRoles
	if trust {
		rules = fullAccessApplicationRoles
	}

	api := a.client.RbacV1().Roles(a.namespace)
	role, err := api.Get(context.Background(), a.serviceAccountName(), metav1.GetOptions{})
	if err != nil {
		return errors.Annotatef(err, "getting service account role %q", a.serviceAccountName())
	}
	role.Rules = rules

	roleResource := resources.NewRole(role.Name, role.Namespace, role)
	err = roleResource.Apply(context.Background(), a.client)
	return errors.Annotatef(err, "setting rules to %v for role %q", rules, a.serviceAccountName())
}
