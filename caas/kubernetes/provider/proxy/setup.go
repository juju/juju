// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
	rbac "k8s.io/client-go/kubernetes/typed/rbac/v1"
	//"github.com/juju/juju/caas/kubernetes/provider/utils"
)

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

// EnsureModelProxy ensures there is a proxy service account in existance for
// the namespace of a Kubernetes model.
func EnsureProxyService(
	ctx context.Context,
	lbs labels.Set,
	name string,
	roleI rbac.RoleInterface,
	roleBindingI rbac.RoleBindingInterface,
	saI core.ServiceAccountInterface,
) error {
	lbs = labels.Merge(lbs, utils.LabelsJuju)
	pr := proxyRoleForName(name, lbs)
	roleRVal, err := roleI.Create(ctx, pr, meta.CreateOptions{})

	if k8serrors.IsAlreadyExists(err) {
		roleRVal, err = roleI.Update(ctx, pr, meta.UpdateOptions{})
	}
	if err != nil {
		return errors.Annotate(err, "creating proxy service account role")
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

	return nil

}

// CreateControllerProxy establishes the Kubernetes resources needed for
// proxying to a Juju controller. The end result of this function is a service
// account with a set of permissions that the Juju client can use for proxying
// to a controller.
func CreateControllerProxy(
	ctx context.Context,
	config ControllerProxyConfig,
	labels labels.Set,
	configI core.ConfigMapInterface,
	roleI rbac.RoleInterface,
	roleBindingI rbac.RoleBindingInterface,
	saI core.ServiceAccountInterface,
) error {
	err := EnsureProxyService(ctx, labels, config.Name, roleI, roleBindingI, saI)
	if err != nil {
		return errors.Annotate(err, "ensuring proxy service account")
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshalling proxy configmap data to json: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels,
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
