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

type ControllerProxyConfig struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	RemotePort    string `json:"remote-port"`
	TargetService string `json:"target-service"`
}

const (
	proxyConfigMapKey = "config"
)

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
				Resources: []string{"pod", "service"},
				Verbs:     []string{"list", "get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pod/portforward"},
				Verbs:     []string{"create"},
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
			proxyConfigMapKey: string(configJSON),
		},
	}

	_, err = configI.Create(context.TODO(), cm, meta.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating proxy config map: %w", err)
	}

	return nil
}
