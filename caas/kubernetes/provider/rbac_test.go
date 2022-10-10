// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider"
)

var _ = gc.Suite(&rbacSuite{})

type rbacSuite struct {
	BaseSuite
}

func (s *rbacSuite) TestEnsureSecretAccessTokenCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	tag := names.NewUnitTag("gitlab/0")

	objMeta := v1.ObjectMeta{
		Name:      tag.String(),
		Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "gitlab"},
		Namespace: s.namespace,
	}
	automountServiceAccountToken := true
	sa := &core.ServiceAccount{
		ObjectMeta:                   objMeta,
		AutomountServiceAccountToken: &automountServiceAccountToken,
	}
	role := &rbacv1.Role{
		ObjectMeta: objMeta,
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:         []string{"get", "list"},
				APIGroups:     []string{"*"},
				Resources:     []string{"namespaces"},
				ResourceNames: []string{"test"},
			},
			{
				Verbs:     []string{"create", "patch"},
				APIGroups: []string{"*"},
				Resources: []string{"secrets"},
			},
		},
	}
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: objMeta,
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}
	expiresInSeconds := int64(60 * 10)
	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiresInSeconds,
		},
	}
	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{}).
			Return(sa, nil),
		s.mockRoles.EXPECT().Get(gomock.Any(), "unit-gitlab-0", v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockRoles.EXPECT().Create(gomock.Any(), role, v1.CreateOptions{}).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=gitlab",
		}).Return(&rbacv1.RoleBindingList{}, nil),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), roleBinding, v1.CreateOptions{}).Return(roleBinding, nil),
		s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), "unit-gitlab-0", treq, v1.CreateOptions{}).Return(
			&authenticationv1.TokenRequest{Status: authenticationv1.TokenRequestStatus{Token: "token"}}, nil,
		),
	)

	token, err := s.broker.EnsureSecretAccessToken(tag, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Equals, "token")
}

func (s *rbacSuite) TestEnsureSecretAccessTokeUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	tag := names.NewUnitTag("gitlab/0")

	objMeta := v1.ObjectMeta{
		Name:      tag.String(),
		Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "gitlab"},
		Namespace: s.namespace,
	}
	automountServiceAccountToken := true
	sa := &core.ServiceAccount{
		ObjectMeta:                   objMeta,
		AutomountServiceAccountToken: &automountServiceAccountToken,
	}
	role := &rbacv1.Role{
		ObjectMeta: objMeta,
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:         []string{"get", "list"},
				APIGroups:     []string{"*"},
				Resources:     []string{"namespaces"},
				ResourceNames: []string{"test"},
			},
			{
				Verbs:     []string{"create", "patch"},
				APIGroups: []string{"*"},
				Resources: []string{"secrets"},
			},
		},
	}
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: objMeta,
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}
	expiresInSeconds := int64(60 * 10)
	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiresInSeconds,
		},
	}
	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, v1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=gitlab",
		}).Return(&core.ServiceAccountList{Items: []core.ServiceAccount{*sa}}, nil),
		s.mockServiceAccounts.EXPECT().Update(gomock.Any(), sa, v1.UpdateOptions{}).
			Return(sa, nil),
		s.mockRoles.EXPECT().Get(gomock.Any(), "unit-gitlab-0", v1.GetOptions{}).Return(role, nil),
		s.mockRoles.EXPECT().Update(gomock.Any(), role, v1.UpdateOptions{}).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(gomock.Any(), v1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=gitlab",
		}).Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{*roleBinding}}, nil),
		s.mockRoleBindings.EXPECT().Delete(gomock.Any(), "unit-gitlab-0", s.deleteOptions(v1.DeletePropagationForeground, "")).Return(nil),
		s.mockRoleBindings.EXPECT().Get(gomock.Any(), "unit-gitlab-0", v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(gomock.Any(), roleBinding, v1.CreateOptions{}).Return(roleBinding, nil),
		s.mockServiceAccounts.EXPECT().CreateToken(gomock.Any(), "unit-gitlab-0", treq, v1.CreateOptions{}).Return(
			&authenticationv1.TokenRequest{Status: authenticationv1.TokenRequestStatus{Token: "token"}}, nil,
		),
	)

	token, err := s.broker.EnsureSecretAccessToken(tag, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Equals, "token")
}

func (s *rbacSuite) TestRulesForSecretAccessNew(c *gc.C) {
	owned := []string{"owned-secret-1"}
	read := []string{"read-secret-1"}
	newPolicies := provider.RulesForSecretAccess("test", nil, owned, read, nil)
	c.Assert(newPolicies, gc.DeepEquals, []rbacv1.PolicyRule{
		{
			Verbs:         []string{"get", "list"},
			APIGroups:     []string{"*"},
			Resources:     []string{"namespaces"},
			ResourceNames: []string{"test"},
		},
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
	})
}

func (s *rbacSuite) TestRulesForSecretAccessUpdate(c *gc.C) {
	existing := []rbacv1.PolicyRule{
		{
			Verbs:         []string{"get", "list"},
			APIGroups:     []string{"*"},
			Resources:     []string{"namespaces"},
			ResourceNames: []string{"test"},
		},
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"removed-owned-secret"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"removed-read-secret"},
		},
	}

	owned := []string{"owned-secret-1", "owned-secret-2"}
	read := []string{"read-secret-1", "read-secret-2"}
	removed := []string{"removed-owned-secret", "removed-read-secret"}

	newPolicies := provider.RulesForSecretAccess("test", existing, owned, read, removed)
	c.Assert(newPolicies, gc.DeepEquals, []rbacv1.PolicyRule{
		{
			Verbs:         []string{"get", "list"},
			APIGroups:     []string{"*"},
			Resources:     []string{"namespaces"},
			ResourceNames: []string{"test"},
		},
		{
			Verbs:     []string{"create", "patch"},
			APIGroups: []string{"*"},
			Resources: []string{"secrets"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-1"},
		},
		{
			Verbs:         []string{"*"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"owned-secret-2"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-1"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"*"},
			Resources:     []string{"secrets"},
			ResourceNames: []string{"read-secret-2"},
		},
	})
}
