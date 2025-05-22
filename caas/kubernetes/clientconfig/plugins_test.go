// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig_test

import (
	"fmt"
	"reflect"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/juju/juju/caas/kubernetes/clientconfig"
	"github.com/juju/juju/internal/testing"
)

type k8sRawClientSuite struct {
	BaseSuite

	UID    string
	name   string
	labels map[string]string
}

func TestK8sRawClientSuite(t *stdtesting.T) {
	tc.Run(t, &k8sRawClientSuite{})
}

func (s *k8sRawClientSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.namespace = "kube-system"
	s.UID = "9baa5e46"
	s.name = fmt.Sprintf("juju-credential-%s", s.UID)
	s.labels = map[string]string{"juju-credential": s.UID}
}

func (s *k8sRawClientSuite) TestEnsureJujuAdminServiceAccount(c *tc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cfg := newClientConfig()
	contextName := reflect.ValueOf(cfg.Contexts).MapKeys()[0].Interface().(string)

	secret := core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
			Annotations: map[string]string{
				core.ServiceAccountNameKey: s.name,
			},
		},
		Type: core.SecretTypeServiceAccountToken,
	}
	secretTokenReady := secret
	secretTokenReady.Data = map[string][]byte{
		"ca.crt":    []byte("a base64 encoded cert"),
		"namespace": []byte("base64 encoded namespace"),
		"token":     []byte("a base64 encoded bearer token"),
	}

	newSa := core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
		},
	}

	newSaWithSecretUpdated := newSa
	newSaWithSecretUpdated.Secrets = []core.ObjectReference{
		{
			Kind:      secret.Kind,
			Namespace: secret.Namespace,
			Name:      secret.Name,
			UID:       secret.UID,
		},
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
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
			Name:      s.name,
			Labels:    s.labels,
			Namespace: s.namespace,
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: cr.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      s.name,
				Namespace: s.namespace,
			},
		},
	}

	// 1st call of ensuring related resources - CREATE.
	gomock.InOrder(
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoles.EXPECT().Create(gomock.Any(), cr, metav1.CreateOptions{}).
			Return(cr, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), &newSa, metav1.CreateOptions{}).Times(1).
			Return(&newSa, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(&newSa, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Create(gomock.Any(), clusterRoleBinding, metav1.CreateOptions{}).Times(1).
			Return(clusterRoleBinding, nil),

		s.mockSecrets.EXPECT().Create(gomock.Any(), &secret, metav1.CreateOptions{}).
			Return(nil, nil),
		// fetching secret of the service account.
		// 1. Secret not found.
		s.mockSecrets.EXPECT().Get(gomock.Any(), secret.GetName(), metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		// 2. Secret does not have token filled yet.
		s.mockSecrets.EXPECT().Get(gomock.Any(), secret.GetName(), metav1.GetOptions{}).
			Return(&secret, nil),
		// 3. Token ready.
		s.mockSecrets.EXPECT().Get(gomock.Any(), secret.GetName(), metav1.GetOptions{}).
			Return(&secretTokenReady, nil),

		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(&newSa, nil),
		s.mockServiceAccounts.EXPECT().Update(gomock.Any(), &newSaWithSecretUpdated, metav1.UpdateOptions{}).Times(1).
			Return(nil, nil),
	)
	errChan := make(chan error)
	cfgOutChan := make(chan *clientcmdapi.Config)
	go func() {
		cfgOut, err := clientconfig.EnsureJujuAdminServiceAccount(c.Context(), s.k8sClient, s.UID, cfg, contextName, s.clock)
		errChan <- err
		cfgOutChan <- cfgOut
	}()

	err := s.clock.WaitAdvance(1*time.Second, testing.ShortWait, 1)
	c.Assert(err, tc.ErrorIsNil)
	err = s.clock.WaitAdvance(1*time.Second, testing.ShortWait, 1)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case err := <-errChan:
		c.Assert(err, tc.ErrorIsNil)
		cfgOut := <-cfgOutChan
		authName := cfg.Contexts[contextName].AuthInfo
		updatedAuthInfo := cfgOut.AuthInfos[authName]
		c.Assert(updatedAuthInfo.AuthProvider, tc.IsNil)
		c.Assert(updatedAuthInfo.Token, tc.Equals, string(secretTokenReady.Data[core.ServiceAccountTokenKey]))
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for deploy return")
	}

}

