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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func (k *kubernetesClient) getIngressLabels(appName string) map[string]string {
	return utils.LabelsForApp(appName, k.LabelVersion())
}

// TODO(caas): should we overwrite the existing `juju expose` created ingress if user runs upgrade-charm with new ingress podspec v2.
// https://bugs.launchpad.net/juju/+bug/1854123
func (k *kubernetesClient) ensureIngressResources(
	appName string, annotations k8sannotations.Annotation, ingSpecs []k8sspecs.K8sIngress,
) (cleanUps []func(), err error) {
	k8sVersion, err := k.Version()
	if err != nil {
		return nil, errors.Annotate(err, "getting k8s api version")
	}
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

		var (
			specV1Beta1 *networkingv1beta1.IngressSpec
			specV1      *networkingv1.IngressSpec
		)

		// k8s 1.22 drops support for networkingv1beta1, so we need
		// to convert the ingress spec to the correct version for the
		// version of k8s we are deploying to.
		// The v1 networking api is available since 1.19.
		switch v.Spec.Version {
		case k8sspecs.K8sIngressV1:
			if k8sVersion.Major == 1 && k8sVersion.Minor < 19 {
				specV1Beta1 = k8sspecs.IngressSpecFromV1(&v.Spec.SpecV1)
			} else {
				specV1 = &v.Spec.SpecV1
			}
		case k8sspecs.K8sIngressV1Beta1:
			if k8sVersion.Major == 1 && k8sVersion.Minor >= 19 {
				specV1 = k8sspecs.IngressSpecToV1(&v.Spec.SpecV1Beta1)
			} else {
				specV1Beta1 = &v.Spec.SpecV1Beta1
			}
		default:
			// This should never happen.
			return cleanUps, errors.NotSupportedf("ingress version %q", v.Spec.Version)
		}

		if specV1 != nil {
			cleanUp, err = k.ensureIngressV1(appName, &networkingv1.Ingress{
				ObjectMeta: obj,
				Spec:       *specV1,
			}, false)
		}
		if specV1Beta1 != nil {
			cleanUp, err = k.ensureIngressV1beta1(appName, &networkingv1beta1.Ingress{
				ObjectMeta: obj,
				Spec:       *specV1Beta1,
			}, false)
		}

		cleanUps = append(cleanUps, cleanUp)
		if err != nil {
			return cleanUps, errors.Annotatef(err, "ensuring ingress %q with version %q", obj.GetName(), v.Spec.Version)
		}
	}
	return cleanUps, nil
}

// Pulled from https://github.com/kubernetes/kubernetes/blob/04969280beb16951793cb64a1aa7fd0cc4b1b60c/plugin/pkg/admission/podnodeselector/admission.go#L263
func isSubset(subSet, superSet k8slabels.Set) bool {
	if len(superSet) == 0 {
		return true
	}

	for k, v := range subSet {
		value, ok := superSet[k]
		if !ok {
			return false
		}
		if value != v {
			return false
		}
	}
	return true
}

func (k *kubernetesClient) ensureIngressV1beta1(appName string, spec *networkingv1beta1.Ingress, force bool) (func(), error) {
	cleanUp := func() {}
	if k.namespace == "" {
		return cleanUp, errNoNamespace
	}
	api := k.client().NetworkingV1beta1().Ingresses(k.namespace)
	out, err := api.Create(context.TODO(), spec, metav1.CreateOptions{})
	if err == nil {
		cleanUp = func() {
			_ = api.Delete(context.TODO(), out.GetName(), utils.NewPreconditionDeleteOptions(out.GetUID()))
		}
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
		if len(existing.GetLabels()) == 0 || !isSubset(k.getIngressLabels(appName), existing.GetLabels()) {
			return cleanUp, errors.NewAlreadyExists(nil, fmt.Sprintf("existing ingress %q found which does not belong to %q", spec.GetName(), appName))
		}
	}
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, spec)
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	_, err = api.Patch(context.TODO(), spec.GetName(), k8stypes.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: resources.JujuFieldManager,
	})
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) ensureIngressV1(appName string, spec *networkingv1.Ingress, force bool) (func(), error) {
	cleanUp := func() {}
	if k.namespace == "" {
		return cleanUp, errNoNamespace
	}
	api := k.client().NetworkingV1().Ingresses(k.namespace)
	out, err := api.Create(context.TODO(), spec, metav1.CreateOptions{})
	if err == nil {
		cleanUp = func() {
			_ = api.Delete(context.TODO(), out.GetName(), utils.NewPreconditionDeleteOptions(out.GetUID()))
		}
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
		if len(existing.GetLabels()) == 0 || !isSubset(k.getIngressLabels(appName), existing.GetLabels()) {
			return cleanUp, errors.NewAlreadyExists(nil, fmt.Sprintf("existing ingress %q found which does not belong to %q", spec.GetName(), appName))
		}
	}
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, spec)
	if err != nil {
		return cleanUp, errors.Trace(err)
	}
	_, err = api.Patch(context.TODO(), spec.GetName(), k8stypes.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: resources.JujuFieldManager,
	})
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) deleteIngress(name string, uid k8stypes.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().NetworkingV1().Ingresses(k.namespace).Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteIngressResources(appName string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().NetworkingV1().Ingresses(k.namespace).DeleteCollection(context.TODO(), metav1.DeleteOptions{
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
