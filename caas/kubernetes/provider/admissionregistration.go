// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
)

// EnsureMutatingWebhookConfiguration is used by the model operator to set up Juju's web hook.
func (k *kubernetesClient) EnsureMutatingWebhookConfiguration(cfg *admissionregistrationv1.MutatingWebhookConfiguration) (func(), error) {
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
