// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
	rbac "k8s.io/client-go/kubernetes/typed/rbac/v1"
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

// CreateControllerProxy establishes the Kubernetes resources needed for
// proxying to a Juju controller. The end result of this function is a service
// account with a set of permissions that the Juju client can use for proxying
// to a controller.
func CreateControllerProxy(
	config ControllerProxyConfig,
	labels labels.Set,
	configI core.ConfigMapInterface,
	roleI rbac.RoleInterface,
	roleBindingI rbac.RoleBindingInterface,
	saI core.ServiceAccountInterface,
) error {
	role := &rbacv1.Role{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels,
			Name:   config.Name,
		},
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

	sa := &corev1.ServiceAccount{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels,
			Name:   config.Name,
		},
	}

	sa, err = saI.Create(context.TODO(), sa, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating proxy service account: %w", err)
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels,
			Name:   config.Name,
		},
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

	cm := &corev1.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Labels: labels,
			Name:   config.Name,
		},
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
