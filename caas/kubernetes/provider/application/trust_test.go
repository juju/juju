// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
)

func (s *applicationSuite) TestTrust(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	_, err := s.client.RbacV1().Roles(s.namespace).Create(context.Background(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.appName,
			Namespace: s.namespace,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.client.RbacV1().ClusterRoles().Create(context.Background(), &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace + "-" + s.appName,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	err = app.Trust(true)
	c.Assert(err, jc.ErrorIsNil)
	role, err := s.client.RbacV1().Roles(s.namespace).Get(context.Background(), s.appName, metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role.Rules, jc.DeepEquals, []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	})
	clusterRole, err := s.client.RbacV1().ClusterRoles().Get(context.Background(), s.namespace+"-"+s.appName, metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRole.Rules, jc.DeepEquals, []rbacv1.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	})
}

func (s *applicationSuite) TestTrustRoleNotFound(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	err := app.Trust(true)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *applicationSuite) TestTrustClusterRoleNotFound(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	_, err := s.client.RbacV1().Roles(s.namespace).Create(context.Background(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.appName,
			Namespace: s.namespace,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	err = app.Trust(true)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	role, err := s.client.RbacV1().Roles(s.namespace).Get(context.Background(), s.appName, metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role.Rules, jc.DeepEquals, []rbacv1.PolicyRule(nil))
	_, err = s.client.RbacV1().ClusterRoles().Get(context.Background(), s.namespace+"-"+s.appName, metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *applicationSuite) TestRemoveTrust(c *gc.C) {
	app, ctrl := s.getApp(c, caas.DeploymentStateless, true)
	defer ctrl.Finish()

	_, err := s.client.RbacV1().Roles(s.namespace).Create(context.Background(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.appName,
			Namespace: s.namespace,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.client.RbacV1().ClusterRoles().Create(context.Background(), &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace + "-" + s.appName,
		},
	}, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	err = app.Trust(false)
	c.Assert(err, jc.ErrorIsNil)
	role, err := s.client.RbacV1().Roles(s.namespace).Get(context.Background(), s.appName, metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role.Rules, jc.DeepEquals, []rbacv1.PolicyRule{
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
	clusterRole, err := s.client.RbacV1().ClusterRoles().Get(context.Background(), s.namespace+"-"+s.appName, metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRole.Rules, gc.HasLen, 0)
}
