// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var defaultApplicationNamespaceRules = []rbacv1.PolicyRule{
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

var fullAccessApplicationNamespaceRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbacv1.APIGroupAll},
		Resources: []string{rbacv1.ResourceAll},
		Verbs:     []string{rbacv1.VerbAll},
	},
}

var defaultApplicationClusterRules []rbacv1.PolicyRule

var fullAccessApplicationClusterRules = []rbacv1.PolicyRule{
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
	if err := a.applyRoles(trust); err != nil {
		return errors.Trace(err)
	}
	return a.applyClusterRoles(trust)
}

func (a *app) applyRoles(trust bool) error {
	rules := defaultApplicationNamespaceRules
	if trust {
		rules = fullAccessApplicationNamespaceRules
	}

	api := a.client.RbacV1().Roles(a.namespace)
	role, err := api.Get(context.Background(), a.serviceAccountName(), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("role %q", a.serviceAccountName())
	}
	if err != nil {
		return errors.Annotatef(err, "getting service account role %q", a.serviceAccountName())
	}
	role.Rules = rules

	roleResource := resources.NewRole(role.Name, role.Namespace, role)
	err = roleResource.Apply(context.Background(), a.client)
	return errors.Annotatef(err, "setting rules to %v for namespace role %q", rules, a.serviceAccountName())
}

func (a *app) applyClusterRoles(trust bool) error {
	rules := defaultApplicationClusterRules
	if trust {
		rules = fullAccessApplicationClusterRules
	}

	api := a.client.RbacV1().ClusterRoles()
	role, err := api.Get(context.Background(), a.qualifiedClusterName(), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("cluster role %q", a.qualifiedClusterName())
	}
	if err != nil {
		return errors.Annotatef(err, "getting service account role %q", a.qualifiedClusterName())
	}
	role.Rules = rules

	roleResource := resources.NewClusterRole(role.Name, role)
	err = roleResource.Apply(context.Background(), a.client)
	return errors.Annotatef(err, "setting rules to %v for cluster role %q", rules, a.qualifiedClusterName())
}
