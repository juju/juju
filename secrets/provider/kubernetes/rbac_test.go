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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

func (s *rbacSuite) TestEnsureRoleBinding(c *gc.C) {
	ctx := context.Background()
	rbName := "rb-name"
	// Check that role binding does not exist initially.
	_, err := s.k8sclient.client.RbacV1().RoleBindings(s.k8sclient.namespace).Get(ctx, rbName, v1.GetOptions{})
	c.Assert(k8serrors.IsNotFound(err), gc.Equals, true)

	// Ensure role binding should create the role binding.
	rb, cleanupsForCreate, err := s.k8sclient.ensureRoleBinding(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      rbName,
			Namespace: s.k8sclient.namespace,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"hello": "world",
				"fred":  "mary",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cleanupsForCreate, gc.HasLen, 1)
	c.Assert(rb.Name, gc.Equals, rbName)

	// Get the role binding to check it was created correctly.
	res, err := s.k8sclient.client.RbacV1().RoleBindings(s.k8sclient.namespace).Get(ctx, rbName, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Name, gc.Equals, rbName)
	c.Assert(res.Labels, gc.DeepEquals, map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"})
	c.Assert(res.Annotations, gc.DeepEquals, map[string]string{"hello": "world", "fred": "mary"})

	// Ensure role binding should get the current role binding with no cleanups if it already exists.
	rb, cleanupsForUpdate, err := s.k8sclient.ensureRoleBinding(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name: rbName,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rb.Name, gc.Equals, rbName)
	c.Assert(cleanupsForUpdate, gc.HasLen, 0)
	c.Assert(rb.Labels, gc.DeepEquals, map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"})
	c.Assert(rb.Annotations, gc.DeepEquals, map[string]string{"hello": "world", "fred": "mary"})

	// Run cleanups and verify resources are removed.
	for _, fn := range cleanupsForCreate {
		fn()
	}
	_, err = s.k8sclient.client.RbacV1().RoleBindings(s.k8sclient.namespace).Get(ctx, rbName, v1.GetOptions{})
	c.Assert(k8serrors.IsNotFound(err), gc.Equals, true)
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

func (s *rbacSuite) makeSA(ns, name string, lbls, ann map[string]string) *core.ServiceAccount {
	return &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Labels:      lbls,
			Annotations: ann,
		},
	}
}

func (s *rbacSuite) TestEnsureBinding_CreateRoleAndCreateBinding(c *gc.C) {
	ctx := context.Background()
	ns := s.k8sclient.namespace
	sa := s.makeSA(ns, "sa",
		map[string]string{"app.kubernetes.io/managed-by": "juju"},
		map[string]string{"foo": "bar"},
	)

	owned := []string{"sec-owned-a"}
	read := []string{"sec-read-b"}
	removed := []string{"sec-removed-c"}

	cleanups, err := s.k8sclient.ensureBindingForSecretAccessToken(ctx, sa, owned, read, removed)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cleanups, gc.HasLen, 2)

	// Check if role was created with correct metadata and rules.
	role, err := s.k8sclient.client.RbacV1().Roles(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role.Labels, gc.DeepEquals, sa.Labels)
	c.Assert(role.Annotations, gc.DeepEquals, sa.Annotations)
	c.Assert(role.Rules, gc.DeepEquals, rulesForSecretAccess(ns, false, nil, owned, read, removed))

	// Check if rolebinding was created.
	rb, err := s.k8sclient.client.RbacV1().RoleBindings(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rb.RoleRef.Kind, gc.Equals, "Role")
	c.Assert(rb.RoleRef.Name, gc.Equals, sa.Name)
	c.Assert(rb.Subjects, gc.HasLen, 1)
	c.Assert(rb.Subjects[0].Kind, gc.Equals, "ServiceAccount")
	c.Assert(rb.Subjects[0].Name, gc.Equals, sa.Name)
	c.Assert(rb.Subjects[0].Namespace, gc.Equals, ns)

	// Run cleanups and verify resources are removed correctly.
	for _, fn := range cleanups {
		fn()
	}
	_, err = s.k8sclient.client.RbacV1().Roles(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(k8serrors.IsNotFound(err), gc.Equals, true)
	_, err = s.k8sclient.client.RbacV1().RoleBindings(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(k8serrors.IsNotFound(err), gc.Equals, true)
}

func (s *rbacSuite) TestEnsureBinding_UpdateRoleAndCreateBinding(c *gc.C) {
	ctx := context.Background()
	ns := s.k8sclient.namespace
	sa := s.makeSA(ns, "sa", map[string]string{"x": "y"}, nil)

	// Pre-create Role with some existing rules to be updated.
	existing := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{Name: sa.Name, Namespace: ns, Labels: sa.Labels},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"list"},
		}},
	}
	_, err := s.k8sclient.client.RbacV1().Roles(ns).Create(ctx, existing, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	owned := []string{"o1"}
	read := []string{"r1", "r2"}
	removed := []string{"z1"}

	cleanups, err := s.k8sclient.ensureBindingForSecretAccessToken(ctx, sa, owned, read, removed)
	c.Assert(err, jc.ErrorIsNil)

	// Role existed -> no role cleanup
	// Rolebinding newly created -> 1 rolebinding cleanup
	c.Assert(cleanups, gc.HasLen, 1)

	// Role rules updated.
	updatedRules, err := s.k8sclient.client.RbacV1().Roles(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedRules.Rules, gc.DeepEquals, rulesForSecretAccess(ns, false, existing.Rules, owned, read, removed))

	// Rolebinding created.
	_, err = s.k8sclient.client.RbacV1().RoleBindings(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *rbacSuite) TestEnsureBinding_UpdateRoleAndNotCreateBinding(c *gc.C) {
	ctx := context.Background()
	ns := s.k8sclient.namespace
	sa := s.makeSA(ns, "sa",
		map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "test"},
		map[string]string{"hello": "world"},
	)
	// Pre-create Role.
	existingRole, err := s.k8sclient.client.RbacV1().Roles(ns).Create(ctx, &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{Name: sa.Name, Namespace: ns},
	}, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	// Pre-create RoleBinding.
	_, err = s.k8sclient.client.RbacV1().RoleBindings(ns).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{Name: sa.Name, Namespace: ns},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: sa.Name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: sa.Name, Namespace: ns}},
	}, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	owned := []string{"o1"}
	read := []string{"r1", "r2"}
	removed := []string{"z1"}

	cleanups, err := s.k8sclient.ensureBindingForSecretAccessToken(ctx, sa, owned, read, removed)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cleanups, gc.HasLen, 0)

	// Check role was indeed updated.
	updatedRole, err := s.k8sclient.client.RbacV1().Roles(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedRole.Rules, gc.DeepEquals, rulesForSecretAccess(ns, false, existingRole.Rules, owned, read, removed))

	// Check rolebinding was indeed created.
	_, err = s.k8sclient.client.RbacV1().RoleBindings(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *rbacSuite) TestEnsureBinding_CreateRoleAndNotCreateBinding(c *gc.C) {
	ctx := context.Background()
	ns := s.k8sclient.namespace
	sa := s.makeSA(ns, "sa",
		map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "test"},
		map[string]string{"hello": "world"},
	)

	// Pre-create RoleBinding.
	_, err := s.k8sclient.client.RbacV1().RoleBindings(ns).Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{Name: sa.Name, Namespace: ns},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: sa.Name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: sa.Name, Namespace: ns}},
	}, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	owned := []string{"o1"}
	read := []string{"r1", "r2"}
	removed := []string{"z1"}

	cleanups, err := s.k8sclient.ensureBindingForSecretAccessToken(ctx, sa, owned, read, removed)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cleanups, gc.HasLen, 1)

	// Check that role was created with correct metadata and rules.
	role, err := s.k8sclient.client.RbacV1().Roles(ns).Get(ctx, sa.Name, v1.GetOptions{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role.Labels, gc.DeepEquals, sa.Labels)
	c.Assert(role.Annotations, gc.DeepEquals, sa.Annotations)
	c.Assert(role.Rules, gc.DeepEquals, rulesForSecretAccess(ns, false, nil, owned, read, removed))
}
