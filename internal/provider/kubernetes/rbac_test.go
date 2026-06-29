// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/testing"
)

func TestRbacSuite(t *stdtesting.T) {
	tc.Run(t, &rbacSuite{})
}

type rbacSuite struct {
	BaseSuite
}

func (s *rbacSuite) k8sAlreadyExistsError() *k8serrors.StatusError {
	return k8serrors.NewAlreadyExists(schema.GroupResource{}, "test")
}

func (s *rbacSuite) TestEnsureRoleBinding(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	rb1 := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "rb-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "role-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa1",
				Namespace: "test",
			},
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa2",
				Namespace: "test",
			},
		},
	}
	rb1SubjectsInDifferentOrder := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "rb-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "role-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa2",
				Namespace: "test",
			},
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa1",
				Namespace: "test",
			},
		},
	}
	rb2 := rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "rb-name",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"fred":                  "mary",
				"controller.juju.is/id": testing.ControllerTag.Id(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "role-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "sa2",
				Namespace: "test",
			},
		},
	}
	rb2DifferentSubjects := rb2
	rb2DifferentSubjects.Subjects = []rbacv1.Subject{
		{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      "sa3",
			Namespace: "test",
		},
	}
	rbUID := rb2DifferentSubjects.GetUID()
	gomock.InOrder(
		// Already exists, no change.
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "rb-name", v1.GetOptions{}).
			Return(rb1, nil),

		// Already exists, but with same subjects in different order.
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "rb-name", v1.GetOptions{}).
			Return(rb1SubjectsInDifferentOrder, nil),

		// No existing role binding, create one.
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "rb-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), &rb2, v1.CreateOptions{}).Return(&rb2, nil),

		// Already exists, but with different subjects, delete and create.
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "rb-name", v1.GetOptions{}).
			Return(&rb2DifferentSubjects, nil),
		s.mockRoleBindings.EXPECT().Delete(gomock.Any(), "rb-name", s.deleteOptions(v1.DeletePropagationForeground, rbUID)).Return(nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "rb-name", v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), &rb2, v1.CreateOptions{}).Return(&rb2, nil),
	)

	_, _, err := s.broker.EnsureRoleBinding(c.Context(), rb1)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.broker.EnsureRoleBinding(c.Context(), rb1)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.broker.EnsureRoleBinding(c.Context(), &rb2)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.broker.EnsureRoleBinding(c.Context(), &rb2)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *rbacSuite) TestEnsureServiceAccountModelOperatorV2Labels(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Labels that MatchModelOperatorMetaLabelVersion expects for v2:
	// LabelsForModelOperator(LabelVersion2) merged with LabelsJuju.
	v2Labels := map[string]string{
		k8sconstants.LabelJujuOperatorName:     k8sconstants.ModelOperatorName,
		k8sconstants.LabelJujuOperatorTarget:   k8sconstants.ModelOperatorTargetValue,
		k8sconstants.LabelKubernetesAppManaged: "juju",
	}

	sa := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    v2Labels,
		},
	}

	existingSA := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    v2Labels,
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), k8sconstants.ModelOperatorName, v1.GetOptions{}).
			Return(existingSA, nil),
		s.mockServiceAccounts.EXPECT().Update(gomock.Any(), sa, v1.UpdateOptions{}).
			Return(sa, nil),
	)

	out, _, err := s.broker.EnsureServiceAccount(c.Context(), sa)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(out.GetName(), tc.Equals, k8sconstants.ModelOperatorName)
}

func (s *rbacSuite) TestEnsureServiceAccountModelOperatorLegacyLabels(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Legacy labels: only LegacyLabelModelOperator.
	legacyLabels := map[string]string{
		k8sconstants.LegacyLabelModelOperator: k8sconstants.ModelOperatorName,
	}

	sa := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    legacyLabels,
		},
	}

	existingSA := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    legacyLabels,
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), k8sconstants.ModelOperatorName, v1.GetOptions{}).
			Return(existingSA, nil),
		s.mockServiceAccounts.EXPECT().Update(gomock.Any(), sa, v1.UpdateOptions{}).
			Return(sa, nil),
	)

	out, _, err := s.broker.EnsureServiceAccount(c.Context(), sa)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(out.GetName(), tc.Equals, k8sconstants.ModelOperatorName)
}

func (s *rbacSuite) TestEnsureServiceAccountModelOperatorUnexpectedLabels(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	sa := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    map[string]string{"random": "label"},
		},
	}

	existingSA := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    map[string]string{"random": "label"},
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), k8sconstants.ModelOperatorName, v1.GetOptions{}).
			Return(existingSA, nil),
	)

	_, _, err := s.broker.EnsureServiceAccount(c.Context(), sa)
	c.Assert(err, tc.ErrorMatches, `.*unexpected operator labels.*`)
}

