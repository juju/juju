// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/errors"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

func getDefaultApplicationNamespaceRules(namespace string) []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups:     []string{""},
			Resources:     []string{"namespaces"},
			ResourceNames: []string{namespace},
			Verbs: []string{
				"get",
				"list",
			},
		},
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
}

var fullAccessApplicationNamespaceRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbacv1.APIGroupAll},
		Resources: []string{rbacv1.ResourceAll},
		Verbs:     []string{rbacv1.VerbAll},
	},
}

var defaultApplicationClusterRules = []rbacv1.PolicyRule{
	{
		// TODO - these are only needed to determine legacy labels or not.
		// We need to migrate away from legacy labels.
		APIGroups: []string{""},
		Resources: []string{"namespaces"},
		Verbs:     []string{"get", "list"},
	},
}

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
	applier := resources.NewApplier()
	err := a.applyTrust(applier, trust)
	if err != nil {
		return errors.Trace(err)
	}
	err = applier.Run(context.Background(), a.client, false)
	return errors.Annotatef(err, "configuring trust for %q", a.name)
}

func (a *app) applyTrust(applier resources.Applier, trust bool) error {
	logger.Debugf(context.TODO(), "application %q, trust %v", a.name, trust)
	if err := a.applyRoles(applier, trust); err != nil {
		return errors.Trace(err)
	}
	return a.applyClusterRoles(applier, trust)
}

func (a *app) roleRules(trust bool) []rbacv1.PolicyRule {
	if trust {
		return fullAccessApplicationNamespaceRules
	}
	return getDefaultApplicationNamespaceRules(a.namespace)
}

func (a *app) applyRoles(applier resources.Applier, trust bool) error {
	role := resources.NewRole(a.serviceAccountName(), a.namespace, nil)
	err := role.Get(context.Background(), a.client)
	if err != nil {
		return errors.Annotatef(err, "getting service account role %q", a.serviceAccountName())
	}

	role.Rules = a.roleRules(trust)
	applier.Apply(role)
	return nil
}

func (a *app) clusterRoleRules(trust bool) []rbacv1.PolicyRule {
	rules := defaultApplicationClusterRules
	if trust {
		rules = fullAccessApplicationClusterRules
	}
	return rules
}

func (a *app) applyClusterRoles(applier resources.Applier, trust bool) error {
	role := resources.NewClusterRole(a.qualifiedClusterName(), nil)
	err := role.Get(context.Background(), a.client)
	if err != nil {
		return errors.Annotatef(err, "getting service account cluster role %q", a.qualifiedClusterName())
	}

	role.Rules = a.clusterRoleRules(trust)
	applier.Apply(role)
	return nil
}
