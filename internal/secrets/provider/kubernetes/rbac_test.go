// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type rbacSuite struct {
	testhelpers.IsolationSuite
	k8sclient *kubernetesClient
}

func TestRBACSuite(t *testing.T) {
	tc.Run(t, &rbacSuite{})
}

func (s *rbacSuite) getFakeClient(c *tc.C) *kubernetesClient {
	fakeClient := fake.NewSimpleClientset()

	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	modelUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	broker := &kubernetesClient{
		controllerUUID:    controllerUUID.String(),
		modelUUID:         modelUUID.String(),
		modelName:         "test-model",
		namespace:         "test-namespace",
		serviceAccount:    "default",
		isControllerModel: false,
		client:            fakeClient,
	}
	return broker
}

func (s *rbacSuite) SetUpTest(c *tc.C) {
	s.k8sclient = s.getFakeClient(c)
}

func (s *rbacSuite) TestUpdateClusterRole(c *tc.C) {
	ctx := c.Context()

	// Assert that the cluster role update fails if the cluster role does not exist.
	clusterRoleUpdate := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name: "cluster-role-name",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"group1"},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list"},
			},
		},
	}
	_, err := s.k8sclient.updateClusterRole(ctx, clusterRoleUpdate)
	c.Assert(errors.Is(err, errors.NotFound), tc.Equals, true)

	// Create the cluster role.
	clusterRoleCreated := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name: "cluster-role-name",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"group3"},
				Resources:     []string{"pods"},
				Verbs:         []string{"get", "list", "watch"},
				ResourceNames: []string{"pod1", "pod2"},
			},
		},
	}
	created, err := s.k8sclient.client.RbacV1().ClusterRoles().Create(ctx, clusterRoleCreated, v1.CreateOptions{
		FieldManager: resources.JujuFieldManager,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(created.Name, tc.Equals, "cluster-role-name")
	c.Assert(created.Rules, tc.DeepEquals, clusterRoleCreated.Rules)

	// Update the cluster role and assert that the rules are updated correctly.
	cr, err := s.k8sclient.updateClusterRole(ctx, clusterRoleUpdate)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cr.Name, tc.Equals, "cluster-role-name")
	c.Assert(cr.Rules, tc.DeepEquals, clusterRoleUpdate.Rules)
}
