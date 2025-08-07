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

type roleSuite struct {
	resourceSuite
}

var _ = gc.Suite(&roleSuite{})

func (s *roleSuite) TestApply(c *gc.C) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "role1",
			Namespace: "test",
		},
	}
	// Create.
	roleResource := resources.NewRole("role1", "test", role)
	c.Assert(roleResource.Apply(context.TODO(), s.coreClient, s.extendedClient), jc.ErrorIsNil)
	result, err := s.coreClient.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	roleResource = resources.NewRole("role1", "test", role)
	c.Assert(roleResource.Apply(context.TODO(), s.coreClient, s.extendedClient), jc.ErrorIsNil)

	result, err = s.coreClient.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `role1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleSuite) TestGet(c *gc.C) {
	template := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "role1",
			Namespace: "test",
		},
	}
	role1 := template
	role1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.coreClient.RbacV1().Roles("test").Create(context.TODO(), &role1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	roleResource := resources.NewRole("role1", "test", &template)
	c.Assert(len(roleResource.GetAnnotations()), gc.Equals, 0)
	err = roleResource.Get(context.TODO(), s.coreClient, s.extendedClient)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(roleResource.GetName(), gc.Equals, `role1`)
	c.Assert(roleResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(roleResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleSuite) TestDelete(c *gc.C) {
	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "role1",
			Namespace: "test",
		},
	}
	_, err := s.coreClient.RbacV1().Roles("test").Create(context.TODO(), &role, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.coreClient.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `role1`)

	roleResource := resources.NewRole("role1", "test", &role)
	err = roleResource.Delete(context.TODO(), s.coreClient, s.extendedClient)
	c.Assert(err, jc.ErrorIsNil)

	err = roleResource.Get(context.TODO(), s.coreClient, s.extendedClient)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.coreClient.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
