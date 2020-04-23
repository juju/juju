// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	admissionregistration "k8s.io/api/admissionregistration/v1beta1"
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
	appName string, annotations k8sannotations.Annotation, cfgs []k8sspecs.K8sMutatingWebhookSpec,
) (cleanUps []func(), err error) {
	for _, v := range cfgs {
		spec := &admissionregistration.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:        decideNameForGlobalResource(v.Meta, k.namespace),
				Namespace:   k.namespace,
				Labels:      k8slabels.Merge(v.Labels, k.getAdmissionControllerLabels(appName)),
				Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
			},
			Webhooks: v.Webhooks,
		}
		cfgCleanup, err := k.EnsureMutatingWebhookConfiguration(spec)
		cleanUps = append(cleanUps, cfgCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

// EnsureMutatingWebhookConfiguration ensures the provided mutating webhook
// exists in the given Kubernetes cluster.
// Returned func is used for cleaning up the mutating webhook when error is non
// nil.
func (k *kubernetesClient) EnsureMutatingWebhookConfiguration(cfg *admissionregistration.MutatingWebhookConfiguration) (func(), error) {
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

func (k *kubernetesClient) createMutatingWebhookConfiguration(cfg *admissionregistration.MutatingWebhookConfiguration) (*admissionregistration.MutatingWebhookConfiguration, error) {
	purifyResource(cfg)
	out, err := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(cfg)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("MutatingWebhookConfiguration %q", cfg.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getMutatingWebhookConfiguration(name string) (*admissionregistration.MutatingWebhookConfiguration, error) {
	cfg, err := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Get(name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("MutatingWebhookConfiguration %q", name)
		}
		return nil, errors.Trace(err)
	}
	return cfg, nil
}

func (k *kubernetesClient) updateMutatingWebhookConfiguration(cfg *admissionregistration.MutatingWebhookConfiguration) error {
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

func (k *kubernetesClient) listMutatingWebhookConfigurations(selector k8slabels.Selector) ([]admissionregistration.MutatingWebhookConfiguration, error) {
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

func (k *kubernetesClient) ensureValidatingWebhookConfigurations(
	appName string, annotations k8sannotations.Annotation, cfgs []k8sspecs.K8sValidatingWebhookSpec,
) (cleanUps []func(), err error) {
	for _, v := range cfgs {
		spec := &admissionregistration.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:        decideNameForGlobalResource(v.Meta, k.namespace),
				Namespace:   k.namespace,
				Labels:      k8slabels.Merge(v.Labels, k.getAdmissionControllerLabels(appName)),
				Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
			},
			Webhooks: v.Webhooks,
		}
		cfgCleanup, err := k.ensureValidatingWebhookConfiguration(spec)
		cleanUps = append(cleanUps, cfgCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

func (k *kubernetesClient) ensureValidatingWebhookConfiguration(cfg *admissionregistration.ValidatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	out, err := k.createValidatingWebhookConfiguration(cfg)
	if err == nil {
		logger.Debugf("ValidatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() { _ = k.deleteValidatingWebhookConfiguration(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	_, err = k.listValidatingWebhookConfigurations(labelSetToSelector(cfg.GetLabels()))
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

func (k *kubernetesClient) createValidatingWebhookConfiguration(cfg *admissionregistration.ValidatingWebhookConfiguration) (*admissionregistration.ValidatingWebhookConfiguration, error) {
	purifyResource(cfg)
	out, err := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Create(cfg)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getValidatingWebhookConfiguration(name string) (*admissionregistration.ValidatingWebhookConfiguration, error) {
	cfg, err := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, errors.NotFoundf("ValidatingWebhookConfiguration %q", name)
		}
		return nil, errors.Trace(err)
	}
	return cfg, nil
}

func (k *kubernetesClient) updateValidatingWebhookConfiguration(cfg *admissionregistration.ValidatingWebhookConfiguration) error {
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

func (k *kubernetesClient) listValidatingWebhookConfigurations(selector k8slabels.Selector) ([]admissionregistration.ValidatingWebhookConfiguration, error) {
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
