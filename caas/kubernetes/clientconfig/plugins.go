// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"context"
	"fmt"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	rbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
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

	objMeta := metav1.ObjectMeta{
		Name:      name,
		Labels:    labels,
		Namespace: adminNameSpace,
	}

	// Create admin cluster role.
	clusterRole, crCleanUps, err := getOrCreateClusterRole(objMeta, clientset.RbacV1().ClusterRoles())
	cleanUps = append(cleanUps, crCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring cluster role %q in namespace %q", name, adminNameSpace)
	}

	// Create juju admin service account.
	sa, saCleanUps, err := getOrCreateServiceAccount(objMeta, clientset.CoreV1().ServiceAccounts(adminNameSpace))
	cleanUps = append(cleanUps, saCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring service account %q in namespace %q", name, adminNameSpace)
	}

	// Create role binding for juju admin service account with admin cluster role.
	_, rbCleanUps, err := getOrCreateClusterRoleBinding(objMeta, sa, clusterRole, clientset.RbacV1().ClusterRoleBindings())
	cleanUps = append(cleanUps, rbCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(err, "ensuring cluster role binding %q", name)
	}

	secret, err := ensureSecretForServiceAccount(
		sa.GetName(), objMeta,
		clientset.CoreV1().Secrets(adminNameSpace), clock,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sa.Secrets = append(sa.Secrets, core.ObjectReference{
		Kind:      secret.Kind,
		Namespace: secret.Namespace,
		Name:      secret.Name,
		UID:       secret.UID,
	})
	_, err = clientset.CoreV1().ServiceAccounts(adminNameSpace).Update(context.TODO(), sa, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newK8sConfig(contextName, config, secret)
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
	DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error
}

func deleteRBACResource(api rbacDeleter, labels map[string]string) error {
	propagationPolicy := metav1.DeletePropagationForeground
	err := api.DeleteCollection(context.TODO(), metav1.DeleteOptions{
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
	objMeta metav1.ObjectMeta,
	api rbacv1.ClusterRoleInterface,
) (cr *rbac.ClusterRole, cleanUps cleanUpFuncs, err error) {

	cr, err = api.Get(context.TODO(), objMeta.GetName(), metav1.GetOptions{})
	if !k8serrors.IsNotFound(err) {
		return cr, cleanUps, errors.Trace(err)
	}
	// This cluster role will be granted extra privileges which requires proper
	// permissions setup for the credential in kubeconfig file.
	cr, err = api.Create(context.TODO(), &rbac.ClusterRole{
		ObjectMeta: objMeta,
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{rbac.APIGroupAll},
				Resources: []string{rbac.ResourceAll},
				Verbs:     []string{rbac.VerbAll},
			},
			{
				NonResourceURLs: []string{rbac.NonResourceAll},
				Verbs:           []string{rbac.VerbAll},
			},
		},
	}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		// This should not happen.
		return nil, cleanUps, errors.AlreadyExistsf("cluster role %q", objMeta.GetName())
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(api, objMeta.GetLabels()) })
	return cr, cleanUps, nil
}

func getOrCreateServiceAccount(
	objMeta metav1.ObjectMeta,
	api corev1.ServiceAccountInterface,
) (sa *core.ServiceAccount, cleanUps cleanUpFuncs, err error) {
	sa, err = getServiceAccount(objMeta.GetName(), api)
	if !errors.IsNotFound(err) {
		return sa, cleanUps, errors.Trace(err)
	}

	_, err = api.Create(context.TODO(), &core.ServiceAccount{ObjectMeta: objMeta}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		// This should not happen.
		return nil, cleanUps, errors.AlreadyExistsf("service account %q", objMeta.GetName())
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(api, objMeta.GetLabels()) })

	sa, err = getServiceAccount(objMeta.GetName(), api)
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	return sa, cleanUps, nil
}

func getServiceAccount(name string, client corev1.ServiceAccountInterface) (*core.ServiceAccount, error) {
	sa, err := client.Get(context.TODO(), name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", name)
	}
	return sa, errors.Trace(err)
}

func getOrCreateClusterRoleBinding(
	objMeta metav1.ObjectMeta,
	sa *core.ServiceAccount,
	cr *rbac.ClusterRole,
	api rbacv1.ClusterRoleBindingInterface,
) (rb *rbac.ClusterRoleBinding, cleanUps cleanUpFuncs, err error) {
	rb, err = api.Get(context.TODO(), objMeta.GetName(), metav1.GetOptions{})
	if !k8serrors.IsNotFound(err) {
		return rb, cleanUps, errors.Trace(err)
	}

	rb, err = api.Create(context.TODO(), &rbac.ClusterRoleBinding{
		ObjectMeta: objMeta,
		RoleRef: rbac.RoleRef{
			Kind: "ClusterRole",
			Name: cr.Name,
		},
		Subjects: []rbac.Subject{
			{
				Kind:      rbac.ServiceAccountKind,
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
	}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		// This should not happen.
		return nil, cleanUps, errors.AlreadyExistsf("cluster role binding %q", objMeta.GetName())
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(api, objMeta.GetLabels()) })
	return rb, cleanUps, nil
}

func fetchTokenReadySecret(name string, api corev1.SecretInterface, clock jujuclock.Clock) (*core.Secret, error) {
	var (
		secret *core.Secret
		err    error
	)
	// NOTE (tlm) Increased the max duration here to 120 seconds to deal with
	// microk8s bootstrapping. Microk8s is taking a significant amoutn of time
	// to be Kubernetes ready while still reporting that it is ready to go.
	// See lp:1937282
	retryCallArgs := retry.CallArgs{
		Delay:       time.Second,
		MaxDuration: 120 * time.Second,
		Clock:       clock,
		Func: func() error {
			secret, err = api.Get(context.TODO(), name, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return errors.NotFoundf("token for secret %q", name)
			}
			if err == nil {
				if _, ok := secret.Data[core.ServiceAccountTokenKey]; !ok {
					return errors.NotFoundf("token for secret %q", name)
				}
			}
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
		return nil, errors.Annotatef(err, "can not find the caas credential rbac secret %q", name)
	}
	return secret, nil
}

func ensureSecretForServiceAccount(
	saName string,
	objMeta metav1.ObjectMeta,
	secretAPI corev1.SecretInterface,
	clock jujuclock.Clock,
) (*core.Secret, error) {
	if objMeta.Annotations == nil {
		objMeta.Annotations = map[string]string{}
	}
	objMeta.Annotations[core.ServiceAccountNameKey] = saName
	_, err := secretAPI.Create(context.TODO(), &core.Secret{
		ObjectMeta: objMeta,
		Type:       core.SecretTypeServiceAccountToken,
	}, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.Trace(err)
	}
	return fetchTokenReadySecret(objMeta.GetName(), secretAPI, clock)
}

func newK8sConfig(contextName string, config *clientcmdapi.Config, secret *core.Secret) (*clientcmdapi.Config, error) {
	newConfig := config.DeepCopy()
	authName := newConfig.Contexts[contextName].AuthInfo
	newConfig.AuthInfos[authName] = &clientcmdapi.AuthInfo{
		Token: string(secret.Data[core.ServiceAccountTokenKey]),
	}
	return newConfig, nil
}
