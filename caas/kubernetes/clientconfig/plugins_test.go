// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
)

type K8sRawClientSuite struct {
	BaseSuite
}

var _ = gc.Suite(&K8sRawClientSuite{})

func (s *K8sRawClientSuite) TestEnsureClusterRole(c *gc.C) {
	crName := "juju-admin-cluster-role"
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: s.namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{rbacv1.APIGroupAll},
				Resources: []string{rbacv1.ResourceAll},
				Verbs:     []string{rbacv1.VerbAll},
			},
			{
				NonResourceURLs: []string{rbacv1.NonResourceAll},
				Verbs:           []string{rbacv1.VerbAll},
			},
		},
	}

	gomock.InOrder(
		s.mockClusterRoles.EXPECT().Get(crName, metav1.GetOptions{}).Times(1).
			Return(cr, nil),
	)
	crOut, err := clientconfig.EnsureClusterRole(s.k8sClient, crName, s.namespace)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crOut, jc.DeepEquals, cr)

	gomock.InOrder(
		s.mockClusterRoles.EXPECT().Get(crName, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoles.EXPECT().Create(cr).Times(1).
			Return(cr, nil),
	)
	crOut, err = clientconfig.EnsureClusterRole(s.k8sClient, crName, s.namespace)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crOut, jc.DeepEquals, cr)

}

func (s *K8sRawClientSuite) TestEnsureServiceAccount(c *gc.C) {
	saName := "juju-admin-sa"
	sa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: s.namespace,
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(sa).Times(1).
			Return(sa, nil),
		s.mockServiceAccounts.EXPECT().Get(saName, metav1.GetOptions{}).Times(1).
			Return(sa, nil),
	)
	saOut, err := clientconfig.EnsureServiceAccount(s.k8sClient, saName, s.namespace)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saOut, jc.DeepEquals, sa)

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(sa).Times(1).
			Return(sa, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().Get(saName, metav1.GetOptions{}).Times(1).
			Return(sa, nil),
	)
	saOut, err = clientconfig.EnsureServiceAccount(s.k8sClient, saName, s.namespace)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saOut, jc.DeepEquals, sa)
}

func (s *K8sRawClientSuite) TestEnsureClusterRoleBinding(c *gc.C) {
	saName := "juju-admin-sa"
	sa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: s.namespace,
		},
	}

	crName := "juju-admin-cluster-role"
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: s.namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{rbacv1.APIGroupAll},
				Resources: []string{rbacv1.ResourceAll},
				Verbs:     []string{rbacv1.VerbAll},
			},
			{
				NonResourceURLs: []string{rbacv1.NonResourceAll},
				Verbs:           []string{rbacv1.VerbAll},
			},
		},
	}

	clusterRoleBindingName := "juju-admin-cluster-role-binding"
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: cr.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}

	gomock.InOrder(
		s.mockClusterRoleBinding.EXPECT().Create(clusterRoleBinding).Times(1).
			Return(clusterRoleBinding, nil),
	)
	clusterRoleBindingOut, err := clientconfig.EnsureClusterRoleBinding(s.k8sClient, clusterRoleBindingName, sa, cr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRoleBindingOut, jc.DeepEquals, clusterRoleBinding)

	gomock.InOrder(
		s.mockClusterRoleBinding.EXPECT().Create(clusterRoleBinding).Times(1).
			Return(clusterRoleBinding, s.k8sAlreadyExistsError()),
	)
	clusterRoleBindingOut, err = clientconfig.EnsureClusterRoleBinding(s.k8sClient, clusterRoleBindingName, sa, cr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRoleBindingOut, jc.DeepEquals, clusterRoleBinding)
}
