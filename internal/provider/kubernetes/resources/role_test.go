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

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
)

type roleSuite struct {
	resourceSuite
	namespace  string
	roleClient rbacv1client.RoleInterface
}

var _ = gc.Suite(&roleSuite{})

func (s *roleSuite) SetUpTest(c *gc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.roleClient = s.client.RbacV1().Roles(s.namespace)
}

func (s *roleSuite) TestApply(c *gc.C) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "role1",
			Namespace: "test",
		},
	}
	// Create.
	roleResource := resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", role)
	c.Assert(roleResource.Apply(context.TODO()), jc.ErrorIsNil)
	result, err := s.client.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), gc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	roleResource = resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", role)
	c.Assert(roleResource.Apply(context.TODO()), jc.ErrorIsNil)

	result, err = s.client.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
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
	_, err := s.client.RbacV1().Roles("test").Create(context.TODO(), &role1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	roleResource := resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", &template)
	c.Assert(len(roleResource.GetAnnotations()), gc.Equals, 0)
	err = roleResource.Get(context.TODO())
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
	_, err := s.client.RbacV1().Roles("test").Create(context.TODO(), &role, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.client.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.GetName(), gc.Equals, `role1`)

	roleResource := resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", &role)
	err = roleResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIsNil)

	err = roleResource.Delete(context.TODO())
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	err = roleResource.Get(context.TODO())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = s.client.RbacV1().Roles("test").Get(context.TODO(), "role1", metav1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)
}

func (s *roleSuite) TestListRoles(c *gc.C) {
	// Set up labels for model and app to list resource
	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelName := "testmodel"

	appName := "app1"
	appLabel := providerutils.SelectorLabelsForApp(appName, constants.LabelVersion2)

	modelLabel := providerutils.LabelsForModel(modelName, modelUUID.String(), controllerUUID.String(), constants.LabelVersion2)
	labelSet := providerutils.LabelsMerge(appLabel, modelLabel)

	// Create role1
	role1Name := "role1"
	role1 := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:   role1Name,
			Labels: labelSet,
		},
	}
	_, err = s.roleClient.Create(context.TODO(), role1, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Create role2
	role2Name := "role2"
	role2 := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:   role2Name,
			Labels: labelSet,
		},
	}
	_, err = s.roleClient.Create(context.TODO(), role2, metav1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// List resources with correct labels.
	roles, err := resources.ListRoles(context.Background(), s.roleClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(roles), gc.Equals, 2)
	c.Assert(roles[0].GetName(), gc.Equals, role1Name)
	c.Assert(roles[1].GetName(), gc.Equals, role2Name)

	// List resources with no labels.
	roles, err = resources.ListRoles(context.Background(), s.roleClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(roles), gc.Equals, 2)

	// List resources with wrong labels.
	roles, err = resources.ListRoles(context.Background(), s.roleClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(roles), gc.Equals, 0)
}
