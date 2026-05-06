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
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
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

func (s *rbacSuite) managedLabels() map[string]string {
	return map[string]string{
		constants.LabelKubernetesAppManaged: resources.JujuFieldManager,
	}
}

func (s *rbacSuite) modelAnnotations(modelUUID string) map[string]string {
	return map[string]string{
		modelIdKey: modelUUID,
	}
}

func (s *rbacSuite) TestCreateServiceAccountErrors(c *gc.C) {
	ctx := context.Background()
	saName := "sa-conflict"

	_, err := s.k8sclient.client.CoreV1().ServiceAccounts(s.k8sclient.namespace).Create(ctx, &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:   saName,
			Labels: map[string]string{},
		},
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createServiceAccount(ctx, &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{Name: saName},
	})
	c.Assert(err, gc.ErrorMatches, `service account "sa-conflict" exists and is not managed by Juju`)

	_, err = s.k8sclient.client.CoreV1().ServiceAccounts(s.k8sclient.namespace).Create(ctx, &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        "sa-other-model",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations("different-model"),
		},
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createServiceAccount(ctx, &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{Name: "sa-other-model"},
	})
	c.Assert(err, gc.ErrorMatches, `service account "sa-other-model" exists and is not managed by this model`)
}

func (s *rbacSuite) TestCreateRoleErrors(c *gc.C) {
	ctx := context.Background()

	_, err := s.k8sclient.client.RbacV1().Roles(s.k8sclient.namespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:   "role-not-juju",
			Labels: map[string]string{},
		},
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createRole(ctx, &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{Name: "role-not-juju"},
	})
	c.Assert(err, gc.ErrorMatches, `role "role-not-juju" exists and is not managed by Juju`)

	_, err = s.k8sclient.client.RbacV1().Roles(s.k8sclient.namespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        "role-other-model",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations("different-model"),
		},
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createRole(ctx, &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{Name: "role-other-model"},
	})
	c.Assert(err, gc.ErrorMatches, `role "role-other-model" exists and is not managed by this model`)

	_, err = s.k8sclient.client.RbacV1().Roles(s.k8sclient.namespace).Create(ctx, &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        "role-rules-change",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations(s.k8sclient.modelUUID),
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
			Verbs:     []string{"get"},
		}},
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createRole(ctx, &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{Name: "role-rules-change"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list"},
		}},
	})
	c.Assert(errors.Is(err, errors.NotSupported), gc.Equals, true)
	c.Assert(err, gc.ErrorMatches, `.*changing rules for role "role-rules-change" not supported`)
}

func (s *rbacSuite) TestCreateRoleBindingErrors(c *gc.C) {
	ctx := context.Background()

	rbRef := rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "a-role"}
	rbSubjects := []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa", Namespace: s.k8sclient.namespace}}

	_, err := s.k8sclient.client.RbacV1().RoleBindings(s.k8sclient.namespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:   "rb-not-juju",
			Labels: map[string]string{},
		},
		RoleRef:  rbRef,
		Subjects: rbSubjects,
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, _, err = s.k8sclient.createRoleBinding(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{Name: "rb-not-juju"},
		RoleRef:    rbRef,
		Subjects:   rbSubjects,
	})
	c.Assert(err, gc.ErrorMatches, `role binding "rb-not-juju" exists and is not managed by Juju`)

	_, err = s.k8sclient.client.RbacV1().RoleBindings(s.k8sclient.namespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "rb-other-model",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations("different-model"),
		},
		RoleRef:  rbRef,
		Subjects: rbSubjects,
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, _, err = s.k8sclient.createRoleBinding(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{Name: "rb-other-model"},
		RoleRef:    rbRef,
		Subjects:   rbSubjects,
	})
	c.Assert(err, gc.ErrorMatches, `role binding "rb-other-model" exists and is not managed by this model`)

	_, err = s.k8sclient.client.RbacV1().RoleBindings(s.k8sclient.namespace).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "rb-bindings-change",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations(s.k8sclient.modelUUID),
		},
		RoleRef:  rbRef,
		Subjects: rbSubjects,
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, _, err = s.k8sclient.createRoleBinding(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{Name: "rb-bindings-change"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "different-role"},
		Subjects:   rbSubjects,
	})
	c.Assert(errors.Is(err, errors.NotSupported), gc.Equals, true)
	c.Assert(err, gc.ErrorMatches, `.*changing bindings for role binding "rb-bindings-change" not supported`)
}

func (s *rbacSuite) TestCreateClusterRoleErrors(c *gc.C) {
	ctx := context.Background()

	_, err := s.k8sclient.client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:   "cr-not-juju",
			Labels: map[string]string{},
		},
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createClusterRole(ctx, &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{Name: "cr-not-juju"},
	})
	c.Assert(err, gc.ErrorMatches, `cluster role "cr-not-juju" exists and is not managed by Juju`)

	_, err = s.k8sclient.client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:        "cr-other-model",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations("different-model"),
		},
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createClusterRole(ctx, &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{Name: "cr-other-model"},
	})
	c.Assert(err, gc.ErrorMatches, `cluster role "cr-other-model" exists and is not managed by this model`)

	_, err = s.k8sclient.client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:        "cr-rules-change",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations(s.k8sclient.modelUUID),
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
			Verbs:     []string{"get"},
		}},
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createClusterRole(ctx, &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{Name: "cr-rules-change"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list"},
		}},
	})
	c.Assert(errors.Is(err, errors.NotSupported), gc.Equals, true)
	c.Assert(err, gc.ErrorMatches, `.*changing rules for cluster role "cr-rules-change" not supported`)
}

func (s *rbacSuite) TestCreateClusterRoleBindingErrors(c *gc.C) {
	ctx := context.Background()

	crbRef := rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "a-cluster-role"}
	crbSubjects := []rbacv1.Subject{{Kind: "ServiceAccount", Name: "sa", Namespace: s.k8sclient.namespace}}

	_, err := s.k8sclient.client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:   "crb-not-juju",
			Labels: map[string]string{},
		},
		RoleRef:  crbRef,
		Subjects: crbSubjects,
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createClusterRoleBinding(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{Name: "crb-not-juju"},
		RoleRef:    crbRef,
		Subjects:   crbSubjects,
	})
	c.Assert(err, gc.ErrorMatches, `cluster role binding "crb-not-juju" exists and is not managed by Juju`)

	_, err = s.k8sclient.client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "crb-other-model",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations("different-model"),
		},
		RoleRef:  crbRef,
		Subjects: crbSubjects,
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createClusterRoleBinding(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{Name: "crb-other-model"},
		RoleRef:    crbRef,
		Subjects:   crbSubjects,
	})
	c.Assert(err, gc.ErrorMatches, `cluster role binding "crb-other-model" exists and is not managed by this model`)

	_, err = s.k8sclient.client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "crb-bindings-change",
			Labels:      s.managedLabels(),
			Annotations: s.modelAnnotations(s.k8sclient.modelUUID),
		},
		RoleRef:  crbRef,
		Subjects: crbSubjects,
	}, v1.CreateOptions{FieldManager: resources.JujuFieldManager})
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.k8sclient.createClusterRoleBinding(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{Name: "crb-bindings-change"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "different-cluster-role"},
		Subjects:   crbSubjects,
	})
	c.Assert(errors.Is(err, errors.NotSupported), gc.Equals, true)
	c.Assert(err, gc.ErrorMatches, `.*changing bindings for cluster role binding "crb-bindings-change" not supported`)
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
