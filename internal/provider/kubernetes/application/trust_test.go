// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
)

func (s *applicationSuite) TestTrust(c *tc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	_, err := s.client.RbacV1().Roles(s.namespace).Create(c.Context(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.appName,
			Namespace: s.namespace,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.client.RbacV1().ClusterRoles().Create(c.Context(), &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace + "-" + s.appName,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	err = app.Trust(true)
	c.Assert(err, tc.ErrorIsNil)
	role, err := s.client.RbacV1().Roles(s.namespace).Get(c.Context(), s.appName, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role.Rules, tc.DeepEquals, []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	})
	clusterRole, err := s.client.RbacV1().ClusterRoles().Get(c.Context(), s.namespace+"-"+s.appName, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clusterRole.Rules, tc.DeepEquals, []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	})
}

func (s *applicationSuite) TestTrustRoleNotFound(c *tc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	err := app.Trust(true)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *applicationSuite) TestTrustClusterRoleNotFound(c *tc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	_, err := s.client.RbacV1().Roles(s.namespace).Create(c.Context(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.appName,
			Namespace: s.namespace,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	err = app.Trust(true)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	role, err := s.client.RbacV1().Roles(s.namespace).Get(c.Context(), s.appName, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role.Rules, tc.DeepEquals, []rbacv1.PolicyRule(nil))
	_, err = s.client.RbacV1().ClusterRoles().Get(c.Context(), s.namespace+"-"+s.appName, metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *applicationSuite) TestRemoveTrust(c *tc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	_, err := s.client.RbacV1().Roles(s.namespace).Create(c.Context(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.appName,
			Namespace: s.namespace,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.client.RbacV1().ClusterRoles().Create(c.Context(), &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace + "-" + s.appName,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	err = app.Trust(false)
	c.Assert(err, tc.ErrorIsNil)
	role, err := s.client.RbacV1().Roles(s.namespace).Get(c.Context(), s.appName, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role.Rules, tc.DeepEquals, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs: []string{
				"get",
				"list",
			},
			ResourceNames: []string{s.namespace},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "services"},
			Verbs:     []string{"get", "list", "patch"},
		}, {
			APIGroups: []string{""},
			Resources: []string{"pods/exec"},
			Verbs:     []string{"create"},
		},
	})
	clusterRole, err := s.client.RbacV1().ClusterRoles().Get(c.Context(), s.namespace+"-"+s.appName, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clusterRole.Rules, tc.DeepEquals, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs: []string{
				"get",
				"list",
			},
		},
	})
}
