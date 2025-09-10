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
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	providerutils "github.com/juju/juju/internal/provider/kubernetes/utils"
	"github.com/juju/juju/internal/uuid"
)

type roleSuite struct {
	resourceSuite
	namespace  string
	roleClient rbacv1client.RoleInterface
}

func TestRoleSuite(t *testing.T) {
	tc.Run(t, &roleSuite{})
}

func (s *roleSuite) SetUpTest(c *tc.C) {
	s.resourceSuite.SetUpTest(c)
	s.namespace = "ns1"
	s.roleClient = s.client.RbacV1().Roles(s.namespace)
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
	c.Assert(roleResource.Apply(c.Context()), tc.ErrorIsNil)
	result, err := s.client.RbacV1().Roles("test").Get(c.Context(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(result.GetAnnotations()), tc.Equals, 0)

	// Update.
	role.SetAnnotations(map[string]string{"a": "b"})
	roleResource = resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", role)
	c.Assert(roleResource.Apply(c.Context()), tc.ErrorIsNil)

	result, err = s.client.RbacV1().Roles("test").Get(c.Context(), "role1", metav1.GetOptions{})
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
	_, err := s.client.RbacV1().Roles("test").Create(c.Context(), &role1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	roleResource := resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", &template)
	c.Assert(len(roleResource.GetAnnotations()), tc.Equals, 0)
	err = roleResource.Get(c.Context())
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
	_, err := s.client.RbacV1().Roles("test").Create(c.Context(), &role, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.client.RbacV1().Roles("test").Get(c.Context(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.GetName(), tc.Equals, `role1`)

	roleResource := resources.NewRole(s.client.RbacV1().Roles("test"), "test", "role1", &role)
	err = roleResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = roleResource.Delete(c.Context())
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	err = roleResource.Get(c.Context())
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	_, err = s.client.RbacV1().Roles("test").Get(c.Context(), "role1", metav1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)
}

func (s *roleSuite) TestListRoles(c *tc.C) {
	// Set up labels for model and app to list resource
	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

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
	_, err = s.roleClient.Create(c.Context(), role1, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// Create role2
	role2Name := "role2"
	role2 := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:   role2Name,
			Labels: labelSet,
		},
	}
	_, err = s.roleClient.Create(c.Context(), role2, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	// List resources with correct labels.
	roles, err := resources.ListRoles(context.Background(), s.roleClient, s.namespace, metav1.ListOptions{
		LabelSelector: labelSet.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(roles), tc.Equals, 2)
	c.Assert(roles[0].GetName(), tc.Equals, role1Name)
	c.Assert(roles[1].GetName(), tc.Equals, role2Name)

	// List resources with no labels.
	roles, err = resources.ListRoles(context.Background(), s.roleClient, s.namespace, metav1.ListOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(roles), tc.Equals, 2)

	// List resources with wrong labels.
	roles, err = resources.ListRoles(context.Background(), s.roleClient, s.namespace, metav1.ListOptions{
		LabelSelector: "foo=bar",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(roles), tc.Equals, 0)
}
