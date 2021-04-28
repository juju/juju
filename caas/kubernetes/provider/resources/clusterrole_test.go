// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type clusterRoleSuite struct {
	resourceSuite
}

var _ = gc.Suite(&clusterRoleSuite{})

func (s *clusterRoleSuite) TestApply(c *gc.C) {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "role1",
		},
	}
	// Create.
	clusterRoleResource := resources.NewClusterRole("role1", role)
	c.Assert(clusterRoleResource.Apply(context.TODO(), s.client), jc.ErrorIsNil)
	result, err := s.client.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	clusterRoleResource = resources.NewClusterRole("role1", role)
	c.Assert(clusterRoleResource.Apply(context.TODO(), s.client), jc.ErrorIsNil)

	result, err = s.client.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `role1`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *clusterRoleSuite) TestGet(c *gc.C) {
	template := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "role1",
		},
	}
	role1 := template
	role1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.RbacV1().ClusterRoles().Create(context.TODO(), &role1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	roleResource := resources.NewClusterRole("role1", &template)
	c.Assert(len(roleResource.GetAnnotations()), gc.Equals, 0)
	err = roleResource.Get(context.TODO(), s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(roleResource.GetName(), gc.Equals, `role1`)
	c.Assert(roleResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *clusterRoleSuite) TestDelete(c *gc.C) {
	role := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "role1",
		},
	}
	_, err := s.client.RbacV1().ClusterRoles().Create(context.TODO(), &role, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `role1`)

	roleResource := resources.NewClusterRole("role1", &role)
	err = roleResource.Delete(context.TODO(), s.client)
	c.Assert(err, jc.ErrorIsNil)

	err = roleResource.Get(context.TODO(), s.client)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