func (s *rbacSuite) TestEnsureServiceAccountExecRBACV2Labels(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	v2Labels := map[string]string{
		k8sconstants.LabelJujuOperatorName:     k8sconstants.ModelOperatorName,
		k8sconstants.LabelJujuOperatorTarget:   k8sconstants.ModelOperatorTargetValue,
		k8sconstants.LabelKubernetesAppManaged: "juju",
	}

	sa := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "model-exec",
			Namespace: "test",
			Labels:    v2Labels,
		},
	}

	existingSA := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "model-exec",
			Namespace: "test",
			Labels:    v2Labels,
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), "model-exec", v1.GetOptions{}).
			Return(existingSA, nil),
		s.mockServiceAccounts.EXPECT().Update(gomock.Any(), sa, v1.UpdateOptions{}).
			Return(sa, nil),
	)

	out, _, err := s.broker.EnsureServiceAccount(c.Context(), sa)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(out.GetName(), tc.Equals, "model-exec")
}

func (s *rbacSuite) TestEnsureRoleModelOperatorV2Labels(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	v2Labels := map[string]string{
		k8sconstants.LabelJujuOperatorName:     k8sconstants.ModelOperatorName,
		k8sconstants.LabelJujuOperatorTarget:   k8sconstants.ModelOperatorTargetValue,
		k8sconstants.LabelKubernetesAppManaged: "juju",
	}

	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    v2Labels,
		},
	}

	existingRole := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    v2Labels,
		},
	}

	gomock.InOrder(
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockRoles.EXPECT().Get(gomock.Any(), k8sconstants.ModelOperatorName, v1.GetOptions{}).
			Return(existingRole, nil),
		s.mockRoles.EXPECT().Update(gomock.Any(), role, v1.UpdateOptions{}).
			Return(role, nil),
	)

	out, _, err := s.broker.EnsureRole(c.Context(), role)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(out.GetName(), tc.Equals, k8sconstants.ModelOperatorName)
}

func (s *rbacSuite) TestEnsureRoleModelOperatorLegacyLabels(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	legacyLabels := map[string]string{
		k8sconstants.LegacyLabelModelOperator: k8sconstants.ModelOperatorName,
	}

	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    legacyLabels,
		},
	}

	existingRole := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    legacyLabels,
		},
	}

	gomock.InOrder(
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockRoles.EXPECT().Get(gomock.Any(), k8sconstants.ModelOperatorName, v1.GetOptions{}).
			Return(existingRole, nil),
		s.mockRoles.EXPECT().Update(gomock.Any(), role, v1.UpdateOptions{}).
			Return(role, nil),
	)

	out, _, err := s.broker.EnsureRole(c.Context(), role)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(out.GetName(), tc.Equals, k8sconstants.ModelOperatorName)
}

func (s *rbacSuite) TestEnsureRoleModelOperatorUnexpectedLabels(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    map[string]string{"random": "label"},
		},
	}

	existingRole := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      k8sconstants.ModelOperatorName,
			Namespace: "test",
			Labels:    map[string]string{"random": "label"},
		},
	}

	gomock.InOrder(
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockRoles.EXPECT().Get(gomock.Any(), k8sconstants.ModelOperatorName, v1.GetOptions{}).
			Return(existingRole, nil),
	)

	_, _, err := s.broker.EnsureRole(c.Context(), role)
	c.Assert(err, tc.ErrorMatches, `.*unexpected operator labels.*`)
}

func (s *rbacSuite) TestEnsureRoleExecRBACV2Labels(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	v2Labels := map[string]string{
		k8sconstants.LabelJujuOperatorName:     k8sconstants.ModelOperatorName,
		k8sconstants.LabelJujuOperatorTarget:   k8sconstants.ModelOperatorTargetValue,
		k8sconstants.LabelKubernetesAppManaged: "juju",
	}

	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "model-exec",
			Namespace: "test",
			Labels:    v2Labels,
		},
	}

	existingRole := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "model-exec",
			Namespace: "test",
			Labels:    v2Labels,
		},
	}

	gomock.InOrder(
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockRoles.EXPECT().Get(gomock.Any(), "model-exec", v1.GetOptions{}).
			Return(existingRole, nil),
		s.mockRoles.EXPECT().Update(gomock.Any(), role, v1.UpdateOptions{}).
			Return(role, nil),
	)

	out, _, err := s.broker.EnsureRole(c.Context(), role)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(out.GetName(), tc.Equals, "model-exec")
}
