// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas/specs"
)

func (k *kubernetesClient) getConfigMapLabels(appName string) map[string]string {
	return map[string]string{
		labelApplication: appName,
		labelModel:       k.namespace,
	}
}

func (k *kubernetesClient) ensureConfigMaps(appName string, cms map[string]specs.ConfigMap) (cleanUps []func(), _ error) {
	for name, v := range cms {
		spec := &core.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: k.namespace,
				Labels:    k.getConfigMapLabels(appName),
			},
			Data: v,
			// BinaryData: // TODO: do we need support binanry data????????????
		}
		cmCleanup, err := k.ensureConfigMap(spec)
		cleanUps = append(cleanUps, cmCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

// ensureConfigMap ensures a ConfigMap resource.
func (k *kubernetesClient) ensureConfigMap(cm *core.ConfigMap) (cleanUp func(), _ error) {
	out, err := k.createConfigMap(cm)
	if err == nil {
		logger.Debugf("configmap %q created", out.GetName())
		cleanUp = func() { k.deleteConfigMap(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listConfigMaps(cm.GetLabels())
	if err != nil {
		if errors.IsNotFound(err) {
			// cm.Name is already used for an existing secret.
			return cleanUp, errors.AlreadyExistsf("configmap %q", cm.GetName())
		}
		return cleanUp, errors.Trace(err)
	}
	err = k.updateConfigMap(cm)
	logger.Debugf("updating configmap %q", cm.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) updateConfigMap(cm *core.ConfigMap) error {
	_, err := k.client().CoreV1().ConfigMaps(k.namespace).Update(cm)
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("configmap %q", cm.GetName())
	}
	return errors.Trace(err)
}

// getConfigMap returns a ConfigMap resource.
func (k *kubernetesClient) getConfigMap(cmName string) (*core.ConfigMap, error) {
	cm, err := k.client().CoreV1().ConfigMaps(k.namespace).Get(cmName, v1.GetOptions{IncludeUninitialized: true})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("configmap %q", cmName)
		}
		return nil, errors.Trace(err)
	}
	return cm, nil
}

// createConfigMap creates a ConfigMap resource.
func (k *kubernetesClient) createConfigMap(cm *core.ConfigMap) (*core.ConfigMap, error) {
	out, err := k.client().CoreV1().ConfigMaps(k.namespace).Create(cm)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("configmap %q", out.GetName())
	}
	return out, errors.Trace(err)
}

// deleteConfigMap deletes a ConfigMap resource.
func (k *kubernetesClient) deleteConfigMap(name string, uid types.UID) error {
	err := k.client().CoreV1().ConfigMaps(k.namespace).Delete(name, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listConfigMaps(labels map[string]string) ([]core.ConfigMap, error) {
	listOps := v1.ListOptions{
		LabelSelector:        labelsToSelector(labels),
		IncludeUninitialized: true,
	}
	cmList, err := k.client().CoreV1().ConfigMaps(k.namespace).List(listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cmList.Items) == 0 {
		return nil, errors.NotFoundf("configmap with labels %v", labels)
	}
	return cmList.Items, nil
}
