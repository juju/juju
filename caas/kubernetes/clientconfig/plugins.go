// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"context"
	"fmt"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
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

	"github.com/juju/juju/internal/provider/kubernetes/proxy"
)

const (
	adminNameSpace  = "kube-system"
	rbacStackPrefix = "juju-credential"
)

func getRBACLabels(uid string) map[string]string {
	return map[string]string{
		rbacStackPrefix: uid,
	}
}

func getRBACResourceName(uid string) string {
	return fmt.Sprintf("%s-%s", rbacStackPrefix, uid)
}

type cleanUpFuncs []func()

// To regenerate the mocks for the kubernetes Client used by this package,
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/k8sclient_mock.go k8s.io/client-go/kubernetes Interface
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/serviceaccount_mock.go k8s.io/client-go/kubernetes/typed/core/v1 ServiceAccountInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/rbacv1_mock.go k8s.io/client-go/kubernetes/typed/rbac/v1 RbacV1Interface,ClusterRoleBindingInterface,ClusterRoleInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/corev1_mock.go k8s.io/client-go/kubernetes/typed/core/v1 CoreV1Interface,SecretInterface

func newK8sClientSet(config *clientcmdapi.Config, contextName string) (*kubernetes.Clientset, error) {
	clientCfg, err := clientcmd.NewNonInteractiveClientConfig(
		*config, contextName, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return kubernetes.NewForConfig(clientCfg)
}

func ensureJujuAdminServiceAccount(
	ctx context.Context,
	clientset kubernetes.Interface,
	uid string,
	config *clientcmdapi.Config,
	contextName string,
	clock jujuclock.Clock,
) (_ *clientcmdapi.Config, err error) {
	labels := getRBACLabels(uid)
	name := getRBACResourceName(uid)

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
	clusterRole, crCleanUps, err := getOrCreateClusterRole(ctx, objMeta, clientset.RbacV1().ClusterRoles())
	cleanUps = append(cleanUps, crCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring cluster role %q in namespace %q", name, adminNameSpace)
	}

	// Create juju admin service account.
	sa, saCleanUps, err := getOrCreateServiceAccount(ctx, objMeta, clientset.CoreV1().ServiceAccounts(adminNameSpace))
	cleanUps = append(cleanUps, saCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(
			err, "ensuring service account %q in namespace %q", name, adminNameSpace)
	}

	// Create role binding for juju admin service account with admin cluster role.
	_, rbCleanUps, err := getOrCreateClusterRoleBinding(ctx, objMeta, sa, clusterRole, clientset.RbacV1().ClusterRoleBindings())
	cleanUps = append(cleanUps, rbCleanUps...)
	if err != nil {
		return nil, errors.Annotatef(err, "ensuring cluster role binding %q", name)
	}

	secret, err := proxy.EnsureSecretForServiceAccount(
		ctx, sa.GetName(), objMeta, clock,
		clientset.CoreV1().Secrets(adminNameSpace),
		clientset.CoreV1().ServiceAccounts(adminNameSpace),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newK8sConfig(contextName, config, secret)
}

// RemoveCredentialRBACResources removes all RBAC resources for specific caas credential UID.
func RemoveCredentialRBACResources(ctx context.Context, config *rest.Config, uid string) error {
	// TODO(caas): call this in destroy/kill-controller with UID == "microk8s".
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Trace(err)
	}
	return removeJujuAdminServiceAccount(ctx, clientset, uid)
}

func removeJujuAdminServiceAccount(ctx context.Context, clientset kubernetes.Interface, uid string) error {
	labels := getRBACLabels(uid)
	for _, api := range []rbacDeleter{
		// Order matters.
		clientset.RbacV1().ClusterRoleBindings(),
		clientset.RbacV1().ClusterRoles(),
		clientset.CoreV1().ServiceAccounts(adminNameSpace),
	} {
		if err := deleteRBACResource(ctx, api, labels); err != nil {
			logger.Warningf(context.TODO(), "deleting rbac resources: %v", err)
		}
	}
	return nil
}

type rbacDeleter interface {
	DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error
}

func deleteRBACResource(ctx context.Context, api rbacDeleter, labels map[string]string) error {
	propagationPolicy := metav1.DeletePropagationForeground
	err := api.DeleteCollection(ctx, metav1.DeleteOptions{
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
	ctx context.Context,
	objMeta metav1.ObjectMeta,
	api rbacv1.ClusterRoleInterface,
) (cr *rbac.ClusterRole, cleanUps cleanUpFuncs, err error) {

	cr, err = api.Get(ctx, objMeta.GetName(), metav1.GetOptions{})
	if !k8serrors.IsNotFound(err) {
		return cr, cleanUps, errors.Trace(err)
	}
	// This cluster role will be granted extra privileges which requires proper
	// permissions setup for the credential in kubeconfig file.
	cr, err = api.Create(ctx, &rbac.ClusterRole{
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
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(ctx, api, objMeta.GetLabels()) })
	return cr, cleanUps, nil
}

func getOrCreateServiceAccount(
	ctx context.Context,
	objMeta metav1.ObjectMeta,
	api corev1.ServiceAccountInterface,
) (sa *core.ServiceAccount, cleanUps cleanUpFuncs, err error) {
	sa, err = getServiceAccount(ctx, objMeta.GetName(), api)
	if !errors.Is(err, errors.NotFound) {
		return sa, cleanUps, errors.Trace(err)
	}

	_, err = api.Create(ctx, &core.ServiceAccount{ObjectMeta: objMeta}, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		// This should not happen.
		return nil, cleanUps, errors.AlreadyExistsf("service account %q", objMeta.GetName())
	}
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(ctx, api, objMeta.GetLabels()) })

	sa, err = getServiceAccount(ctx, objMeta.GetName(), api)
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	return sa, cleanUps, nil
}

func getServiceAccount(ctx context.Context, name string, client corev1.ServiceAccountInterface) (*core.ServiceAccount, error) {
	sa, err := client.Get(ctx, name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("service account %q", name)
	}
	return sa, errors.Trace(err)
}

func getOrCreateClusterRoleBinding(
	ctx context.Context,
	objMeta metav1.ObjectMeta,
	sa *core.ServiceAccount,
	cr *rbac.ClusterRole,
	api rbacv1.ClusterRoleBindingInterface,
) (rb *rbac.ClusterRoleBinding, cleanUps cleanUpFuncs, err error) {
	rb, err = api.Get(ctx, objMeta.GetName(), metav1.GetOptions{})
	if !k8serrors.IsNotFound(err) {
		return rb, cleanUps, errors.Trace(err)
	}

	rb, err = api.Create(ctx, &rbac.ClusterRoleBinding{
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
	cleanUps = append(cleanUps, func() { _ = deleteRBACResource(ctx, api, objMeta.GetLabels()) })
	return rb, cleanUps, nil
}

func newK8sConfig(contextName string, config *clientcmdapi.Config, secret *core.Secret) (*clientcmdapi.Config, error) {
	newConfig := config.DeepCopy()
	authName := newConfig.Contexts[contextName].AuthInfo
	newConfig.AuthInfos[authName] = &clientcmdapi.AuthInfo{
		Token: string(secret.Data[core.ServiceAccountTokenKey]),
	}
	return newConfig, nil
}
