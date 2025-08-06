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
	c.Assert(clusterRoleResource.Apply(context.TODO(), s.coreClient), jc.ErrorIsNil)
	result, err := s.coreClient.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	clusterRoleResource = resources.NewClusterRole("role1", role)
	c.Assert(clusterRoleResource.Apply(context.TODO(), s.coreClient), jc.ErrorIsNil)

	result, err = s.coreClient.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
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
	_, err := s.coreClient.RbacV1().ClusterRoles().Create(context.TODO(), &role1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	roleResource := resources.NewClusterRole("role1", &template)
	c.Assert(len(roleResource.GetAnnotations()), gc.Equals, 0)
	err = roleResource.Get(context.TODO(), s.coreClient)
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
	_, err := s.coreClient.RbacV1().ClusterRoles().Create(context.TODO(), &role, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.coreClient.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `role1`)

	roleResource := resources.NewClusterRole("role1", &role)
	err = roleResource.Delete(context.TODO(), s.coreClient)
	c.Assert(err, jc.ErrorIsNil)

	err = roleResource.Get(context.TODO(), s.coreClient)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.coreClient.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

// This test ensures that there has not been a regression with ensure cluster
// role where it can not update roles that have a labels change.
// https://bugs.launchpad.net/juju/+bug/1929909
func (s *clusterRoleSuite) TestEnsureClusterRoleRegressionOnLabelChange(c *gc.C) {
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
		context.TODO(),
		s.coreClient,
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, jc.ErrorIsNil)

	rrole, err := s.coreClient.RbacV1().ClusterRoles().Get(
		context.TODO(),
		"test",
		metav1.GetOptions{},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rrole, jc.DeepEquals, clusterRole)

	crApi.ClusterRole.ObjectMeta.Labels = map[string]string{
		"new-label": "new-value",
	}

	crApi.Ensure(
		context.TODO(),
		s.coreClient,
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, jc.ErrorIsNil)

	rrole, err = s.coreClient.RbacV1().ClusterRoles().Get(
		context.TODO(),
		"test",
		metav1.GetOptions{},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rrole, jc.DeepEquals, &crApi.ClusterRole)
}
