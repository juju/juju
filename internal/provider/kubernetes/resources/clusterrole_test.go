// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	providerutils "github.com/juju/juju/caas/kubernetes/provider/utils"
)

type clusterRoleSuite struct {
	resourceSuite
	clusterRoleClient rbacv1client.ClusterRoleInterface
}

var _ = gc.Suite(&clusterRoleSuite{})

func (s *clusterRoleSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.clusterRoleClient = s.client.RbacV1().ClusterRoles()
}

func (s *clusterRoleSuite) TestApply(c *gc.C) {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "role1",
		},
	}
	// Create.
	clusterRoleResource := resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "role1", role)
	c.Assert(clusterRoleResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	clusterRoleResource = resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "role1", role)
	c.Assert(clusterRoleResource.Apply(context.TODO()), jc.ErrorIsNil)

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

	roleResource := resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "role1", &template)
	c.Assert(len(roleResource.GetAnnotations()), gc.Equals, 0)
	err = roleResource.Get(context.TODO())
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

	roleResource := resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "role1", &role)
	err = roleResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = roleResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = roleResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.RbacV1().ClusterRoles().Get(context.TODO(), "role1", metav1.GetOptions{})
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

	crApi := resources.NewClusterRole(s.client.RbacV1().ClusterRoles(), "test", clusterRole)
	_, err := crApi.Ensure(
		context.TODO(),
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, jc.ErrorIsNil)

	rrole, err := s.client.RbacV1().ClusterRoles().Get(
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
		resources.ClaimFn(func(_ interface{}) (bool, error) { return true, nil }),
	)
	c.Assert(err, jc.ErrorIsNil)

	rrole, err = s.client.RbacV1().ClusterRoles().Get(
		context.TODO(),
		"test",
		metav1.GetOptions{},
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rrole, jc.DeepEquals, &crApi.ClusterRole)
}

func (s *clusterRoleSuite) TestListClusterRoles(c *gc.C) {
	// Set up labels for model and app to list resource.
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

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
	_, err = s.clusterRoleClient.Create(context.TODO(), cr1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create cr2.
	cr2Name := "cr2"
	cr2 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   cr2Name,
			Labels: labelSet,
		},
	}
	_, err = s.clusterRoleClient.Create(context.TODO(), cr2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	cms, err := resources.ListClusterRoles(context.Background(), s.clusterRoleClient, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 2)
	c.Assert(cms[0].GetName(), gc.Equals, cr1Name)
	c.Assert(cms[1].GetName(), gc.Equals, cr2Name)

	// List resources with no labels.
	cms, err = resources.ListClusterRoles(context.Background(), s.clusterRoleClient, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 2)

	// List resources with wrong labels.
	cms, err = resources.ListClusterRoles(context.Background(), s.clusterRoleClient, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(cms), gc.Equals, 0)
}
