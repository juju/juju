// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig_test

import (
	"reflect"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
)

type k8sRawClientSuite struct {
	BaseSuite
}

var _ = gc.Suite(&k8sRawClientSuite{})

func (s *k8sRawClientSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.namespace = "kube-system"
}

func (s *k8sRawClientSuite) TestEnsureJujuAdminRBACResources(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cfg := newClientConfig()
	contextName := reflect.ValueOf(cfg.Contexts).MapKeys()[0].Interface().(string)

	secret := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-sa-secret",
			Namespace: s.namespace,
		},
		Data: map[string][]byte{
			"ca.crt":    []byte("a base64 encoded cert"),
			"namespace": []byte("base64 encoded namespace"),
			"token":     []byte("a base64 encoded bearer token"),
		},
	}

	saName := "juju-service-account"
	newSa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: s.namespace,
		},
	}
	saWithSecret := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: s.namespace,
		},
		Secrets: []core.ObjectReference{
			{
				Kind: "Secret",
				Name: secret.Name,
			},
		},
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-admin",
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

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "juju-cluster-role-binding",
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: cr.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      saName,
				Namespace: s.namespace,
			},
		},
	}

	// 1st call of ensuring related resources - CREATE.
	gomock.InOrder(
		s.mockClusterRoles.EXPECT().Get(cr.Name, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoles.EXPECT().Create(cr).Times(1).
			Return(cr, nil),
		s.mockServiceAccounts.EXPECT().Create(newSa).Times(1).
			Return(newSa, nil),
		s.mockServiceAccounts.EXPECT().Get(newSa.Name, metav1.GetOptions{}).Times(1).
			Return(newSa, nil),
		s.mockClusterRoleBinding.EXPECT().Create(clusterRoleBinding).Times(1).
			Return(clusterRoleBinding, nil),
		s.mockServiceAccounts.EXPECT().Get(newSa.Name, metav1.GetOptions{}).Times(1).
			Return(saWithSecret, nil),
		s.mockSecrets.EXPECT().Get(saWithSecret.Secrets[0].Name, metav1.GetOptions{}).Times(1).
			Return(secret, nil),
	)
	cfgOut, err := clientconfig.EnsureJujuAdminRBACResources(s.k8sClient, cfg, contextName)
	c.Assert(err, jc.ErrorIsNil)
	authName := cfg.Contexts[contextName].AuthInfo
	updatedAuthInfo := cfgOut.AuthInfos[authName]
	c.Assert(updatedAuthInfo.AuthProvider, gc.IsNil)
	c.Assert(updatedAuthInfo.ClientCertificateData, gc.DeepEquals, secret.Data[core.ServiceAccountRootCAKey])
	c.Assert(updatedAuthInfo.Token, gc.Equals, string(secret.Data[core.ServiceAccountTokenKey]))

}

func (s *k8sRawClientSuite) TestEnsureJujuServiceAdminAccountIdempotent(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cfg := newClientConfig()
	contextName := reflect.ValueOf(cfg.Contexts).MapKeys()[0].Interface().(string)

	secret := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-sa-secret",
			Namespace: s.namespace,
		},
		Data: map[string][]byte{
			"ca.crt":    []byte("a base64 encoded cert"),
			"namespace": []byte("base64 encoded namespace"),
			"token":     []byte("a base64 encoded bearer token"),
		},
	}

	saName := "juju-service-account"
	newSa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: s.namespace,
		},
	}
	saWithSecret := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: s.namespace,
		},
		Secrets: []core.ObjectReference{
			{
				Kind: "Secret",
				Name: secret.Name,
			},
		},
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-admin",
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

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "juju-cluster-role-binding",
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: cr.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      saName,
				Namespace: s.namespace,
			},
		},
	}

	// 2nd call of ensuring related resources - GET.
	gomock.InOrder(
		s.mockClusterRoles.EXPECT().Get(cr.Name, metav1.GetOptions{}).Times(1).
			Return(cr, nil),
		s.mockServiceAccounts.EXPECT().Create(newSa).Times(1).
			Return(newSa, nil),
		s.mockServiceAccounts.EXPECT().Get(newSa.Name, metav1.GetOptions{}).Times(1).
			Return(newSa, nil),
		s.mockClusterRoleBinding.EXPECT().Create(clusterRoleBinding).Times(1).
			Return(clusterRoleBinding, nil),
		s.mockServiceAccounts.EXPECT().Get(newSa.Name, metav1.GetOptions{}).Times(1).
			Return(saWithSecret, nil),
		s.mockSecrets.EXPECT().Get(saWithSecret.Secrets[0].Name, metav1.GetOptions{}).Times(1).
			Return(secret, nil),
	)
	cfgOut, err := clientconfig.EnsureJujuAdminRBACResources(s.k8sClient, cfg, contextName)
	c.Assert(err, jc.ErrorIsNil)
	authName := cfg.Contexts[contextName].AuthInfo
	updatedAuthInfo := cfgOut.AuthInfos[authName]
	c.Assert(updatedAuthInfo.AuthProvider, gc.IsNil)
	c.Assert(updatedAuthInfo.ClientCertificateData, gc.DeepEquals, secret.Data[core.ServiceAccountRootCAKey])
	c.Assert(updatedAuthInfo.Token, gc.Equals, string(secret.Data[core.ServiceAccountTokenKey]))

}

