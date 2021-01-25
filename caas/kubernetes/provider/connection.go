// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	k8sproxy "github.com/juju/juju/caas/kubernetes/provider/proxy"
	"github.com/juju/juju/proxy"
)

func (k *kubernetesClient) ConnectionProxyInfo() (proxy.Proxier, error) {
	p, err := k8sproxy.GetControllerProxy(
		getBootstrapResourceName(JujuControllerStackName, proxyResourceName),
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
