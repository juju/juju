// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func (k *kubernetesClient) getIngressLabels(appName string) map[string]string {
	return utils.LabelsForApp(appName, k.IsLegacyLabels())
}

// TODO(caas): should we overwrite the existing `juju expose` created ingress if user runs upgrade-charm with new ingress podspec v2.
// https://bugs.launchpad.net/juju/+bug/1854123
func (k *kubernetesClient) ensureIngressResources(
	appName string, annotations k8sannotations.Annotation, ingSpecs []k8sspecs.K8sIngress,
) (cleanUps []func(), err error) {
	for _, v := range ingSpecs {
		if v.Name == appName {
			return cleanUps, errors.NotValidf("ingress name %q is reserved for juju expose", appName)
		}

		obj := metav1.ObjectMeta{
			Name:        v.Name,
			Labels:      k8slabels.Merge(v.Labels, k.getIngressLabels(appName)),
			Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
		}

		logger.Infof("ensuring ingress %q with version %q", obj.GetName(), v.Spec.Version)
		var cleanUp func()
		switch v.Spec.Version {
		case k8sspecs.K8sIngressV1:
			cleanUp, err = k.ensureIngressV1(appName, &networkingv1.Ingress{
				ObjectMeta: obj,
				Spec:       v.Spec.SpecV1,
			}, false)
		case k8sspecs.K8sIngressV1Beta1:
			cleanUp, err = k.ensureIngressV1beta1(appName, &networkingv1beta1.Ingress{
				ObjectMeta: obj,
				Spec:       v.Spec.SpecV1Beta1,
			}, false)
		default:
			// This should never happen.
			return cleanUps, errors.NotSupportedf("ingress version %q", v.Spec.Version)
		}

		cleanUps = append(cleanUps, cleanUp)
		if err != nil {
			return cleanUps, errors.Annotatef(err, "ensuring ingress %q with version %q", obj.GetName(), v.Spec.Version)
		}
	}
	return cleanUps, nil
}

func (k *kubernetesClient) ensureIngressV1beta1(appName string, spec *networkingv1beta1.Ingress, force bool) (func(), error) {
	cleanUp := func() {}
	api := k.client().NetworkingV1beta1().Ingresses(k.namespace)
	out, err := api.Create(context.TODO(), spec, metav1.CreateOptions{})
	if err == nil {
		cleanUp = func() { _ = api.Delete(context.TODO(), out.GetName(), newPreconditionDeleteOptions(out.GetUID())) }
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	if !force {
		existing, err := api.Get(context.TODO(), spec.GetName(), metav1.GetOptions{})
		if err != nil {
			return cleanUp, errors.Trace(err)
		}
		if len(existing.GetLabels()) == 0 || !k8slabels.AreLabelsInWhiteList(k.getIngressLabels(appName), existing.GetLabels()) {
			return cleanUp, errors.NewAlreadyExists(nil, fmt.Sprintf("existing ingress %q found which does not belong to %q", spec.GetName(), appName))
		}
	}
	_, err = api.Update(context.TODO(), spec, metav1.UpdateOptions{})
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) ensureIngressV1(appName string, spec *networkingv1.Ingress, force bool) (func(), error) {
	cleanUp := func() {}
	api := k.client().NetworkingV1().Ingresses(k.namespace)
	out, err := api.Create(context.TODO(), spec, metav1.CreateOptions{})
	if err == nil {
		cleanUp = func() { _ = api.Delete(context.TODO(), out.GetName(), newPreconditionDeleteOptions(out.GetUID())) }
		return cleanUp, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	if !force {
		existing, err := api.Get(context.TODO(), spec.GetName(), metav1.GetOptions{})
		if err != nil {
			return cleanUp, errors.Trace(err)
		}
		if len(existing.GetLabels()) == 0 || !k8slabels.AreLabelsInWhiteList(k.getIngressLabels(appName), existing.GetLabels()) {
			return cleanUp, errors.NewAlreadyExists(nil, fmt.Sprintf("existing ingress %q found which does not belong to %q", spec.GetName(), appName))
		}
	}
	_, err = api.Update(context.TODO(), spec, metav1.UpdateOptions{})
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) createIngress(ingress *networkingv1beta1.Ingress) (*networkingv1beta1.Ingress, error) {
	utils.PurifyResource(ingress)
	out, err := k.client().NetworkingV1beta1().Ingresses(k.namespace).Create(context.TODO(), ingress, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("ingress resource %q", ingress.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getIngress(name string) (*networkingv1beta1.Ingress, error) {
	out, err := k.client().NetworkingV1beta1().Ingresses(k.namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("ingress resource %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateIngress(ingress *networkingv1beta1.Ingress) (*networkingv1beta1.Ingress, error) {
	out, err := k.client().NetworkingV1beta1().Ingresses(k.namespace).Update(context.TODO(), ingress, metav1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("ingress resource %q", ingress.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteIngress(name string, uid k8stypes.UID) error {
	err := k.client().NetworkingV1beta1().Ingresses(k.namespace).Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listIngressResources(labels map[string]string) ([]networkingv1beta1.Ingress, error) {
	listOps := metav1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	}
	ingList, err := k.client().NetworkingV1beta1().Ingresses(k.namespace).List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(ingList.Items) == 0 {
		return nil, errors.NotFoundf("ingress with labels %v", labels)
	}
	return ingList.Items, nil
}

func (k *kubernetesClient) deleteIngressResources(appName string) error {
	err := k.client().NetworkingV1beta1().Ingresses(k.namespace).DeleteCollection(context.TODO(), metav1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, metav1.ListOptions{
		LabelSelector: utils.LabelsToSelector(k.getIngressLabels(appName)).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listIngressClasses(labels map[string]string) ([]networkingv1.IngressClass, error) {
	listOps := metav1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	}
	ingCList, err := k.client().NetworkingV1().IngressClasses().List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(ingCList.Items) == 0 {
		return nil, errors.NotFoundf("ingress class with labels %v", labels)
	}
	return ingCList.Items, nil
}
