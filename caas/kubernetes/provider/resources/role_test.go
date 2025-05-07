// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type roleSuite struct {
	resourceSuite
}

var _ = tc.Suite(&roleSuite{})

func (s *roleSuite) TestApply(c *tc.C) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "role1",
			Namespace: "test",
		},
	}
	// Create.
	roleResource := resources.NewRole("role1", "test", role)
	c.Assert(roleResource.Apply(context.Background(), s.client), jc.ErrorIsNil)
	result, err := s.client.RbacV1().Roles("test").Get(context.Background(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	roleResource = resources.NewRole("role1", "test", role)
	c.Assert(roleResource.Apply(context.Background(), s.client), jc.ErrorIsNil)

	result, err = s.client.RbacV1().Roles("test").Get(context.Background(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `role1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleSuite) TestGet(c *tc.C) {
	template := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "role1",
			Namespace: "test",
		},
	}
	role1 := template
	role1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.RbacV1().Roles("test").Create(context.Background(), &role1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	roleResource := resources.NewRole("role1", "test", &template)
	c.Assert(len(roleResource.GetAnnotations()), tc.Equals, 0)
	err = roleResource.Get(context.Background(), s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(roleResource.GetName(), tc.Equals, `role1`)
	c.Assert(roleResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(roleResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleSuite) TestDelete(c *tc.C) {
	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "role1",
			Namespace: "test",
		},
	}
	_, err := s.client.RbacV1().Roles("test").Create(context.Background(), &role, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.RbacV1().Roles("test").Get(context.Background(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `role1`)

	roleResource := resources.NewRole("role1", "test", &role)
	err = roleResource.Delete(context.Background(), s.client)
	c.Assert(err, jc.ErrorIsNil)

	err = roleResource.Get(context.Background(), s.client)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	_, err = s.client.RbacV1().Roles("test").Get(context.Background(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
