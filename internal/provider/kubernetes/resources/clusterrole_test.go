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
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type clusterRoleSuite struct {
	resourceSuite
	clusterRoleClient rbacv1client.ClusterRoleInterface
}

func TestClusterRoleSuite(t *testing.T) {
	tc.Run(t, &clusterRoleSuite{})
}

func (s *clusterRoleSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.clusterRoleClient = s.client.RbacV1().ClusterRoles()
}

func (s *clusterRoleSuite) TestApply(c *tc.C) {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "role1",
		},
	}
	// Create.
	clusterRoleResource := resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "role1", role)
	c.Assert(clusterRoleResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.RbacV1().ClusterRoles().Get(c.Context(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	clusterRoleResource = resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "role1", role)
	c.Assert(clusterRoleResource.Apply(c.Context()), tc.ErrorIsNil)

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

	roleResource := resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "role1", &template)
	c.Assert(len(roleResource.GetAnnotations()), tc.Equals, 0)
	err = roleResource.Get(c.Context())
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

	roleResource := resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "role1", &role)
	err = roleResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = roleResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = roleResource.Get(c.Context())
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

	crApi := resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "test", clusterRole)
	_, err := crApi.Ensure(
		c.Context(),
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

func (s *clusterRoleSuite) TestListClusterRoles(c *tc.C) {
	// Set up labels for model and app to list resource.
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelName := "testmodel"

	appName := "app1"
	appLabel := providerutils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := providerutils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := providerutils.LabelsMerge(appLabel, modelLabel)

	// Create cr1.
	cr1Name := "cr1"
	cr1 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   cr1Name,
			Labels: labelSet,
		},
	}
	_, err = s.clusterRoleClient.Create(c.Context(), cr1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create cr2.
	cr2Name := "cr2"
	cr2 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   cr2Name,
			Labels: labelSet,
		},
	}
	_, err = s.clusterRoleClient.Create(c.Context(), cr2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	cms, err := resources.ListClusterRoles(c.Context(), s.clusterRoleClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(cms), tc.Equals, 2)
	c.Assert(cms[0].GetName(), tc.Equals, cr1Name)
	c.Assert(cms[1].GetName(), tc.Equals, cr2Name)

	// List resources with no labels.
	cms, err = resources.ListClusterRoles(c.Context(), s.clusterRoleClient, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(cms), tc.Equals, 2)

	// List resources with wrong labels.
	cms, err = resources.ListClusterRoles(c.Context(), s.clusterRoleClient, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(cms), tc.Equals, 0)
}
