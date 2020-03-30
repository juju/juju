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
	}
}

func (k *kubernetesClient) ensureConfigMaps(
	appName string,
	annotations map[string]string,
	cms map[string]specs.ConfigMap,
) (cleanUps []func(), _ error) {
	for name, v := range cms {
		spec := &core.ConfigMap{
			ObjectMeta: v1.ObjectMeta{
				Name:        name,
				Namespace:   k.namespace,
				Labels:      k.getConfigMapLabels(appName),
				Annotations: annotations,
			},
			Data: v,
		}
		cmCleanup, err := k.ensureConfigMap(spec)
		cleanUps = append(cleanUps, cmCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

// ensureConfigMapLegacy is a tmp fix for upgrading configmap(no proper labels) created in 2.6.
// TODO(caas): remove this and use "updateConfigMap" once `modelupgrader` supports CaaS models.
func (k *kubernetesClient) ensureConfigMapLegacy(cm *core.ConfigMap) (cleanUp func(), err error) {
	cleanUp = func() {}
	api := k.client().CoreV1().ConfigMaps(k.namespace)
	_, err = api.Update(cm)
	if k8serrors.IsNotFound(err) {
		var out *core.ConfigMap
		if out, err = api.Create(cm); err == nil {
			logger.Debugf("configmap %q created", out.GetName())
			cleanUp = func() { _ = k.deleteConfigMap(out.GetName(), out.GetUID()) }
			return cleanUp, nil
		}
	}
	return cleanUp, errors.Trace(err)
}

// ensureConfigMap ensures a ConfigMap resource.
func (k *kubernetesClient) ensureConfigMap(cm *core.ConfigMap) (func(), error) {
	cleanUp := func() {}
	out, err := k.createConfigMap(cm)
	if err == nil {
		logger.Debugf("configmap %q created", out.GetName())
		cleanUp = func() { _ = k.deleteConfigMap(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listConfigMaps(cm.GetLabels())
	if err != nil {
		if errors.IsNotFound(err) {
			// configmap name is already used for an existing secret.
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
func (k *kubernetesClient) getConfigMap(name string) (*core.ConfigMap, error) {
	cm, err := k.client().CoreV1().ConfigMaps(k.namespace).Get(name, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("configmap %q", name)
		}
		return nil, errors.Trace(err)
	}
	return cm, nil
}

// createConfigMap creates a ConfigMap resource.
func (k *kubernetesClient) createConfigMap(cm *core.ConfigMap) (*core.ConfigMap, error) {
	purifyResource(cm)
	out, err := k.client().CoreV1().ConfigMaps(k.namespace).Create(cm)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("configmap %q", cm.GetName())
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
		LabelSelector: labelSetToSelector(labels).String(),
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

func (k *kubernetesClient) deleteConfigMaps(appName string) error {
	err := k.client().CoreV1().ConfigMaps(k.namespace).DeleteCollection(&v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector: labelSetToSelector(k.getConfigMapLabels(appName)).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
