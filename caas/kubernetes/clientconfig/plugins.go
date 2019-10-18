// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"fmt"
	"sort"
	"strings"

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

const adminNameSpace = "kube-system"

func getRBACLabels(stackName string) map[string]string {
	return map[string]string{
		"juju-cloud": stackName,
	}
}

type cleanUpFuncs []func()

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

func createJujuAdminServiceAccount(
	clientset kubernetes.Interface,
	stackName string,
	config *clientcmdapi.Config,
	contextName string,
) (_ *clientcmdapi.Config, err error) {
	labels := getRBACLabels(stackName)

	var cleanUps cleanUpFuncs
	defer func() {
		if err == nil {
			return
		}
		for _, f := range cleanUps {
			f()
		}
	}()

	// Create admin cluster role.
	clusterRole, crCleanUps, err := createClusterRole(clientset, stackName, adminNameSpace, labels)
	cleanUps = append(cleanUps, crCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring cluster role %q in namespace %q", stackName, adminNameSpace)
	}

	// Create juju admin service account.
	sa, saCleanUps, err := createServiceAccount(clientset, stackName, adminNameSpace, labels)
	cleanUps = append(cleanUps, saCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring service account %q in namespace %q", stackName, adminNameSpace)
	}

	// Create role binding for juju admin service account with admin cluster role.
	_, rbCleanUps, err := createClusterRoleBinding(clientset, stackName, sa, clusterRole, labels)
	cleanUps = append(cleanUps, rbCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(err, "ensuring cluster role binding %q", stackName)
	}

	// Refresh service account to get the secret/token after cluster role binding created.
	sa, err = getServiceAccount(clientset, stackName, adminNameSpace)
	if err != nil {
		return nil, errors.Annotatef(
			err, "refetching service account %q after cluster role binding created", stackName)
	}

	// Get bearer token of the service account.
	secret, err := getServiceAccountSecret(clientset, sa)
	if err != nil {
		return nil, errors.Annotatef(err, "fetching bearer token for service account %q", sa.Name)
	}

	replaceAuthProviderWithServiceAccountAuthData(contextName, config, secret)
	return config, nil
}

func removeJujuAdminServiceAccount(clientset kubernetes.Interface, stackName string) error {
	// TODO: can we delete the credential while using itself as credential to talk to the cluster???????
	labels := getRBACLabels(stackName)
	for _, api := range []rbacDeleter{
		// Order matters.
		clientset.RbacV1().ClusterRoleBindings(),
		clientset.RbacV1().ClusterRoles(),
		clientset.CoreV1().ServiceAccounts(adminNameSpace),
	} {
		if err := deleteRBACResource(api, labels); err != nil {
			logger.Warningf("deleting rbac resources: %v", err)
		}
	}
	return nil
}

type rbacDeleter interface {
	DeleteCollection(*metav1.DeleteOptions, metav1.ListOptions) error
}

func deleteRBACResource(api rbacDeleter, labels map[string]string) error {
	labelsToSelector := func(labels map[string]string) string {
		var selectors []string
		for k, v := range labels {
			selectors = append(selectors, fmt.Sprintf("%v==%v", k, v))
		}
		sort.Strings(selectors) // for tests.
		return strings.Join(selectors, ",")
	}
	propagationPolicy := metav1.DeletePropagationForeground
	err := api.DeleteCollection(&metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}, metav1.ListOptions{
		LabelSelector:        labelsToSelector(labels),
		IncludeUninitialized: true,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func createClusterRole(clientset kubernetes.Interface, name, namespace string, labels map[string]string) (cr *rbacv1.ClusterRole, cleanUps cleanUpFuncs, err error) {
	// This cluster role will be granted extra privileges which requires proper
	// permissions setup for the credential in kubeconfig file.
	api := clientset.RbacV1().ClusterRoles()
	cr, err = api.Create(&rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
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
	if k8serrors.IsAlreadyExists(err) {
		return nil, cleanUps, errors.AlreadyExistsf("cluster role %q", name)
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(api, labels) })
	return cr, cleanUps, nil
}

func createServiceAccount(
	clientset kubernetes.Interface,
	name, namespace string, labels map[string]string,
) (sa *core.ServiceAccount, cleanUps cleanUpFuncs, err error) {
	api := clientset.CoreV1().ServiceAccounts(namespace)
	_, err = api.Create(&core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
	})
	if k8serrors.IsAlreadyExists(err) {
		return nil, cleanUps, errors.AlreadyExistsf("service account %q", name)
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(api, labels) })

	sa, err = getServiceAccount(clientset, name, namespace)
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	return sa, cleanUps, nil
}

func getServiceAccount(clientset kubernetes.Interface, name, namespace string) (*core.ServiceAccount, error) {
	return clientset.CoreV1().ServiceAccounts(namespace).Get(name, metav1.GetOptions{})
}

func createClusterRoleBinding(
	clientset kubernetes.Interface,
	name string,
	sa *core.ServiceAccount,
	cr *rbacv1.ClusterRole,
	labels map[string]string,
) (rb *rbacv1.ClusterRoleBinding, cleanUps cleanUpFuncs, err error) {
	api := clientset.RbacV1().ClusterRoleBindings()
	rb, err = api.Create(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
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
	if k8serrors.IsAlreadyExists(err) {
		return nil, cleanUps, errors.AlreadyExistsf("cluster role binding %q", name)
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(api, labels) })
	return rb, cleanUps, nil
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
	config.AuthInfos[authName] = &clientcmdapi.AuthInfo{
		ClientCertificateData: secret.Data[core.ServiceAccountRootCAKey],
		Token:                 string(secret.Data[core.ServiceAccountTokenKey]),
	}
}
