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
func (k *kubernetesClient) ProxyToApplication(ctx context.Context, appName, remotePort string) (proxy.Proxier, error) {
	svc, err := findServiceForApplication(
		ctx,
		k.client().CoreV1().Services(k.namespace),
		appName,
		k.LabelVersion())
	if err != nil {
		return nil, errors.Annotatef(err, "finding service to proxy to for application %s", appName)
	}

	proxyName := fmt.Sprintf("%s-model-proxy", k.ModelName())
	err = k8sproxy.EnsureProxyService(
		context.Background(),
		labels.Set{},
		proxyName,
		k.clock,
		k.client().RbacV1().Roles(k.Namespace()),
		k.client().RbacV1().RoleBindings(k.Namespace()),
		k.client().CoreV1().ServiceAccounts(k.Namespace()),
		k.client().CoreV1().Secrets(k.Namespace()),
	)
	if err != nil {
		return nil, errors.Annotatef(err, "ensuring proxy service for application %s", appName)
	}

	err = k8sproxy.WaitForProxyService(
		context.Background(),
		proxyName,
		k.client().CoreV1().ServiceAccounts(k.Namespace()),
	)
	if err != nil {
		return nil, errors.Annotatef(err, "waiting for proxy service for application %s", appName)
	}

	config := k8sproxy.GetProxyConfig{
		APIHost:    k.k8sCfgUnlocked.Host,
		Namespace:  k.Namespace(),
		RemotePort: remotePort,
		Service:    svc.Name,
	}

	return k8sproxy.GetProxy(
		ctx,
		proxyName,
		config,
		k.client().CoreV1().ServiceAccounts(k.Namespace()),
		k.client().CoreV1().Secrets(k.Namespace()),
	)
}

// ConnectionProxyInfo provides the means for getting a proxier onto a Juju
// controller deployed in this provider.
func (k *kubernetesClient) ConnectionProxyInfo(ctx context.Context) (proxy.Proxier, error) {
	p, err := k8sproxy.GetControllerProxy(
		ctx,
		getBootstrapResourceName(k8sconstants.JujuControllerStackName, proxyResourceName),
		k.k8sCfgUnlocked.Host,
		k.client().CoreV1().ConfigMaps(k.Namespace()),
		k.client().CoreV1().ServiceAccounts(k.Namespace()),
		k.client().CoreV1().Secrets(k.Namespace()),
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
