// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/internal/provider/kubernetes/resources"
)

type rbacSuite struct {
	testing.IsolationSuite
	k8sclient *kubernetesClient
}

var _ = gc.Suite(&rbacSuite{})

func (s *rbacSuite) getFakeClient(c *gc.C) *kubernetesClient {
	fakeClient := fake.NewSimpleClientset()

	controllerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	modelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

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

func (s *rbacSuite) SetUpTest(c *gc.C) {
	s.k8sclient = s.getFakeClient(c)
}

func (s *rbacSuite) TestUpdateClusterRole(c *gc.C) {
	ctx := context.Background()

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
	c.Assert(errors.Is(err, errors.NotFound), gc.Equals, true)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(created.Name, gc.Equals, "cluster-role-name")
	c.Assert(created.Rules, gc.DeepEquals, clusterRoleCreated.Rules)

	// Update the cluster role and assert that the rules are updated correctly.
	cr, err := s.k8sclient.updateClusterRole(ctx, clusterRoleUpdate)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cr.Name, gc.Equals, "cluster-role-name")
	c.Assert(cr.Rules, gc.DeepEquals, clusterRoleUpdate.Rules)
}
