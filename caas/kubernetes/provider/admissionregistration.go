// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func (k *kubernetesClient) getAdmissionControllerLabels(appName string) map[string]string {
	return map[string]string{
		labelApplication: appName,
		labelModel:       k.namespace,
	}
}

var annotationDisableNamePrefixKey = jujuAnnotationKey("disable-name-prefix")

const annotationDisableNamePrefixValue = "true"

func decideNameForGlobalResource(meta k8sspecs.Meta, namespace string) string {
	name := meta.Name
	if k8sannotations.New(meta.Annotations).Has(annotationDisableNamePrefixKey, annotationDisableNamePrefixValue) {
		return name
	}
	return fmt.Sprintf("%s-%s", namespace, name)
}

func (k *kubernetesClient) ensureMutatingWebhookConfigurations(
	appName string, annotations k8sannotations.Annotation, cfgs []k8sspecs.K8sMutatingWebhook,
) (cleanUps []func(), err error) {
	for _, v := range cfgs {
		obj := metav1.ObjectMeta{
			Name:        decideNameForGlobalResource(v.Meta, k.namespace),
			Namespace:   k.namespace,
			Labels:      k8slabels.Merge(v.Labels, k.getAdmissionControllerLabels(appName)),
			Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
		}

		var cfgCleanup func()
		switch v.Version {
		case k8sspecs.K8sWebhookV1:
			cfgCleanup, err = k.ensureMutatingWebhookConfigurationV1(&admissionregistrationv1.MutatingWebhookConfiguration{
				ObjectMeta: obj,
				Webhooks:   toMutatingWebhookV1(v.Webhooks),
			})
		case k8sspecs.K8sWebhookV1Beta1:
			cfgCleanup, err = k.ensureMutatingWebhookConfigurationV1beta1(&admissionregistrationv1beta1.MutatingWebhookConfiguration{
				ObjectMeta: obj,
				Webhooks:   toMutatingWebhookV1beta1(v.Webhooks),
			})
		default:
			// This should never happen.
			return cleanUps, errors.NotSupportedf("mutating webhook version %q", v.Version)
		}

		cleanUps = append(cleanUps, cfgCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

func toMutatingWebhookV1beta1(i []k8sspecs.K8sMutatingWebhookSpec) (o []admissionregistrationv1beta1.MutatingWebhook) {
	for _, v := range i {
		o = append(o, v.SpecV1Beta1)
	}
	return o
}

func toMutatingWebhookV1(i []k8sspecs.K8sMutatingWebhookSpec) (o []admissionregistrationv1.MutatingWebhook) {
	for _, v := range i {
		o = append(o, v.SpecV1)
	}
	return o
}

func (k *kubernetesClient) ensureMutatingWebhookConfigurationV1(cfg *admissionregistrationv1.MutatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	api := k.client().AdmissionregistrationV1().MutatingWebhookConfigurations()
	out, err := api.Create(cfg)
	if err == nil {
		logger.Debugf("MutatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() { _ = api.Delete(out.GetName(), newPreconditionDeleteOptions(out.GetUID())) }
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}

	existingItems, err := api.List(metav1.ListOptions{
		LabelSelector: labelSetToSelector(cfg.GetLabels()).String(),
	})
	if k8serrors.IsNotFound(err) || len(existingItems.Items) == 0 {
		// cfg.Name is already used for an existing MutatingWebhookConfiguration.
		return cleanUp, errors.AlreadyExistsf("MutatingWebhookConfiguration %q", cfg.GetName())
	}
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	existingCfg, err := api.Get(cfg.GetName(), metav1.GetOptions{})
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	cfg.SetResourceVersion(existingCfg.GetResourceVersion())
	_, err = api.Update(cfg)
	logger.Debugf("updating MutatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) ensureMutatingWebhookConfigurationV1beta1(cfg *admissionregistrationv1beta1.MutatingWebhookConfiguration) (func(), error) {
	return k.EnsureMutatingWebhookConfiguration(cfg)
}

// EnsureMutatingWebhookConfiguration ensures the provided mutating webhook
// exists in the given Kubernetes cluster.
// Returned func is used for cleaning up the mutating webhook when error is non
// nil.
func (k *kubernetesClient) EnsureMutatingWebhookConfiguration(cfg *admissionregistrationv1beta1.MutatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	out, err := k.createMutatingWebhookConfiguration(cfg)
	if err == nil {
		logger.Debugf("MutatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() { _ = k.deleteMutatingWebhookConfiguration(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listMutatingWebhookConfigurations(labelSetToSelector(cfg.GetLabels()))
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

func (k *kubernetesClient) createMutatingWebhookConfiguration(cfg *admissionregistrationv1beta1.MutatingWebhookConfiguration) (*admissionregistrationv1beta1.MutatingWebhookConfiguration, error) {
	purifyResource(cfg)
	out, err := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(cfg)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("MutatingWebhookConfiguration %q", cfg.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getMutatingWebhookConfiguration(name string) (*admissionregistrationv1beta1.MutatingWebhookConfiguration, error) {
	cfg, err := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Get(name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("MutatingWebhookConfiguration %q", name)
		}
		return nil, errors.Trace(err)
	}
	return cfg, nil
}

func (k *kubernetesClient) updateMutatingWebhookConfiguration(cfg *admissionregistrationv1beta1.MutatingWebhookConfiguration) error {
	_, err := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Update(cfg)
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("MutatingWebhookConfiguration %q", cfg.GetName())
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteMutatingWebhookConfiguration(name string, uid types.UID) error {
	err := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Delete(name, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listMutatingWebhookConfigurations(selector k8slabels.Selector) ([]admissionregistrationv1beta1.MutatingWebhookConfiguration, error) {
	listOps := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	cfgList, err := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations().List(listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cfgList.Items) == 0 {
		return nil, errors.NotFoundf("MutatingWebhookConfiguration with selector %q", selector)
	}
	return cfgList.Items, nil
}

func (k *kubernetesClient) deleteMutatingWebhookConfigurations(selector k8slabels.Selector) error {
	err := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations().DeleteCollection(&metav1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteMutatingWebhookConfigurationsForApp(appName string) error {
	selector := labelSetToSelector(k.getAdmissionControllerLabels(appName))
	return errors.Trace(k.deleteMutatingWebhookConfigurations(selector))
}

func toValidatingWebhook(i []k8sspecs.K8sValidatingWebhookSpec) (o []admissionregistrationv1beta1.ValidatingWebhook) {
	for _, v := range i {
		o = append(o, v.SpecV1Beta1)
	}
	return o
}

func (k *kubernetesClient) ensureValidatingWebhookConfigurations(
	appName string, annotations k8sannotations.Annotation, cfgs []k8sspecs.K8sValidatingWebhook,
) (cleanUps []func(), err error) {
	for _, v := range cfgs {
		obj := metav1.ObjectMeta{
			Name:        decideNameForGlobalResource(v.Meta, k.namespace),
			Namespace:   k.namespace,
			Labels:      k8slabels.Merge(v.Labels, k.getAdmissionControllerLabels(appName)),
			Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
		}

		var cfgCleanup func()
		switch v.Version {
		case k8sspecs.K8sWebhookV1:
			cfgCleanup, err = k.ensureValidatingWebhookConfigurationV1(&admissionregistrationv1.ValidatingWebhookConfiguration{
				ObjectMeta: obj,
				Webhooks:   toValidatingWebhookV1(v.Webhooks),
			})
		case k8sspecs.K8sWebhookV1Beta1:
			cfgCleanup, err = k.ensureValidatingWebhookConfigurationV1beta1(&admissionregistrationv1beta1.ValidatingWebhookConfiguration{
				ObjectMeta: obj,
				Webhooks:   toValidatingWebhookV1beta1(v.Webhooks),
			})
		default:
			// This should never happen.
			return cleanUps, errors.NotSupportedf("mutating webhook version %q", v.Version)
		}
		cleanUps = append(cleanUps, cfgCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

func toValidatingWebhookV1beta1(i []k8sspecs.K8sValidatingWebhookSpec) (o []admissionregistrationv1beta1.ValidatingWebhook) {
	for _, v := range i {
		o = append(o, v.SpecV1Beta1)
	}
	return o
}

func toValidatingWebhookV1(i []k8sspecs.K8sValidatingWebhookSpec) (o []admissionregistrationv1.ValidatingWebhook) {
	for _, v := range i {
		o = append(o, v.SpecV1)
	}
	return o
}

func (k *kubernetesClient) ensureValidatingWebhookConfigurationV1(cfg *admissionregistrationv1.ValidatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	api := k.client().AdmissionregistrationV1().ValidatingWebhookConfigurations()
	out, err := api.Create(cfg)
	if err == nil {
		logger.Debugf("ValidatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() { _ = api.Delete(out.GetName(), newPreconditionDeleteOptions(out.GetUID())) }
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}

	existingItems, err := api.List(metav1.ListOptions{
		LabelSelector: labelSetToSelector(cfg.GetLabels()).String(),
	})
	if k8serrors.IsNotFound(err) || len(existingItems.Items) == 0 {
		// cfg.Name is already used for an existing ValidatingWebhookConfiguration.
		return cleanUp, errors.AlreadyExistsf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	existingCfg, err := api.Get(cfg.GetName(), metav1.GetOptions{})
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	cfg.SetResourceVersion(existingCfg.GetResourceVersion())
	_, err = api.Update(cfg)
	logger.Debugf("updating ValidatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) ensureValidatingWebhookConfigurationV1beta1(cfg *admissionregistrationv1beta1.ValidatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	api := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations()
	out, err := api.Create(cfg)
	if err == nil {
		logger.Debugf("ValidatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() { _ = api.Delete(out.GetName(), newPreconditionDeleteOptions(out.GetUID())) }
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}

	existingItems, err := api.List(metav1.ListOptions{
		LabelSelector: labelSetToSelector(cfg.GetLabels()).String(),
	})
	if k8serrors.IsNotFound(err) || len(existingItems.Items) == 0 {
		// cfg.Name is already used for an existing ValidatingWebhookConfiguration.
		return cleanUp, errors.AlreadyExistsf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	existingCfg, err := api.Get(cfg.GetName(), metav1.GetOptions{})
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	cfg.SetResourceVersion(existingCfg.GetResourceVersion())
	_, err = api.Update(cfg)
	logger.Debugf("updating ValidatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) createValidatingWebhookConfiguration(cfg *admissionregistrationv1beta1.ValidatingWebhookConfiguration) (*admissionregistrationv1beta1.ValidatingWebhookConfiguration, error) {
	purifyResource(cfg)
	out, err := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Create(cfg)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getValidatingWebhookConfiguration(name string) (*admissionregistrationv1beta1.ValidatingWebhookConfiguration, error) {
	cfg, err := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("ValidatingWebhookConfiguration %q", name)
		}
		return nil, errors.Trace(err)
	}
	return cfg, nil
}

func (k *kubernetesClient) updateValidatingWebhookConfiguration(cfg *admissionregistrationv1beta1.ValidatingWebhookConfiguration) error {
	_, err := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Update(cfg)
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteValidatingWebhookConfiguration(name string, uid types.UID) error {
	err := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Delete(name, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listValidatingWebhookConfigurations(selector k8slabels.Selector) ([]admissionregistrationv1beta1.ValidatingWebhookConfiguration, error) {
	listOps := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	cfgList, err := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().List(listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cfgList.Items) == 0 {
		return nil, errors.NotFoundf("ValidatingWebhookConfiguration with selector %q", selector)
	}
	return cfgList.Items, nil
}

func (k *kubernetesClient) deleteValidatingWebhookConfigurations(selector k8slabels.Selector) error {
	err := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().DeleteCollection(&metav1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteValidatingWebhookConfigurationsForApp(appName string) error {
	selector := labelSetToSelector(k.getAdmissionControllerLabels(appName))
	return errors.Trace(k.deleteValidatingWebhookConfigurations(selector))
}