func (s *k8sRawClientSuite) TestEnsureJujuServiceAdminAccountIdempotent(c *tc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cfg := newClientConfig()
	contextName := reflect.ValueOf(cfg.Contexts).MapKeys()[0].Interface().(string)

	secret := core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
			Annotations: map[string]string{
				core.ServiceAccountNameKey: s.name,
			},
		},
		Type: core.SecretTypeServiceAccountToken,
	}
	secretTokenReady := secret
	secretTokenReady.Data = map[string][]byte{
		"ca.crt":    []byte("a base64 encoded cert"),
		"namespace": []byte("base64 encoded namespace"),
		"token":     []byte("a base64 encoded bearer token"),
	}

	saName := s.name
	sa := core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: s.namespace,
			Labels:    s.labels,
		},
		Secrets: []core.ObjectReference{
			{
				Kind:      secret.Kind,
				Namespace: secret.Namespace,
				Name:      secret.Name,
				UID:       secret.UID,
			},
		},
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
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
			Name:   s.name,
			Labels: s.labels,
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
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(cr, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(&sa, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(clusterRoleBinding, nil),
		s.mockSecrets.EXPECT().Create(gomock.Any(), &secret, metav1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockSecrets.EXPECT().Get(gomock.Any(), secret.GetName(), metav1.GetOptions{}).
			Return(&secretTokenReady, nil),

		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(&sa, nil),
	)
	cfgOut, err := clientconfig.EnsureJujuAdminServiceAccount(c.Context(), s.k8sClient, s.UID, cfg, contextName, s.clock)
	c.Assert(err, tc.ErrorIsNil)
	authName := cfg.Contexts[contextName].AuthInfo
	updatedAuthInfo := cfgOut.AuthInfos[authName]
	c.Assert(updatedAuthInfo.AuthProvider, tc.IsNil)
	c.Assert(updatedAuthInfo.Token, tc.Equals, string(secretTokenReady.Data[core.ServiceAccountTokenKey]))
}

func (s *k8sRawClientSuite) TestEnsureJujuServiceAdminAccount2ndUpdate(c *tc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cfg := newClientConfig()
	contextName := reflect.ValueOf(cfg.Contexts).MapKeys()[0].Interface().(string)

	secret := core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
			Annotations: map[string]string{
				core.ServiceAccountNameKey: s.name,
			},
		},
		Type: core.SecretTypeServiceAccountToken,
	}
	secretTokenReady := secret
	secretTokenReady.Data = map[string][]byte{
		"ca.crt":    []byte("a base64 encoded cert"),
		"namespace": []byte("base64 encoded namespace"),
		"token":     []byte("a base64 encoded bearer token"),
	}

	saName := s.name
	newSa := core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: s.namespace,
			Labels:    s.labels,
		},
	}

	newSaWithSecretUpdated := newSa
	newSaWithSecretUpdated.Secrets = []core.ObjectReference{
		{
			Kind:      secret.Kind,
			Namespace: secret.Namespace,
			Name:      secret.Name,
			UID:       secret.UID,
		},
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
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
			Name:   s.name,
			Labels: s.labels,
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
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(cr, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(&newSa, nil),
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(clusterRoleBinding, nil),
		s.mockSecrets.EXPECT().Create(gomock.Any(), &secret, metav1.CreateOptions{}).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockSecrets.EXPECT().Get(gomock.Any(), secret.GetName(), metav1.GetOptions{}).
			Return(&secretTokenReady, nil),

		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(&newSa, nil),
		s.mockServiceAccounts.EXPECT().Update(gomock.Any(), &newSaWithSecretUpdated, metav1.UpdateOptions{}).Times(1).
			Return(nil, nil),
	)
	cfgOut, err := clientconfig.EnsureJujuAdminServiceAccount(c.Context(), s.k8sClient, s.UID, cfg, contextName, s.clock)
	c.Assert(err, tc.ErrorIsNil)
	authName := cfg.Contexts[contextName].AuthInfo
	updatedAuthInfo := cfgOut.AuthInfos[authName]
	c.Assert(updatedAuthInfo.AuthProvider, tc.IsNil)
	c.Assert(updatedAuthInfo.Token, tc.Equals, string(secretTokenReady.Data[core.ServiceAccountTokenKey]))
}

