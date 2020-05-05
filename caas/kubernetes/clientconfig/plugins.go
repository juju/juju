// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"fmt"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // load gcp auth plugin.
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	adminNameSpace  = "kube-system"
	rbacStackPrefix = "juju-credential"
)

func getRBACLabels(UID string) map[string]string {
	return map[string]string{
		rbacStackPrefix: UID,
	}
}

func getRBACResourceName(UID string) string {
	return fmt.Sprintf("%s-%s", rbacStackPrefix, UID)
}

type cleanUpFuncs []func()

// To regenerate the mocks for the kubernetes Client used by this package,
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ../provider/mocks/restclient_mock.go -mock_names=Interface=MockRestClientInterface k8s.io/client-go/rest  Interface
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ../provider/mocks/serviceaccount_mock.go k8s.io/client-go/kubernetes/typed/core/v1 ServiceAccountInterface

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
	UID string,
	config *clientcmdapi.Config,
	contextName string,
	clock jujuclock.Clock,
) (_ *clientcmdapi.Config, err error) {
	labels := getRBACLabels(UID)
	name := getRBACResourceName(UID)

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
	clusterRole, crCleanUps, err := getOrCreateClusterRole(clientset, name, adminNameSpace, labels)
	cleanUps = append(cleanUps, crCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring cluster role %q in namespace %q", name, adminNameSpace)
	}

	// Create juju admin service account.
	sa, saCleanUps, err := getOrCreateServiceAccount(clientset, name, adminNameSpace, labels)
	cleanUps = append(cleanUps, saCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring service account %q in namespace %q", name, adminNameSpace)
	}

	// Create role binding for juju admin service account with admin cluster role.
	_, rbCleanUps, err := getOrCreateClusterRoleBinding(clientset, name, sa, clusterRole, labels)
	cleanUps = append(cleanUps, rbCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(err, "ensuring cluster role binding %q", name)
	}

	var secret *core.Secret
	retryCallArgs := retry.CallArgs{
		Delay:       time.Second,
		MaxDuration: 5 * time.Second,
		Clock:       clock,
		Func: func() error {
			secret, err = getServiceAccountSecret(clientset, name, adminNameSpace)
			return err
		},
		IsFatalError: func(err error) bool {
			return !errors.IsNotFound(err)
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf("polling caas credential rbac secret, in %d attempt, %v", attempt, err)
		},
	}
	if err = retry.Call(retryCallArgs); err != nil {
		return nil, errors.Trace(err)
	}

	if secret == nil {
		return nil, errors.Annotatef(nil, "polling caas credential rbac secret %q", name)
	}

	replaceAuthProviderWithServiceAccountAuthData(contextName, config, secret)
	return config, nil
}

// RemoveCredentialRBACResources removes all RBAC resources for specific caas credential UID.
func RemoveCredentialRBACResources(config *rest.Config, UID string) error {
	// TODO(caas): call this in destroy/kill-controller with UID == "microk8s".
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Trace(err)
	}
	return removeJujuAdminServiceAccount(clientset, UID)
}

func removeJujuAdminServiceAccount(clientset kubernetes.Interface, UID string) error {
	labels := getRBACLabels(UID)
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
	propagationPolicy := metav1.DeletePropagationForeground
	err := api.DeleteCollection(&metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}, metav1.ListOptions{
		LabelSelector: k8slabels.SelectorFromValidatedSet(labels).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func getOrCreateClusterRole(
	clientset kubernetes.Interface,
	name, namespace string, labels map[string]string,
) (cr *rbacv1.ClusterRole, cleanUps cleanUpFuncs, err error) {
	api := clientset.RbacV1().ClusterRoles()

	cr, err = api.Get(name, metav1.GetOptions{})
	if !k8serrors.IsNotFound(err) {
		return cr, cleanUps, errors.Trace(err)
	}
	// This cluster role will be granted extra privileges which requires proper
	// permissions setup for the credential in kubeconfig file.
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
		// This should not happen.
		return nil, cleanUps, errors.AlreadyExistsf("cluster role %q", name)
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(api, labels) })
	return cr, cleanUps, nil
}

func getOrCreateServiceAccount(
	clientset kubernetes.Interface,
	name, namespace string, labels map[string]string,
) (sa *core.ServiceAccount, cleanUps cleanUpFuncs, err error) {

	sa, err = getServiceAccount(clientset, name, namespace)
	if !errors.IsNotFound(err) {
		return sa, cleanUps, errors.Trace(err)
	}

	api := clientset.CoreV1().ServiceAccounts(namespace)
	_, err = api.Create(&core.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
	})
	if k8serrors.IsAlreadyExists(err) {
		// This should not happen.
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
	sa, err := clientset.CoreV1().ServiceAccounts(namespace).Get(name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", name)
	}
	return sa, errors.Trace(err)
}

func getOrCreateClusterRoleBinding(
	clientset kubernetes.Interface,
	name string,
	sa *core.ServiceAccount,
	cr *rbacv1.ClusterRole,
	labels map[string]string,
) (rb *rbacv1.ClusterRoleBinding, cleanUps cleanUpFuncs, err error) {
	api := clientset.RbacV1().ClusterRoleBindings()

	rb, err = api.Get(name, metav1.GetOptions{})
	if !k8serrors.IsNotFound(err) {
		return rb, cleanUps, errors.Trace(err)
	}

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
		// This should not happen.
		return nil, cleanUps, errors.AlreadyExistsf("cluster role binding %q", name)
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(api, labels) })
	return rb, cleanUps, nil
}

func getServiceAccountSecret(clientset kubernetes.Interface, saName, namespace string) (*core.Secret, error) {
	sa, err := getServiceAccount(clientset, saName, namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}

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
