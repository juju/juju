// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

type roleSuite struct {
	resourceSuite
}

func TestRoleSuite(t *testing.T) {
	tc.Run(t, &roleSuite{})
}

func (s *roleSuite) TestApply(c *tc.C) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "role1",
			Namespace: "test",
		},
	}
	// Create.
	roleResource := resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", role)
	c.Assert(roleResource.Apply(context.TODO()), tc.ErrorIsNil)
	result, err := s.client.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	roleResource = resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", role)
	c.Assert(roleResource.Apply(context.TODO()), tc.ErrorIsNil)

	result, err = s.client.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
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
	_, err := s.client.RbacV1().Roles("test").Create(context.TODO(), &role1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	roleResource := resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", &template)
	c.Assert(len(roleResource.GetAnnotations()), tc.Equals, 0)
	err = roleResource.Get(context.TODO())
	c.Assert(err, tc.ErrorIsNil)
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
	_, err := s.client.RbacV1().Roles("test").Create(context.TODO(), &role, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `role1`)

	roleResource := resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", &role)
	err = roleResource.Delete(context.TODO())
	c.Assert(err, tc.ErrorIsNil)

	err = roleResource.Get(context.TODO())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}
