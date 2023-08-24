// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"k8s.io/apimachinery/pkg/labels"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sproxy "github.com/juju/juju/caas/kubernetes/provider/proxy"
	"github.com/juju/juju/internal/proxy"
)

// ProxyToApplication attempts to construct a Juju proxier for use in proxying
// connections to the specified application. This assume the presence of a
// corresponding service for the application.
func (k *kubernetesClient) ProxyToApplication(appName, remotePort string) (proxy.Proxier, error) {
	svc, err := findServiceForApplication(
		context.TODO(),
		k.client().CoreV1().Services(k.namespace),
		appName,
		k.IsLegacyLabels())
	if err != nil {
		return nil, errors.Annotatef(err, "finding service to proxy to for application %s", appName)
	}

	proxyName := fmt.Sprintf("%s-model-proxy", k.CurrentModel())
	err = k8sproxy.EnsureProxyService(
		context.Background(),
		labels.Set{},
		proxyName,
		k.clock,
		k.client().RbacV1().Roles(k.GetCurrentNamespace()),
		k.client().RbacV1().RoleBindings(k.GetCurrentNamespace()),
		k.client().CoreV1().ServiceAccounts(k.GetCurrentNamespace()),
		k.client().CoreV1().Secrets(k.GetCurrentNamespace()),
	)
	if err != nil {
		return nil, errors.Annotatef(err, "ensuring proxy service for application %s", appName)
	}

	err = k8sproxy.WaitForProxyService(
		context.Background(),
		proxyName,
		k.client().CoreV1().ServiceAccounts(k.GetCurrentNamespace()),
	)
	if err != nil {
		return nil, errors.Annotatef(err, "waiting for proxy service for application %s", appName)
	}

	config := k8sproxy.GetProxyConfig{
		APIHost:    k.k8sCfgUnlocked.Host,
		Namespace:  k.GetCurrentNamespace(),
		RemotePort: remotePort,
		Service:    svc.Name,
	}

	return k8sproxy.GetProxy(
		proxyName,
		config,
		k.client().CoreV1().ServiceAccounts(k.GetCurrentNamespace()),
		k.client().CoreV1().Secrets(k.GetCurrentNamespace()),
	)
}

// ConnectionProxyInfo provides the means for getting a proxier onto a Juju
// controller deployed in this provider.
func (k *kubernetesClient) ConnectionProxyInfo() (proxy.Proxier, error) {
	p, err := k8sproxy.GetControllerProxy(
		getBootstrapResourceName(k8sconstants.JujuControllerStackName, proxyResourceName),
		k.k8sCfgUnlocked.Host,
		k.client().CoreV1().ConfigMaps(k.GetCurrentNamespace()),
		k.client().CoreV1().ServiceAccounts(k.GetCurrentNamespace()),
		k.client().CoreV1().Secrets(k.GetCurrentNamespace()),
	)

	// If an error occurred return a nil to avoid converting the nil
	// *Proxier into a typed nil which allows MarshalYAML to be called
	// against a nil value which effectively causes a nil pointer
	// dereference.
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p, nil
}
