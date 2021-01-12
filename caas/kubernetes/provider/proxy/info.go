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
	serviceAccountSecretCADataKey    = "ca.crt"
	serviceAccountSecretNamespaceKey = "namespace"
	serviceAccountSecretTokenKey     = "token"
)

func GetControllerProxy(
	name,
	apiHost string,
	configI core.ConfigMapInterface,
	saI core.ServiceAccountInterface,
	secretI core.SecretInterface,
) (*Proxier, error) {
	cm, err := configI.Get(context.TODO(), name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("no controller proxy config found for %s", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	config := ControllerProxyConfig{}
	if err := json.Unmarshal([]byte(cm.Data[proxyConfigMapKey]), &config); err != nil {
		return nil, errors.Trace(err)
	}

	sa, err := saI.Get(context.TODO(), config.Name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("no controller proxy service account found for %s", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if secLen := len(sa.Secrets); secLen < 1 || secLen > 1 {
		return nil, fmt.Errorf("unsupported number of service account secrets: %d", secLen)
	}

	sec, err := secretI.Get(context.TODO(), sa.Secrets[0].Name, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("could not get proxy service account secret: %s", sa.Secrets[0].Name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	proxierConfig := ProxierConfig{
		APIHost:             apiHost,
		CAData:              string(sec.Data[serviceAccountSecretCADataKey]),
		Namespace:           config.Namespace,
		RemotePort:          config.RemotePort,
		Service:             config.TargetService,
		ServiceAccountToken: string(sec.Data[serviceAccountSecretTokenKey]),
	}

	return NewProxier(proxierConfig), nil
}

func HasControllerProxy(
	name string,
	saI core.ServiceAccountInterface,
) bool {
	if _, err := saI.Get(context.TODO(), name, meta.GetOptions{}); err != nil {
		return false
	}
	return true
}
