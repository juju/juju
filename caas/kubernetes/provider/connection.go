// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	k8sproxy "github.com/juju/juju/caas/kubernetes/provider/proxy"
	"github.com/juju/juju/proxy"
)

func (k *kubernetesClient) ConnectionProxyInfo() (proxy.Proxier, error) {
	return k8sproxy.GetControllerProxy(
		getBootstrapResourceName(JujuControllerStackName, proxyResourceName),
		k.k8sCfgUnlocked.Host,
		k.client().CoreV1().ConfigMaps(k.GetCurrentNamespace()),
		k.client().CoreV1().ServiceAccounts(k.GetCurrentNamespace()),
		k.client().CoreV1().Secrets(k.GetCurrentNamespace()),
	)
}
