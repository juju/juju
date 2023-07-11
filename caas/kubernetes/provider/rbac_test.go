// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&rbacSuite{})

type rbacSuite struct {
	BaseSuite
}

func (s *rbacSuite) TestEnsureRoleBinding(c *gc.C) {
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

	_, _, err := s.broker.EnsureRoleBinding(rb1)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.broker.EnsureRoleBinding(rb1)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.broker.EnsureRoleBinding(&rb2)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.broker.EnsureRoleBinding(&rb2)
	c.Assert(err, jc.ErrorIsNil)
}
