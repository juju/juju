// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func (k *kubernetesClient) getAdmissionControllerLabels(appName string) map[string]string {
	return utils.LabelsMerge(
		utils.LabelsForApp(appName, k.LabelVersion()),
		utils.LabelsForModel(k.ModelName(), k.ModelUUID(), k.ControllerUUID(), k.LabelVersion()),
	)
}

const annotationDisableNamePrefixValue = "true"

func decideNameForGlobalResource(meta k8sspecs.Meta, namespace string, labelVersion constants.LabelVersion) string {
	name := meta.Name
	key := utils.AnnotationDisableNameKey(labelVersion)
	if k8sannotations.New(meta.Annotations).Has(key, annotationDisableNamePrefixValue) {
		return name
	}
	return fmt.Sprintf("%s-%s", namespace, name)
}

func (k *kubernetesClient) ensureMutatingWebhookConfigurations(
	appName string, annotations k8sannotations.Annotation, cfgs []k8sspecs.K8sMutatingWebhook,
) (cleanUps []func(), err error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	k8sVersion, err := k.Version()
	if err != nil {
		return nil, errors.Annotate(err, "getting k8s api version")
	}
	for _, v := range cfgs {
		obj := metav1.ObjectMeta{
			Name:        decideNameForGlobalResource(v.Meta, k.namespace, k.LabelVersion()),
			Namespace:   k.namespace,
			Labels:      utils.LabelsMerge(v.Labels, k.getAdmissionControllerLabels(appName)),
			Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
		}

		logger.Infof("ensuring mutating webhook %q with version %q", obj.GetName(), v.APIVersion())
		var cfgCleanup func()
		switch v.APIVersion() {
		case k8sspecs.K8sWebhookV1:
			if k8sVersion.Major == 1 && k8sVersion.Minor < 16 {
				return cleanUps, errors.NotSupportedf("mutating webhook version %q", v.APIVersion())
			} else {
				cfgCleanup, err = k.ensureMutatingWebhookConfigurationV1(&admissionregistrationv1.MutatingWebhookConfiguration{
					ObjectMeta: obj,
					Webhooks:   toMutatingWebhookV1(v.Webhooks),
				})
			}
		case k8sspecs.K8sWebhookV1Beta1:
			if k8sVersion.Major == 1 && k8sVersion.Minor < 16 {
				cfgCleanup, err = k.ensureMutatingWebhookConfigurationV1beta1(&admissionregistrationv1beta1.MutatingWebhookConfiguration{
					ObjectMeta: obj,
					Webhooks:   toMutatingWebhookV1beta1(v.Webhooks),
				})
			} else {
				var webHooks []admissionregistrationv1.MutatingWebhook
				webHooks, err = convertToMutatingWebhookV1(v.Webhooks)
				if err != nil {
					err = errors.Annotatef(err, "cannot convert v1beta1 MutatingWebhookConfiguration to v1")
					break
				}
				cfgCleanup, err = k.ensureMutatingWebhookConfigurationV1(&admissionregistrationv1.MutatingWebhookConfiguration{
					ObjectMeta: obj,
					Webhooks:   webHooks,
				})
			}
		default:
			// This should never happen.
			return cleanUps, errors.NotSupportedf("mutating webhook version %q", v.APIVersion())
		}

		cleanUps = append(cleanUps, cfgCleanup)
		if err != nil {
			return cleanUps, errors.Annotatef(err, "ensuring mutating webhook %q with version %q", obj.GetName(), v.APIVersion())
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

func convertToMutatingWebhookV1(i []k8sspecs.K8sMutatingWebhookSpec) (o []admissionregistrationv1.MutatingWebhook, _ error) {
	for _, v := range i {
		o = append(o, k8sspecs.UpgradeK8sMutatingWebhookSpecV1Beta1(v.SpecV1Beta1))
	}
	return o, nil
}

func (k *kubernetesClient) ensureMutatingWebhookConfigurationV1(cfg *admissionregistrationv1.MutatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	api := k.client().AdmissionregistrationV1().MutatingWebhookConfigurations()
	out, err := api.Create(context.TODO(), cfg, metav1.CreateOptions{})
	if err == nil {
		logger.Debugf("MutatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() {
			_ = api.Delete(context.TODO(), out.GetName(), utils.NewPreconditionDeleteOptions(out.GetUID()))
		}
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}

	existing, err := api.Get(context.TODO(), cfg.GetName(), metav1.GetOptions{})
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	existingLabelVersion, err := utils.DetectModelMetaLabelVersion(existing.ObjectMeta, k.modelName, k.modelUUID, k.controllerUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "ensuring MutatingWebhookConfiguration %q with labels %v ", cfg.GetName(), existing.Labels)
	}
	if existingLabelVersion < k.labelVersion {
		logger.Warningf("updating label version for existing MutatingWebhookConfiguration %q from %d to %d ", cfg.GetName(), existingLabelVersion, k.labelVersion)
	}

	cfg.SetResourceVersion(existing.GetResourceVersion())
	_, err = api.Update(context.TODO(), cfg, metav1.UpdateOptions{})
	logger.Debugf("updating MutatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) EnsureMutatingWebhookConfiguration(cfg *admissionregistrationv1.MutatingWebhookConfiguration) (func(), error) {
	return k.ensureMutatingWebhookConfigurationV1(cfg)
}

// EnsureMutatingWebhookConfiguration ensures the provided mutating webhook
// exists in the given Kubernetes cluster.
// Returned func is used for cleaning up the mutating webhook when error is non
// nil.
func (k *kubernetesClient) ensureMutatingWebhookConfigurationV1beta1(cfg *admissionregistrationv1beta1.MutatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	api := k.client().AdmissionregistrationV1beta1().MutatingWebhookConfigurations()
	out, err := api.Create(context.TODO(), cfg, metav1.CreateOptions{})
	if err == nil {
		logger.Debugf("MutatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() {
			_ = api.Delete(context.TODO(), out.GetName(), utils.NewPreconditionDeleteOptions(out.GetUID()))
		}
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	existingItems, err := api.List(context.TODO(), metav1.ListOptions{
		LabelSelector: utils.LabelsToSelector(cfg.GetLabels()).String(),
	})
	if k8serrors.IsNotFound(err) || existingItems == nil || len(existingItems.Items) == 0 {
		// cfg.Name is already used for an existing MutatingWebhookConfiguration.
		return cleanUp, errors.AlreadyExistsf("MutatingWebhookConfiguration %q", cfg.GetName())
	}
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	existingCfg, err := api.Get(context.TODO(), cfg.GetName(), metav1.GetOptions{})
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	cfg.SetResourceVersion(existingCfg.GetResourceVersion())
	_, err = api.Update(context.TODO(), cfg, metav1.UpdateOptions{})
	logger.Debugf("updating MutatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) listMutatingWebhookConfigurations(selector k8slabels.Selector) ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	listOps := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	cfgList, err := k.client().AdmissionregistrationV1().MutatingWebhookConfigurations().List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cfgList.Items) == 0 {
		return nil, errors.NotFoundf("MutatingWebhookConfiguration with selector %q", selector)
	}
	return cfgList.Items, nil
}

func (k *kubernetesClient) deleteMutatingWebhookConfigurations(selector k8slabels.Selector) error {
	err := k.client().AdmissionregistrationV1().MutatingWebhookConfigurations().DeleteCollection(context.TODO(), metav1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteMutatingWebhookConfigurationsForApp(appName string) error {
	selector := utils.LabelsToSelector(k.getAdmissionControllerLabels(appName))
	return errors.Trace(k.deleteMutatingWebhookConfigurations(selector))
}

func (k *kubernetesClient) ensureValidatingWebhookConfigurations(
	appName string, annotations k8sannotations.Annotation, cfgs []k8sspecs.K8sValidatingWebhook,
) (cleanUps []func(), err error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	k8sVersion, err := k.Version()
	if err != nil {
		return nil, errors.Annotate(err, "getting k8s api version")
	}
	for _, v := range cfgs {
		obj := metav1.ObjectMeta{
			Name:        decideNameForGlobalResource(v.Meta, k.namespace, k.LabelVersion()),
			Namespace:   k.namespace,
			Labels:      utils.LabelsMerge(v.Labels, k.getAdmissionControllerLabels(appName)),
			Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
		}

		logger.Infof("ensuring validating webhook %q with version %q", obj.GetName(), v.APIVersion())
		var cfgCleanup func()
		switch v.APIVersion() {
		case k8sspecs.K8sWebhookV1:
			if k8sVersion.Major == 1 && k8sVersion.Minor < 16 {
				return cleanUps, errors.NotSupportedf("validating webhook version %q", v.APIVersion())
			} else {
				cfgCleanup, err = k.ensureValidatingWebhookConfigurationV1(&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: obj,
					Webhooks:   toValidatingWebhookV1(v.Webhooks),
				})
			}
		case k8sspecs.K8sWebhookV1Beta1:
			if k8sVersion.Major == 1 && k8sVersion.Minor < 16 {
				cfgCleanup, err = k.ensureValidatingWebhookConfigurationV1beta1(&admissionregistrationv1beta1.ValidatingWebhookConfiguration{
					ObjectMeta: obj,
					Webhooks:   toValidatingWebhookV1beta1(v.Webhooks),
				})
			} else {
				var webHooks []admissionregistrationv1.ValidatingWebhook
				webHooks, err = convertToValidatingWebhookV1(v.Webhooks)
				if err != nil {
					err = errors.Annotatef(err, "cannot convert v1beta1 ValidatingWebhookConfiguration to v1")
					break
				}
				cfgCleanup, err = k.ensureValidatingWebhookConfigurationV1(&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: obj,
					Webhooks:   webHooks,
				})
			}
		default:
			// This should never happen.
			return cleanUps, errors.NotSupportedf("validating webhook version %q", v.APIVersion())
		}
		cleanUps = append(cleanUps, cfgCleanup)
		if err != nil {
			return cleanUps, errors.Annotatef(err, "ensuring validating webhook %q with version %q", obj.GetName(), v.APIVersion())
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

func convertToValidatingWebhookV1(i []k8sspecs.K8sValidatingWebhookSpec) (o []admissionregistrationv1.ValidatingWebhook, _ error) {
	for _, v := range i {
		o = append(o, k8sspecs.UpgradeK8sValidatingWebhookSpecV1Beta1(v.SpecV1Beta1))
	}
	return o, nil
}

func (k *kubernetesClient) ensureValidatingWebhookConfigurationV1(cfg *admissionregistrationv1.ValidatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	api := k.client().AdmissionregistrationV1().ValidatingWebhookConfigurations()
	out, err := api.Create(context.TODO(), cfg, metav1.CreateOptions{})
	if err == nil {
		logger.Debugf("ValidatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() {
			_ = api.Delete(context.TODO(), out.GetName(), utils.NewPreconditionDeleteOptions(out.GetUID()))
		}
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}

	existingItems, err := api.List(context.TODO(), metav1.ListOptions{
		LabelSelector: utils.LabelsToSelector(cfg.GetLabels()).String(),
	})
	if k8serrors.IsNotFound(err) || len(existingItems.Items) == 0 {
		// cfg.Name is already used for an existing ValidatingWebhookConfiguration.
		return cleanUp, errors.AlreadyExistsf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	existingCfg, err := api.Get(context.TODO(), cfg.GetName(), metav1.GetOptions{})
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	cfg.SetResourceVersion(existingCfg.GetResourceVersion())
	_, err = api.Update(context.TODO(), cfg, metav1.UpdateOptions{})
	logger.Debugf("updating ValidatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) ensureValidatingWebhookConfigurationV1beta1(cfg *admissionregistrationv1beta1.ValidatingWebhookConfiguration) (func(), error) {
	cleanUp := func() {}
	api := k.client().AdmissionregistrationV1beta1().ValidatingWebhookConfigurations()
	out, err := api.Create(context.TODO(), cfg, metav1.CreateOptions{})
	if err == nil {
		logger.Debugf("ValidatingWebhookConfiguration %q created", out.GetName())
		cleanUp = func() {
			_ = api.Delete(context.TODO(), out.GetName(), utils.NewPreconditionDeleteOptions(out.GetUID()))
		}
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}

	existingItems, err := api.List(context.TODO(), metav1.ListOptions{
		LabelSelector: utils.LabelsToSelector(cfg.GetLabels()).String(),
	})
	if k8serrors.IsNotFound(err) || len(existingItems.Items) == 0 {
		// cfg.Name is already used for an existing ValidatingWebhookConfiguration.
		return cleanUp, errors.AlreadyExistsf("ValidatingWebhookConfiguration %q", cfg.GetName())
	}
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	existingCfg, err := api.Get(context.TODO(), cfg.GetName(), metav1.GetOptions{})
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	cfg.SetResourceVersion(existingCfg.GetResourceVersion())
	_, err = api.Update(context.TODO(), cfg, metav1.UpdateOptions{})
	logger.Debugf("updating ValidatingWebhookConfiguration %q", cfg.GetName())
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) listValidatingWebhookConfigurations(selector k8slabels.Selector) ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	listOps := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	cfgList, err := k.client().AdmissionregistrationV1().ValidatingWebhookConfigurations().List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(cfgList.Items) == 0 {
		return nil, errors.NotFoundf("ValidatingWebhookConfiguration with selector %q", selector)
	}
	return cfgList.Items, nil
}

func (k *kubernetesClient) deleteValidatingWebhookConfigurations(selector k8slabels.Selector) error {
	err := k.client().AdmissionregistrationV1().ValidatingWebhookConfigurations().DeleteCollection(context.TODO(), metav1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteValidatingWebhookConfigurationsForApp(appName string) error {
	selector := utils.LabelsToSelector(k.getAdmissionControllerLabels(appName))
	return errors.Trace(k.deleteValidatingWebhookConfigurations(selector))
}
