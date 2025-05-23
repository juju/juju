// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
	rbac "k8s.io/client-go/kubernetes/typed/rbac/v1"

	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

var logger = internallogger.GetLogger("juju.caas.kubernetes.provider.proxy")

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

var (
	proxyRole = rbacv1.Role{
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
)

// proxyRoleForName builds the role needed for proxying to pods within a given
// namespace.
func proxyRoleForName(name string, lbs labels.Set) *rbacv1.Role {
	role := proxyRole
	role.ObjectMeta = meta.ObjectMeta{
		Labels: lbs,
		Name:   name,
	}
	return &role
}

// EnsureProxyService ensures there is a proxy service account in existence for
// the namespace of a Kubernetes model.
func EnsureProxyService(
	ctx context.Context,
	lbs labels.Set,
	name string,
	clock clock.Clock,
	roleI rbac.RoleInterface,
	roleBindingI rbac.RoleBindingInterface,
	saI core.ServiceAccountInterface,
	secretI core.SecretInterface,
) error {
	pr := proxyRoleForName(name, lbs)
	roleRVal, err := roleI.Create(ctx, pr, meta.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		roleRVal, err = roleI.Update(ctx, pr, meta.UpdateOptions{})
	}
	if err != nil {
		return errors.Annotate(err, "cannot create proxy service account role")
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: meta.ObjectMeta{
			Labels: lbs,
			Name:   name,
		},
	}

	saRVal, err := saI.Create(ctx, sa, meta.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		saRVal, err = saI.Get(ctx, sa.Name, meta.GetOptions{})
	}
	if err != nil {
		return errors.Annotate(err, "creating proxy service account")
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Labels: lbs,
			Name:   name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saRVal.Name,
				Namespace: saRVal.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleRVal.Name,
		},
	}

	_, err = roleBindingI.Create(ctx, roleBinding, meta.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		_, err = roleBindingI.Update(ctx, roleBinding, meta.UpdateOptions{})
	}
	if err != nil {
		return errors.Annotate(err, "creating proxy service account role binding")
	}
	objMeta := meta.ObjectMeta{
		Labels: lbs,
		Name:   name,
	}
	_, err = EnsureSecretForServiceAccount(ctx, sa.GetName(), objMeta, clock, secretI, saI)
	if err != nil {
		return errors.Trace(err)
	}

	return nil

}

// WaitForProxyService attempt to block the caller until the proxy service is
// fully provisioned within Kubernetes or until the function gives up trying to
// wait. This should be a very quick wait.
func WaitForProxyService(
	ctx context.Context,
	name string,
	saI core.ServiceAccountInterface,
) error {
	hasSASecret := func() error {
		svc, err := saI.Get(ctx, name, meta.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return errors.NewNotFound(err, "proxy service for "+name)
		} else if err != nil {
			return errors.Annotatef(err, "getting proxy service for %s", name)
		}

		if len(svc.Secrets) == 0 {
			return errors.NotProvisionedf("proxy service for %s", name)
		}

		return nil
	}
	return retry.Call(retry.CallArgs{
		Func: hasSASecret,
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotProvisioned)
		},
		Attempts: 5,
		Delay:    time.Second * 3,
		Clock:    clock.WallClock,
	})
}

// CreateControllerProxy establishes the Kubernetes resources needed for
// proxying to a Juju controller. The end result of this function is a service
// account with a set of permissions that the Juju client can use for proxying
// to a controller.
func CreateControllerProxy(
	ctx context.Context,
	config ControllerProxyConfig,
	lbs labels.Set,
	clock clock.Clock,
	configI core.ConfigMapInterface,
	roleI rbac.RoleInterface,
	roleBindingI rbac.RoleBindingInterface,
	saI core.ServiceAccountInterface,
	secretI core.SecretInterface,
) error {
	lbs = labels.Merge(lbs, utils.LabelsJuju)

	err := EnsureProxyService(ctx, lbs, config.Name, clock, roleI, roleBindingI, saI, secretI)
	if err != nil {
		return errors.Annotate(err, "ensuring proxy service account")
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshalling proxy configmap data to json: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Labels: lbs,
			Name:   config.Name,
		},
		Data: map[string]string{
			ProxyConfigMapKey: string(configJSON),
		},
	}

	_, err = configI.Create(ctx, cm, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating proxy config map: %w", err)
	}

	return nil
}

// FetchTokenReadySecret fetches and returns the secret when the token field gets populated.
func FetchTokenReadySecret(ctx context.Context, name string, api core.SecretInterface, clock clock.Clock) (*corev1.Secret, error) {
	var (
		secret *corev1.Secret
		err    error
	)
	// NOTE (tlm) Increased the max duration here to 120 seconds to deal with
	// microk8s bootstrapping. Microk8s is taking a significant amount of time
	// to be Kubernetes ready while still reporting that it is ready to go.
	// See lp:1937282
	retryCallArgs := retry.CallArgs{
		Delay:       time.Second,
		MaxDuration: 120 * time.Second,
		Stop:        ctx.Done(),
		Clock:       clock,
		Func: func() error {
			secret, err = api.Get(ctx, name, meta.GetOptions{})
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
			return !errors.Is(err, errors.NotFound)
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf(context.TODO(), "polling caas credential rbac secret, in %d attempt, %v", attempt, err)
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
	ctx context.Context,
	saName string,
	objMeta meta.ObjectMeta,
	clock clock.Clock,
	secretAPI core.SecretInterface,
	saAPI core.ServiceAccountInterface,
) (*corev1.Secret, error) {
	if objMeta.Annotations == nil {
		objMeta.Annotations = map[string]string{}
	}
	objMeta.Annotations[corev1.ServiceAccountNameKey] = saName
	_, err := secretAPI.Create(ctx, &corev1.Secret{
		ObjectMeta: objMeta,
		Type:       corev1.SecretTypeServiceAccountToken,
	}, meta.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, errors.Trace(err)
	}
	// NOTE (tlm) Increased the max duration here to 120 seconds to deal with
	// microk8s bootstrapping. Microk8s is taking a significant amoutn of time
	// to be Kubernetes ready while still reporting that it is ready to go.
	// See lp:1937282
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	secret, err := FetchTokenReadySecret(ctx, objMeta.GetName(), secretAPI, clock)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// sa may be updated already in 1.21 or earlier versions, so get the latest sa.
	sa, err := saAPI.Get(ctx, saName, meta.GetOptions{})
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
		_, err = saAPI.Update(ctx, sa, meta.UpdateOptions{})
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return secret, nil
}

func isSecretIncluded(secret corev1.Secret, secrets []corev1.ObjectReference) bool {
	for _, item := range secrets {
		if item.Kind != "" && item.Kind != secret.Kind {
			continue
		}
		if item.Namespace != "" && item.Namespace != secret.Namespace {
			continue
		}
		if item.Name != "" && item.Name != secret.Name {
			continue
		}
		return true
	}
	return false
}
