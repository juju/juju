// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
	rbac "k8s.io/client-go/kubernetes/typed/rbac/v1"
)

var logger = loggo.GetLogger("juju.caas.kubernetes.provider.proxy")

// ControllerProxyConfig is used to configure the kubernetes resources made for
// the controller proxy objects.
type ControllerProxyConfig struct {
	// Name to apply to kubernetes resources created for the controller
	// proxy. This name is also used later on for discovery of the proxy config.
	Name string `json:"name"`

	// Namespace to create the proxy kubernetes resources in. This is ultimately
	// used for discovery of the proxy settings.
	Namespace string `json:"namespace"`

	// RemotePort the remote port of the service to use when proxying
	RemotePort string `json:"remote-port"`

	// TargetService the service to target for proxying
	TargetService string `json:"target-service"`
}

const (
	// ProxyConfigMapKey the key to use in the configmap made for the proxy to
	// describe the config key
	ProxyConfigMapKey = "config"
)

// CreateControllerProxy establishes the Kubernetes resources needed for
// proxying to a Juju controller. The end result of this function is a service
// account with a set of permissions that the Juju client can use for proxying
// to a controller.
func CreateControllerProxy(
	config ControllerProxyConfig,
	labels labels.Set,
	clock jujuclock.Clock,
	configI core.ConfigMapInterface,
	roleI rbac.RoleInterface,
	roleBindingI rbac.RoleBindingInterface,
	saI core.ServiceAccountInterface,
	secretI core.SecretInterface,
) error {
	objMeta := meta.ObjectMeta{
		Labels: labels,
		Name:   config.Name,
	}

	role := &rbacv1.Role{
		ObjectMeta: objMeta,
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"list", "get", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"get"},
			},
			// The get verb below is not used directly by juju but is for
			// the python lib
			{
				APIGroups: []string{""},
				Resources: []string{"pods/portforward"},
				Verbs:     []string{"create", "get"},
			},
		},
	}

	role, err := roleI.Create(context.TODO(), role, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating proxy service account role: %w", err)
	}

	sa := &corev1.ServiceAccount{ObjectMeta: objMeta}
	sa, err = saI.Create(context.TODO(), sa, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating proxy service account: %w", err)
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: objMeta,
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
	}

	_, err = roleBindingI.Create(context.TODO(), roleBinding, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating proxy service account role binding: %w", err)
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshalling proxy configmap data to json: %w", err)
	}

	_, err = EnsureSecretForServiceAccount(sa.GetName(), objMeta, clock, secretI, saI)
	if err != nil {
		return errors.Trace(err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: objMeta,
		Data: map[string]string{
			ProxyConfigMapKey: string(configJSON),
		},
	}

	_, err = configI.Create(context.TODO(), cm, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating proxy config map: %w", err)
	}

	return nil
}

func fetchTokenReadySecret(name string, api core.SecretInterface, clock jujuclock.Clock) (*corev1.Secret, error) {
	var (
		secret *corev1.Secret
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
			secret, err = api.Get(context.TODO(), name, meta.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return errors.NotFoundf("token for secret %q", name)
			}
			if err == nil {
				if _, ok := secret.Data[corev1.ServiceAccountTokenKey]; !ok {
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

// EnsureSecretForServiceAccount ensures secret for the provided service created and ready to use.
func EnsureSecretForServiceAccount(
	saName string,
	objMeta meta.ObjectMeta,
	clock jujuclock.Clock,
	secretAPI core.SecretInterface,
	saAPI core.ServiceAccountInterface,
) (*corev1.Secret, error) {
	if objMeta.Annotations == nil {
		objMeta.Annotations = map[string]string{}
	}
	objMeta.Annotations[corev1.ServiceAccountNameKey] = saName
	_, err := secretAPI.Create(context.TODO(), &corev1.Secret{
		ObjectMeta: objMeta,
		Type:       corev1.SecretTypeServiceAccountToken,
	}, meta.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.Trace(err)
	}
	secret, err := fetchTokenReadySecret(objMeta.GetName(), secretAPI, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// sa may be updated already in 1.21 or earlier versions, so get the latest sa.
	sa, err := saAPI.Get(context.TODO(), saName, meta.GetOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isSecretIncluded(*secret, sa.Secrets) {
		sa.Secrets = append(sa.Secrets, corev1.ObjectReference{
			Kind:      secret.Kind,
			Namespace: secret.Namespace,
			Name:      secret.Name,
			UID:       secret.UID,
		})
		_, err = saAPI.Update(context.TODO(), sa, meta.UpdateOptions{})
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return secret, nil
}

func isSecretIncluded(secret corev1.Secret, secrets []corev1.ObjectReference) bool {
	for _, item := range secrets {
		if secret.Kind == item.Kind && secret.Name == item.Name && secret.Namespace == item.Namespace {
			return true
		}
	}
	return false
}
