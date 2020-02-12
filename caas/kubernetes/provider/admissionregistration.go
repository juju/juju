// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	"k8s.io/api/admissionregistration/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8sannotations "github.com/juju/juju/core/annotations"
)

func (k *kubernetesClient) getAdmissionControllerLabels(appName string) map[string]string {
	return map[string]string{
		labelApplication: appName,
		labelModel:       k.namespace,
	}
}

func (k *kubernetesClient) ensureMutatingWebhookConfigurations(
	appName string, annotations k8sannotations.Annotation, cfgs map[string][]v1beta1.Webhook,
) (cleanUps []func(), err error) {
	for name, webhooks := range cfgs {
		spec := &v1beta1.MutatingWebhookConfiguration{
			ObjectMeta: v1.ObjectMeta{
				Name:        fmt.Sprintf("%s-%s", k.namespace, name), // ensure global reource MutatingWebhookConfiguration's name unique.
				Namespace:   k.namespace,
				Labels:      k.getAdmissionControllerLabels(appName),
				Annotations: annotations,
			},
			Webhooks: webhooks,
		}
		cfgCleanup, err := k.ensureMutatingWebhookConfiguration(spec)
		cleanUps = append(cleanUps, cfgCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

func (k *kubernetesClient) ensureMutatingWebhookConfiguration(cfg *v1beta1.MutatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	out, err := k.createMutatingWebhookConfiguration(cfg)
	if err == nil {
		logger.Debugf("MutatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() { k.deleteMutatingWebhookConfiguration(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listMutatingWebhookConfigurations(cfg.GetLabels())
	if err != nil {
		if errors.IsNotFound(err) {
			// cfg.Name is already used for an existing MutatingWebhookConfiguration.
			return cleanUp, errors.AlreadyExistsf("MutatingWebhookConfiguration %q", cfg.GetName())
		}
		return cleanUp, errors.Trace(err)
	}
	existingCfg, err := k.getMutatingWebhookConfiguration(cfg.GetName())
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	cfg.SetResourceVersion(existingCfg.GetResourceVersion())
	err = k.updateMutatingWebhookConfiguration(cfg)
	logger.Debugf("updating MutatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) createMutatingWebhookConfiguration(cfg *v1beta1.MutatingWebhookConfiguration) (*v1beta1.MutatingWebhookConfiguration, error) {
	purifyResource(cfg)
	out, err := k.client().Admissionregistration().MutatingWebhookConfigurations().Create(cfg)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("MutatingWebhookConfiguration %q", cfg.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getMutatingWebhookConfiguration(name string) (*v1beta1.MutatingWebhookConfiguration, error) {
	cfg, err := k.client().Admissionregistration().MutatingWebhookConfigurations().Get(name, v1.GetOptions{IncludeUninitialized: true})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("MutatingWebhookConfiguration %q", name)
		}
		return nil, errors.Trace(err)
	}
	return cfg, nil
}

func (k *kubernetesClient) updateMutatingWebhookConfiguration(cfg *v1beta1.MutatingWebhookConfiguration) error {
	_, err := k.client().Admissionregistration().MutatingWebhookConfigurations().Update(cfg)
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("MutatingWebhookConfiguration %q", cfg.GetName())
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteMutatingWebhookConfiguration(name string, uid types.UID) error {
	err := k.client().Admissionregistration().MutatingWebhookConfigurations().Delete(name, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listMutatingWebhookConfigurations(labels map[string]string) ([]v1beta1.MutatingWebhookConfiguration, error) {
	listOps := v1.ListOptions{
		LabelSelector:        labelsToSelector(labels),
		IncludeUninitialized: true,
	}
	cfgList, err := k.client().Admissionregistration().MutatingWebhookConfigurations().List(listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cfgList.Items) == 0 {
		return nil, errors.NotFoundf("MutatingWebhookConfiguration with labels %v", labels)
	}
	return cfgList.Items, nil
}

func (k *kubernetesClient) deleteMutatingWebhookConfigurations(appName string) error {
	err := k.client().Admissionregistration().MutatingWebhookConfigurations().DeleteCollection(&v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector:        labelsToSelector(k.getAdmissionControllerLabels(appName)),
		IncludeUninitialized: true,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) ensureValidatingWebhookConfigurations(
	appName string, annotations k8sannotations.Annotation, cfgs map[string][]v1beta1.Webhook,
) (cleanUps []func(), err error) {
	for name, webhooks := range cfgs {
		spec := &v1beta1.ValidatingWebhookConfiguration{
			ObjectMeta: v1.ObjectMeta{
				Name:        fmt.Sprintf("%s-%s", k.namespace, name), // ensure global reource ValidatingWebhookConfiguration's name unique.
				Namespace:   k.namespace,
				Labels:      k.getAdmissionControllerLabels(appName),
				Annotations: annotations,
			},
			Webhooks: webhooks,
		}
		cfgCleanup, err := k.ensureValidatingWebhookConfiguration(spec)
		cleanUps = append(cleanUps, cfgCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

func (k *kubernetesClient) ensureValidatingWebhookConfiguration(cfg *v1beta1.ValidatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	out, err := k.createValidatingWebhookConfiguration(cfg)
	if err == nil {
		logger.Debugf("ValidatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() { k.deleteValidatingWebhookConfiguration(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listValidatingWebhookConfigurations(cfg.GetLabels())
	if err != nil {
		if errors.IsNotFound(err) {
			// cfg.Name is already used for an existing ValidatingWebhookConfiguration.
			return cleanUp, errors.AlreadyExistsf("ValidatingWebhookConfiguration %q", cfg.GetName())
		}
		return cleanUp, errors.Trace(err)
	}
	existingCfg, err := k.getValidatingWebhookConfiguration(cfg.GetName())
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	cfg.SetResourceVersion(existingCfg.GetResourceVersion())
	err = k.updateValidatingWebhookConfiguration(cfg)
	logger.Debugf("updating ValidatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) createValidatingWebhookConfiguration(cfg *v1beta1.ValidatingWebhookConfiguration) (*v1beta1.ValidatingWebhookConfiguration, error) {
	purifyResource(cfg)
	out, err := k.client().Admissionregistration().ValidatingWebhookConfigurations().Create(cfg)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getValidatingWebhookConfiguration(name string) (*v1beta1.ValidatingWebhookConfiguration, error) {
	cfg, err := k.client().Admissionregistration().ValidatingWebhookConfigurations().Get(name, v1.GetOptions{IncludeUninitialized: true})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("ValidatingWebhookConfiguration %q", name)
		}
		return nil, errors.Trace(err)
	}
	return cfg, nil
}

func (k *kubernetesClient) updateValidatingWebhookConfiguration(cfg *v1beta1.ValidatingWebhookConfiguration) error {
	_, err := k.client().Admissionregistration().ValidatingWebhookConfigurations().Update(cfg)
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteValidatingWebhookConfiguration(name string, uid types.UID) error {
	err := k.client().Admissionregistration().ValidatingWebhookConfigurations().Delete(name, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listValidatingWebhookConfigurations(labels map[string]string) ([]v1beta1.ValidatingWebhookConfiguration, error) {
	listOps := v1.ListOptions{
		LabelSelector:        labelsToSelector(labels),
		IncludeUninitialized: true,
	}
	cfgList, err := k.client().Admissionregistration().ValidatingWebhookConfigurations().List(listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cfgList.Items) == 0 {
		return nil, errors.NotFoundf("ValidatingWebhookConfiguration with labels %v", labels)
	}
	return cfgList.Items, nil
}

func (k *kubernetesClient) deleteValidatingWebhookConfigurations(appName string) error {
	err := k.client().Admissionregistration().ValidatingWebhookConfigurations().DeleteCollection(&v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector:        labelsToSelector(k.getAdmissionControllerLabels(appName)),
		IncludeUninitialized: true,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
