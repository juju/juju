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

type roleBindingSuite struct {
	resourceSuite
}

func TestRoleBindingSuite(t *testing.T) {
	tc.Run(t, &roleBindingSuite{})
}

func (s *roleBindingSuite) TestApply(c *tc.C) {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	// Create.
	rbResource := resources.NewRoleBinding(s.client.RbacV1().RoleBindings("test"), "test", "roleBinding1", roleBinding)
	c.Assert(rbResource.Apply(context.TODO()), tc.ErrorIsNil)
	result, err := s.client.RbacV1().RoleBindings("test").Get(context.TODO(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	roleBinding.SetAnnotations(map[string]string{"a": "b"})
	rbResource = resources.NewRoleBinding(s.client.RbacV1().RoleBindings("test"), "test", "roleBinding1", roleBinding)
	c.Assert(rbResource.Apply(context.TODO()), tc.ErrorIsNil)

	result, err = s.client.RbacV1().RoleBindings("test").Get(context.TODO(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `roleBinding1`)
	c.Assert(result.GetNamespace(), tc.Equals, `test`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleBindingSuite) TestGet(c *tc.C) {
	template := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	roleBinding1 := template
	roleBinding1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.RbacV1().RoleBindings("test").Create(context.TODO(), &roleBinding1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	rbResource := resources.NewRoleBinding(s.client.RbacV1().RoleBindings("test"), "test", "roleBinding1", &template)
	c.Assert(len(rbResource.GetAnnotations()), tc.Equals, 0)
	err = rbResource.Get(context.TODO())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rbResource.GetName(), tc.Equals, `roleBinding1`)
	c.Assert(rbResource.GetNamespace(), tc.Equals, `test`)
	c.Assert(rbResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *roleBindingSuite) TestDelete(c *tc.C) {
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roleBinding1",
			Namespace: "test",
		},
	}
	_, err := s.client.RbacV1().RoleBindings("test").Create(context.TODO(), &roleBinding, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.RbacV1().RoleBindings("test").Get(context.TODO(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `roleBinding1`)

	rbResource := resources.NewRoleBinding(s.client.RbacV1().RoleBindings("test"), "test", "roleBinding1", &roleBinding)
	err = rbResource.Delete(context.TODO())
	c.Assert(err, tc.ErrorIsNil)

	err = rbResource.Get(context.TODO())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.RbacV1().RoleBindings("test").Get(context.TODO(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}