func (s *k8sRawClientSuite) TestGetOrCreateClusterRole(c *tc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
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
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoles.EXPECT().Create(gomock.Any(), cr, metav1.CreateOptions{}).Times(1).
			Return(cr, nil),
	)
	crOut, cleanUps, err := clientconfig.GetOrCreateClusterRole(c.Context(), cr.ObjectMeta, s.k8sClient.RbacV1().ClusterRoles())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(crOut, tc.DeepEquals, cr)
	c.Assert(len(cleanUps), tc.DeepEquals, 1)

	gomock.InOrder(
		s.mockClusterRoles.EXPECT().Get(gomock.Any(), cr.Name, metav1.GetOptions{}).Times(1).
			Return(cr, nil),
	)
	crOut, cleanUps, err = clientconfig.GetOrCreateClusterRole(c.Context(), cr.ObjectMeta, s.k8sClient.RbacV1().ClusterRoles())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(crOut, tc.DeepEquals, cr)
	c.Assert(len(cleanUps), tc.DeepEquals, 0)
}

func (s *k8sRawClientSuite) TestGetOrCreateServiceAccount(c *tc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	sa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
		},
	}

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(gomock.Any(), sa, metav1.CreateOptions{}).Times(1).
			Return(sa, nil),
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(sa, nil),
	)
	saOut, cleanUps, err := clientconfig.GetOrCreateServiceAccount(c.Context(), sa.ObjectMeta, s.k8sClient.CoreV1().ServiceAccounts(s.namespace))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saOut, tc.DeepEquals, sa)
	c.Assert(len(cleanUps), tc.DeepEquals, 1)

	gomock.InOrder(
		s.mockServiceAccounts.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(sa, nil),
	)
	saOut, cleanUps, err = clientconfig.GetOrCreateServiceAccount(c.Context(), sa.ObjectMeta, s.k8sClient.CoreV1().ServiceAccounts(s.namespace))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(saOut, tc.DeepEquals, sa)
	c.Assert(len(cleanUps), tc.DeepEquals, 0)
}

func (s *k8sRawClientSuite) TestGetOrCreateClusterRoleBinding(c *tc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	sa := &core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
		},
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
			Labels:    s.labels,
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
			Name:   s.name,
			Labels: s.labels,
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
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(clusterRoleBinding, nil),
	)
	clusterRoleBindingOut, cleanUps, err := clientconfig.GetOrCreateClusterRoleBinding(
		c.Context(), clusterRoleBinding.ObjectMeta, sa, cr, s.k8sClient.RbacV1().ClusterRoleBindings(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clusterRoleBindingOut, tc.DeepEquals, clusterRoleBinding)
	c.Assert(len(cleanUps), tc.DeepEquals, 0)

	gomock.InOrder(
		s.mockClusterRoleBindings.EXPECT().Get(gomock.Any(), s.name, metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockClusterRoleBindings.EXPECT().Create(gomock.Any(), clusterRoleBinding, metav1.CreateOptions{}).Times(1).
			Return(clusterRoleBinding, nil),
	)
	clusterRoleBindingOut, cleanUps, err = clientconfig.GetOrCreateClusterRoleBinding(
		c.Context(), clusterRoleBinding.ObjectMeta, sa, cr, s.k8sClient.RbacV1().ClusterRoleBindings(),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clusterRoleBindingOut, tc.DeepEquals, clusterRoleBinding)
	c.Assert(len(cleanUps), tc.DeepEquals, 1)
}

func (s *k8sRawClientSuite) TestRemoveJujuAdminServiceAccount(c *tc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	labelSelector := fmt.Sprintf("juju-credential=%s", s.UID)
	gomock.InOrder(
		s.mockClusterRoleBindings.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(metav1.DeletePropagationForeground),
			metav1.ListOptions{LabelSelector: labelSelector},
		).Times(1).Return(nil),
		s.mockClusterRoles.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(metav1.DeletePropagationForeground),
			metav1.ListOptions{LabelSelector: labelSelector},
		).Times(1).Return(nil),
		s.mockServiceAccounts.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(metav1.DeletePropagationForeground),
			metav1.ListOptions{LabelSelector: labelSelector},
		).Times(1).Return(nil),
	)

	err := clientconfig.RemoveJujuAdminServiceAccount(c.Context(), s.k8sClient, s.UID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *k8sRawClientSuite) deleteOptions(policy metav1.DeletionPropagation) metav1.DeleteOptions {
	return metav1.DeleteOptions{
		PropagationPolicy: &policy,
	}
}
