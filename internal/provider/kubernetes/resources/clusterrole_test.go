// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

type clusterRoleSuite struct {
	resourceSuite
}

func TestClusterRoleSuite(t *testing.T) {
	tc.Run(t, &clusterRoleSuite{})
}

func (s *clusterRoleSuite) TestApply(c *tc.C) {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "role1",
		},
	}
	// Create.
	clusterRoleResource := resources.NewClusterRole("role1", role)
	c.Assert(clusterRoleResource.Apply(c.Context(), s.client), tc.ErrorIsNil)
	result, err := s.client.RbacV1().ClusterRoles().Get(c.Context(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	clusterRoleResource = resources.NewClusterRole("role1", role)
	c.Assert(clusterRoleResource.Apply(c.Context(), s.client), tc.ErrorIsNil)

	result, err = s.client.RbacV1().ClusterRoles().Get(c.Context(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `role1`)
	c.Assert(result.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *clusterRoleSuite) TestGet(c *tc.C) {
	template := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "role1",
		},
	}
	role1 := template
	role1.SetAnnotations(map[string]string{"a": "b"})
	_, err := s.client.RbacV1().ClusterRoles().Create(c.Context(), &role1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	roleResource := resources.NewClusterRole("role1", &template)
	c.Assert(len(roleResource.GetAnnotations()), tc.Equals, 0)
	err = roleResource.Get(c.Context(), s.client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(roleResource.GetName(), tc.Equals, `role1`)
	c.Assert(roleResource.GetAnnotations(), tc.DeepEquals, map[string]string{"a": "b"})
}

func (s *clusterRoleSuite) TestDelete(c *tc.C) {
	role := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "role1",
		},
	}
	_, err := s.client.RbacV1().ClusterRoles().Create(c.Context(), &role, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.RbacV1().ClusterRoles().Get(c.Context(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `role1`)

	roleResource := resources.NewClusterRole("role1", &role)
	err = roleResource.Delete(c.Context(), s.client)
	c.Assert(err, tc.ErrorIsNil)

	err = roleResource.Get(c.Context(), s.client)
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	_, err = s.client.RbacV1().ClusterRoles().Get(c.Context(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

// This test ensures that there has not been a regression with ensure cluster
// role where it can not update roles that have a labels change.
// https://bugs.launchpad.net/juju/+bug/1929909
func (s *clusterRoleSuite) TestEnsureClusterRoleRegressionOnLabelChange(c *tc.C) {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"namespaces"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"admissionregistration.k8s.io"},
				Resources: []string{"mutatingwebhookconfigurations"},
				Verbs: []string{
					"create",
					"delete",
					"get",
					"list",
					"update",
				},
			},
		},
	}

	crApi := resources.NewClusterRole("test", clusterRole)
	_, err := crApi.Ensure(
		c.Context(),
		s.client,
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, tc.ErrorIsNil)

	rrole, err := s.client.RbacV1().ClusterRoles().Get(
		c.Context(),
		"test",
		metav1.GetOptions{},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rrole, tc.DeepEquals, clusterRole)

	crApi.ClusterRole.ObjectMeta.Labels = map[string]string{
		"new-label": "new-value",
	}

	crApi.Ensure(
		c.Context(),
		s.client,
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, tc.ErrorIsNil)

	rrole, err = s.client.RbacV1().ClusterRoles().Get(
		c.Context(),
		"test",
		metav1.GetOptions{},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rrole, tc.DeepEquals, &crApi.ClusterRole)
}