func (s *k8sRawClientSuite) TestEnsureClusterRole(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-admin-cluster-role",
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
		s.mockClusterRoles.EXPECT().Get(cr.Name, metav1.GetOptions{}).Times(1).
			Return(cr, nil),
	)
	crOut, err := clientconfig.EnsureClusterRole(s.k8sClient, cr.Name, s.namespace)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crOut, jc.DeepEquals, cr)

	gomock.InOrder(
		s.mockClusterRoles.EXPECT().Get(cr.Name, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoles.EXPECT().Create(cr).Times(1).
			Return(cr, nil),
	)
	crOut, err = clientconfig.EnsureClusterRole(s.k8sClient, cr.Name, s.namespace)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crOut, jc.DeepEquals, cr)

}

func (s *k8sRawClientSuite) TestEnsureServiceAccount(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	sa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-admin-sa",
			Namespace: s.namespace,
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(sa).Times(1).
			Return(sa, nil),
		s.mockServiceAccounts.EXPECT().Get(sa.Name, metav1.GetOptions{}).Times(1).
			Return(sa, nil),
	)
	saOut, err := clientconfig.EnsureServiceAccount(s.k8sClient, sa.Name, s.namespace)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saOut, jc.DeepEquals, sa)

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Create(sa).Times(1).
			Return(sa, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().Get(sa.Name, metav1.GetOptions{}).Times(1).
			Return(sa, nil),
	)
	saOut, err = clientconfig.EnsureServiceAccount(s.k8sClient, sa.Name, s.namespace)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saOut, jc.DeepEquals, sa)
}

func (s *k8sRawClientSuite) TestEnsureClusterRoleBinding(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	sa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-admin-sa",
			Namespace: s.namespace,
		},
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-admin-cluster-role",
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

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "juju-admin-cluster-role-binding",
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
	clusterRoleBindingOut, err := clientconfig.EnsureClusterRoleBinding(s.k8sClient, clusterRoleBinding.Name, sa, cr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRoleBindingOut, jc.DeepEquals, clusterRoleBinding)

	gomock.InOrder(
		s.mockClusterRoleBinding.EXPECT().Create(clusterRoleBinding).Times(1).
			Return(clusterRoleBinding, s.k8sAlreadyExistsError()),
	)
	clusterRoleBindingOut, err = clientconfig.EnsureClusterRoleBinding(s.k8sClient, clusterRoleBinding.Name, sa, cr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clusterRoleBindingOut, jc.DeepEquals, clusterRoleBinding)
}

func (s *k8sRawClientSuite) TestGetServiceAccountSecret(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	secret := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-sa-secret",
			Namespace: s.namespace,
		},
		Data: map[string][]byte{
			"ca.crt":    []byte("a base64 encoded cert"),
			"namespace": []byte("base64 encoded namespace"),
			"token":     []byte("a base64 encoded bearer token"),
		},
	}
	sa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-admin-sa",
			Namespace: s.namespace,
		},
		Secrets: []core.ObjectReference{
			{
				Kind: "Secret",
				Name: secret.Name,
			},
		},
	}

	gomock.InOrder(
		s.mockSecrets.EXPECT().Get(sa.Secrets[0].Name, metav1.GetOptions{}).Times(1).
			Return(secret, nil),
	)
	secretOut, err := clientconfig.GetServiceAccountSecret(s.k8sClient, sa)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(secretOut, jc.DeepEquals, secret)
}

func (s *k8sRawClientSuite) TestReplaceAuthProviderWithServiceAccountAuthData(c *gc.C) {
	cfg := newClientConfig()
	contextName := reflect.ValueOf(cfg.Contexts).MapKeys()[0].Interface().(string)
	authName := cfg.Contexts[contextName].AuthInfo

	secret := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "juju-sa-secret",
			Namespace: s.namespace,
		},
		Data: map[string][]byte{
			"ca.crt":    []byte("a base64 encoded cert"),
			"namespace": []byte("base64 encoded namespace"),
			"token":     []byte("a base64 encoded bearer token"),
		},
	}
	clientconfig.ReplaceAuthProviderWithServiceAccountAuthData(contextName, cfg, secret)
	updatedAuthInfo := cfg.AuthInfos[authName]
	c.Assert(updatedAuthInfo.AuthProvider, gc.IsNil)
	c.Assert(updatedAuthInfo.ClientCertificateData, gc.DeepEquals, secret.Data[core.ServiceAccountRootCAKey])
	c.Assert(updatedAuthInfo.Token, gc.Equals, string(secret.Data[core.ServiceAccountTokenKey]))
}
