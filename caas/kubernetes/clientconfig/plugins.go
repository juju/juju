// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // load gcp auth plugin.
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	adminNameSpace             = "kube-system"
	clusterRoleName            = "cluster-admin"
	jujuServiceAccountName     = "juju-service-account"
	jujuClusterRoleBindingName = "juju-cluster-role-binding"
)

// To regenerate the mocks for the kubernetes Client used by this package,
//go:generate mockgen -package mocks -destination ../provider/mocks/restclient_mock.go -mock_names=Interface=MockRestClientInterface k8s.io/client-go/rest  Interface
//go:generate mockgen -package mocks -destination ../provider/mocks/serviceaccount_mock.go k8s.io/client-go/kubernetes/typed/core/v1 ServiceAccountInterface

func newK8sClientSet(config *clientcmdapi.Config, contextName string) (*kubernetes.Clientset, error) {
	clientCfg, err := clientcmd.NewNonInteractiveClientConfig(
		*config, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return kubernetes.NewForConfig(clientCfg)
}

func ensureJujuAdminServiceAccount(
	clientset kubernetes.Interface,
	config *clientcmdapi.Config,
	contextName string,
) (*clientcmdapi.Config, error) {

	// ensure admin cluster role.
	clusterRole, err := ensureClusterRole(clientset, clusterRoleName, adminNameSpace)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring cluster role %q in namespace %q", clusterRoleName, adminNameSpace)
	}

	// create juju admin service account.
	sa, err := ensureServiceAccount(clientset, jujuServiceAccountName, adminNameSpace)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring service account %q in namespace %q", jujuServiceAccountName, adminNameSpace)
	}

	// ensure role binding for juju admin service account with admin cluster role.
	_, err = ensureClusterRoleBinding(clientset, jujuClusterRoleBindingName, sa, clusterRole)
	if err != nil {
		return nil, errors.Annotatef(err, "ensuring cluster role binding %q", jujuClusterRoleBindingName)
	}

	// refresh service account to get the secret/token after cluster role binding created.
	sa, err = getServiceAccount(clientset, jujuServiceAccountName, adminNameSpace)
	if err != nil {
		return nil, errors.Annotatef(
			err, "refetching service account %q after cluster role binding created", jujuServiceAccountName)
	}

	// get bearer token of juju admin service account.
	secret, err := getServiceAccountSecret(clientset, sa)
	if err != nil {
		return nil, errors.Annotatef(err, "fetching bearer token for service account %q", sa.Name)
	}

	replaceAuthProviderWithServiceAccountAuthData(contextName, config, secret)
	return config, nil
}

func ensureClusterRole(clientset kubernetes.Interface, name, namespace string) (*rbacv1.ClusterRole, error) {
	// try get first because it's more usual to reuse cluster role.
	clusterRole, err := clientset.RbacV1().ClusterRoles().Get(name, metav1.GetOptions{})
	if err == nil {
		return clusterRole, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	// No existing cluster role found, so create one.
	// This cluster role will be granted extra privileges which requires proper
	// permissions setup for the credential in kubeconfig file.
	cr, err := clientset.RbacV1().ClusterRoles().Create(&rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
	})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.Trace(err)
	}
	return cr, nil
}

func ensureServiceAccount(clientset kubernetes.Interface, name, namespace string) (*core.ServiceAccount, error) {
	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(&core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.Trace(err)
	}
	return getServiceAccount(clientset, name, namespace)
}

func getServiceAccount(clientset kubernetes.Interface, name, namespace string) (*core.ServiceAccount, error) {
	return clientset.CoreV1().ServiceAccounts(namespace).Get(name, metav1.GetOptions{})
}

func ensureClusterRoleBinding(
	clientset kubernetes.Interface,
	name string,
	sa *core.ServiceAccount,
	cr *rbacv1.ClusterRole,
) (*rbacv1.ClusterRoleBinding, error) {
	rb, err := clientset.RbacV1().ClusterRoleBindings().Create(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
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
	})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.Trace(err)
	}
	return rb, nil
}

func getServiceAccountSecret(clientset kubernetes.Interface, sa *core.ServiceAccount) (*core.Secret, error) {
	if len(sa.Secrets) == 0 {
		return nil, errors.NotFoundf("secret for service account %q", sa.Name)
	}
	return clientset.CoreV1().Secrets(sa.Namespace).Get(sa.Secrets[0].Name, metav1.GetOptions{})
}

func replaceAuthProviderWithServiceAccountAuthData(
	contextName string,
	config *clientcmdapi.Config,
	secret *core.Secret,
) {
	authName := config.Contexts[contextName].AuthInfo
	currentAuth := config.AuthInfos[authName]
	currentAuth.AuthProvider = nil
	currentAuth.ClientCertificateData = secret.Data[core.ServiceAccountRootCAKey]
	currentAuth.Token = string(secret.Data[core.ServiceAccountTokenKey])
	config.AuthInfos[authName] = currentAuth
}
