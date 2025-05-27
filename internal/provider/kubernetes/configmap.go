// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

// ensureConfigMap ensures a ConfigMap resource.
func (k *kubernetesClient) ensureConfigMap(ctx context.Context, cm *core.ConfigMap) (func(), error) {
	cleanUp := func() {}
	out, err := k.createConfigMap(ctx, cm)
	if err == nil {
		logger.Debugf(context.TODO(), "configmap %q created", out.GetName())
		cleanUp = func() { _ = k.deleteConfigMap(ctx, out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.Is(err, errors.AlreadyExists) {
		return cleanUp, errors.Trace(err)
	}
	err = k.updateConfigMap(ctx, cm)
	logger.Debugf(context.TODO(), "updating configmap %q", cm.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) updateConfigMap(ctx context.Context, cm *core.ConfigMap) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	_, err := k.client().CoreV1().ConfigMaps(k.namespace).Update(ctx, cm, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("configmap %q", cm.GetName())
	}
	return errors.Trace(err)
}

// getConfigMap returns a ConfigMap resource.
func (k *kubernetesClient) getConfigMap(ctx context.Context, name string) (*core.ConfigMap, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	cm, err := k.client().CoreV1().ConfigMaps(k.namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("configmap %q", name)
		}
		return nil, errors.Trace(err)
	}
	return cm, nil
}

// createConfigMap creates a ConfigMap resource.
func (k *kubernetesClient) createConfigMap(ctx context.Context, cm *core.ConfigMap) (*core.ConfigMap, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	utils.PurifyResource(cm)
	out, err := k.client().CoreV1().ConfigMaps(k.namespace).Create(ctx, cm, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("configmap %q", cm.GetName())
	}
	return out, errors.Trace(err)
}

// deleteConfigMap deletes a ConfigMap resource.
func (k *kubernetesClient) deleteConfigMap(ctx context.Context, name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().CoreV1().ConfigMaps(k.namespace).Delete(ctx, name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
