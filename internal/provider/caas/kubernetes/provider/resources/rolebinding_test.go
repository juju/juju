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

	"github.com/juju/juju/internal/provider/caas/kubernetes/provider/resources"
)

type roleBindingSuite struct {
	resourceSuite
}

var _ = gc.Suite(&roleBindingSuite{})

func (s *roleBindingSuite) TestApply(c *gc.C) {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	// Create.
	rbResource := resources.NewRoleBinding("roleBinding1", "test", roleBinding)
	c.Assert(rbResource.Apply(context.Background(), s.client), jc.ErrorIsNil)
	result, err := s.client.RbacV1().RoleBindings("test").Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	roleBinding.SetAnnotations(map[string]string{"a": "b"})
	rbResource = resources.NewRoleBinding("roleBinding1", "test", roleBinding)
	c.Assert(rbResource.Apply(context.Background(), s.client), jc.ErrorIsNil)

	result, err = s.client.RbacV1().RoleBindings("test").Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `roleBinding1`)
	c.Assert(result.GetNamespace(), gc.Equals, `test`)
	c.Assert(result.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleBindingSuite) TestGet(c *gc.C) {
	template := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	roleBinding1 := template
	roleBinding1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.RbacV1().RoleBindings("test").Create(context.Background(), &roleBinding1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	rbResource := resources.NewRoleBinding("roleBinding1", "test", &template)
	c.Assert(len(rbResource.GetAnnotations()), gc.Equals, 0)
	err = rbResource.Get(context.Background(), s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rbResource.GetName(), gc.Equals, `roleBinding1`)
	c.Assert(rbResource.GetNamespace(), gc.Equals, `test`)
	c.Assert(rbResource.GetAnnotations(), gc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleBindingSuite) TestDelete(c *gc.C) {
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	_, err := s.client.RbacV1().RoleBindings("test").Create(context.Background(), &roleBinding, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.RbacV1().RoleBindings("test").Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `roleBinding1`)

	rbResource := resources.NewRoleBinding("roleBinding1", "test", &roleBinding)
	err = rbResource.Delete(context.Background(), s.client)
	c.Assert(err, jc.ErrorIsNil)

	err = rbResource.Get(context.Background(), s.client)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	_, err = s.client.RbacV1().RoleBindings("test").Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}
