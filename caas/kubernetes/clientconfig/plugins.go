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
	adminNameSpace                  = "kube-system"
	clusterAdminRole                = "cluster-admin"
	jujuAdminAccountName            = "juju-caas"
	jujuAdminAccountNameRoleBinding = "juju-caas-role-binding"
)

func newK8sClientSet(config *clientcmdapi.Config, contextName string) (*kubernetes.Clientset, error) {
	clientCfg, err := clientcmd.NewNonInteractiveClientConfig(
		*config, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return kubernetes.NewForConfig(clientCfg)
}

func ensureJujuAdminServiceAccount(
	clientset *kubernetes.Clientset,
	config *clientcmdapi.Config,
	contextName string,
) (*clientcmdapi.Config, error) {

	serviceAccountAPI := clientset.CoreV1().ServiceAccounts(adminNameSpace)

	// get admin cluster role.
	clusterRole, err := clientset.RbacV1().ClusterRoles().Get(clusterAdminRole, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// create juju admin service account.
	jujuAccount, err := serviceAccountAPI.Create(&core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jujuAdminAccountName,
			Namespace: adminNameSpace,
		},
	})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.Trace(err)
	}

	// create role binding for juju admin service account with admin cluster role.
	_, err = clientset.RbacV1().ClusterRoleBindings().Create(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: jujuAdminAccountNameRoleBinding,
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: clusterRole.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      jujuAdminAccountName,
				Namespace: adminNameSpace,
			},
		},
	})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.Trace(err)
	}

	// refresh service account after secrets created.
	jujuAccount, err = serviceAccountAPI.Get(jujuAdminAccountName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// get bearer token of juju admin service account.
	secret, err := clientset.CoreV1().Secrets(adminNameSpace).Get(jujuAccount.Secrets[0].Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	replaceAuthProviderWithServiceAccountAuthData(contextName, config, secret)
	return config, nil
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
