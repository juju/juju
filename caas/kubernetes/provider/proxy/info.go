// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	core "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	serviceAccountSecretCADataKey = "ca.crt"
	serviceAccountSecretTokenKey  = "token"
)

// GetProxyConfig is as config input to the GetProxy function. It describes
// basic properties to seed the returned Proxier object with.
type GetProxyConfig struct {
	// APIHost to expect when performing SNI with the kubernetes API.
	APIHost string

	// Namespace is the namespace the proxied targets resides in.
	Namespace string

	// RemotePort to proxy to.
	RemotePort string

	// The service in the above Namespace to proxy onto.
	Service string
}

// GetProxy attempts to create a Proxier from the named resources using the
// found service account and associated secret.
func GetProxy(
	name string,
	config GetProxyConfig,
	saI core.ServiceAccountInterface,
	secretI core.SecretInterface,
) (*Proxier, error) {
	sa, err := saI.Get(context.TODO(), name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("proxy service account for %s", name)
	} else if err != nil {
		return nil, errors.Annotatef(err, "proxy service account for %s", name)
	}

	if len(sa.Secrets) == 0 {
		return nil, fmt.Errorf("no secret created for service account %q", sa.GetName())
	}

	sec, err := secretI.Get(context.TODO(), sa.Secrets[0].Name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("could not get proxy service account secret: %q", sa.Secrets[0].Name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	proxierConfig := ProxierConfig{
		APIHost:             config.APIHost,
		CAData:              string(sec.Data[serviceAccountSecretCADataKey]),
		Namespace:           config.Namespace,
		RemotePort:          config.RemotePort,
		Service:             config.Service,
		ServiceAccountToken: string(sec.Data[serviceAccountSecretTokenKey]),
	}

	return NewProxier(proxierConfig), nil
}

// GetControllerProxy returns the proxier for the controller specified by name.
func GetControllerProxy(
	name,
	apiHost string,
	configI core.ConfigMapInterface,
	saI core.ServiceAccountInterface,
	secretI core.SecretInterface,
) (*Proxier, error) {
	cm, err := configI.Get(context.TODO(), name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("controller proxy config %s", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	config := ControllerProxyConfig{}
	if err := json.Unmarshal([]byte(cm.Data[ProxyConfigMapKey]), &config); err != nil {
		return nil, errors.Trace(err)
	}

	return GetProxy(config.Name, GetProxyConfig{
		APIHost:    apiHost,
		Namespace:  config.Namespace,
		RemotePort: config.RemotePort,
		Service:    config.TargetService,
	}, saI, secretI)
}

// HasControllerProxy indicates if a controller proxy exists for the supplied
// name and namespace.
func HasControllerProxy(
	name string,
	configI core.ConfigMapInterface,
) (bool, error) {
	_, err := configI.Get(context.TODO(), name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}
