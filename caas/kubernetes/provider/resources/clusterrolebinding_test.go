// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
)

type clusterRoleBindingSuite struct {
	resourceSuite
}

var _ = tc.Suite(&clusterRoleBindingSuite{})

func (s *clusterRoleBindingSuite) TestApply(c *tc.C) {
	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "roleBinding1",
		},
	}
	// Create.
	rbResource := resources.NewClusterRoleBinding("roleBinding1", roleBinding)
	c.Assert(rbResource.Apply(context.Background(), s.client), tc.ErrorIsNil)
	result, err := s.client.RbacV1().ClusterRoleBindings().Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	roleBinding.SetAnnotations(map[string]string{"a": "b"})
	rbResource = resources.NewClusterRoleBinding("roleBinding1", roleBinding)
	c.Assert(rbResource.Apply(context.Background(), s.client), tc.ErrorIsNil)

	result, err = s.client.RbacV1().ClusterRoleBindings().Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `roleBinding1`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *clusterRoleBindingSuite) TestGet(c *tc.C) {
	template := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "roleBinding1",
		},
	}
	roleBinding1 := template
	roleBinding1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.RbacV1().ClusterRoleBindings().Create(context.Background(), &roleBinding1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	rbResource := resources.NewClusterRoleBinding("roleBinding1", &template)
	c.Assert(len(rbResource.GetAnnotations()), tc.Equals, 0)
	err = rbResource.Get(context.Background(), s.client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rbResource.GetName(), tc.Equals, `roleBinding1`)
	c.Assert(rbResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *clusterRoleBindingSuite) TestDelete(c *tc.C) {
	roleBinding := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "roleBinding1",
		},
	}
	_, err := s.client.RbacV1().ClusterRoleBindings().Create(context.Background(), &roleBinding, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.RbacV1().ClusterRoleBindings().Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `roleBinding1`)

	rbResource := resources.NewClusterRoleBinding("roleBinding1", &roleBinding)
	err = rbResource.Delete(context.Background(), s.client)
	c.Assert(err, tc.ErrorIsNil)

	err = rbResource.Get(context.Background(), s.client)
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	_, err = s.client.RbacV1().ClusterRoleBindings().Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *clusterRoleBindingSuite) TestDeleteWithoutPreconditions(c *tc.C) {
	roleBinding := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "roleBinding1",
		},
	}
	_, err := s.client.RbacV1().ClusterRoleBindings().Create(context.Background(), &roleBinding, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.RbacV1().ClusterRoleBindings().Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `roleBinding1`)

	rbResource := resources.NewClusterRoleBinding("roleBinding1", nil)
	err = rbResource.Delete(context.Background(), s.client)
	c.Assert(err, tc.ErrorIsNil)

	err = rbResource.Get(context.Background(), s.client)
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	_, err = s.client.RbacV1().ClusterRoleBindings().Get(context.Background(), "roleBinding1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

// This test ensures that there has not been a regression with ensure cluster
// role where it can not update roles that have a labels change.
// https://bugs.launchpad.net/juju/+bug/1929909
func (s *clusterRoleBindingSuite) TestEnsureClusterRoleBindingRegressionOnLabelChange(c *tc.C) {
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
	}

	crbApi := resources.NewClusterRoleBinding("test", clusterRoleBinding)
	_, err := crbApi.Ensure(
		context.Background(),
		s.client,
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, tc.ErrorIsNil)

	rroleBinding, err := s.client.RbacV1().ClusterRoleBindings().Get(
		context.Background(),
		"test",
		metav1.GetOptions{},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rroleBinding, tc.DeepEquals, clusterRoleBinding)

	crbApi.ClusterRoleBinding.ObjectMeta.Labels = map[string]string{
		"new-label": "new-value",
	}

	crbApi.Ensure(
		context.Background(),
		s.client,
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, tc.ErrorIsNil)

	rroleBinding, err = s.client.RbacV1().ClusterRoleBindings().Get(
		context.Background(),
		"test",
		metav1.GetOptions{},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rroleBinding, tc.DeepEquals, &crbApi.ClusterRoleBinding)
}

func (s *clusterRoleBindingSuite) TestEnsureRecreatesOnRoleRefChange(c *tc.C) {
	clusterRoleBinding := resources.NewClusterRoleBinding(
		"test",
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				Labels: map[string]string{
					"foo": "bar",
				},
				ResourceVersion: "1",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "test",
					APIGroup:  "api",
					Name:      "test",
					Namespace: "test",
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "test",
				Kind:     "test",
				Name:     "test",
			},
		},
	)

	_, err := clusterRoleBinding.Ensure(
		context.Background(),
		s.client,
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, tc.ErrorIsNil)

	rval, err := s.client.RbacV1().ClusterRoleBindings().Get(
		context.Background(),
		"test",
		metav1.GetOptions{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rval.ObjectMeta.ResourceVersion, tc.Equals, "1")

	clusterRoleBinding1 := resources.NewClusterRoleBinding(
		"test",
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				Labels: map[string]string{
					"foo": "bar",
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "test",
					APIGroup:  "api",
					Name:      "test",
					Namespace: "test",
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "test1",
				Kind:     "test1",
				Name:     "test1",
			},
		},
	)

	_, err = clusterRoleBinding1.Ensure(
		context.Background(),
		s.client,
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, tc.ErrorIsNil)
}
