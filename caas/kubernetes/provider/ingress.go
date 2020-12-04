// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"k8s.io/api/extensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	appName string, annotations k8sannotations.Annotation, ingSpecs []k8sspecs.K8sIngressSpec,
) (cleanUps []func(), err error) {
	for _, v := range ingSpecs {
		if v.Name == appName {
			return cleanUps, errors.NotValidf("ingress name %q is reserved for juju expose", appName)
		}
		ing := &v1beta1.Ingress{
			ObjectMeta: v1.ObjectMeta{
				Name:        v.Name,
				Labels:      k8slabels.Merge(v.Labels, k.getIngressLabels(appName)),
				Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
			},
			Spec: v.Spec,
		}
		cleanUp, err := k.ensureIngress(appName, ing, false)
		cleanUps = append(cleanUps, cleanUp)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

func (k *kubernetesClient) ensureIngress(appName string, spec *v1beta1.Ingress, force bool) (func(), error) {
	cleanUp := func() {}
	out, err := k.createIngress(spec)
	if err == nil {
		cleanUp = func() { _ = k.deleteIngress(out.GetName(), out.GetUID()) }
		return cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return cleanUp, errors.Trace(err)
	}
	if !force {
		existing, err := k.getIngress(spec.GetName())
		if err != nil {
			return cleanUp, errors.Trace(err)
		}
		if len(existing.GetLabels()) == 0 || !k8slabels.AreLabelsInWhiteList(k.getIngressLabels(appName), existing.GetLabels()) {
			return cleanUp, errors.NewAlreadyExists(nil, fmt.Sprintf("existing ingress %q found which does not belong to %q", spec.GetName(), appName))
		}
	}
	_, err = k.updateIngress(spec)
	return cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) createIngress(ingress *v1beta1.Ingress) (*v1beta1.Ingress, error) {
	utils.PurifyResource(ingress)
	out, err := k.client().ExtensionsV1beta1().Ingresses(k.namespace).Create(context.TODO(), ingress, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("ingress resource %q", ingress.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getIngress(name string) (*v1beta1.Ingress, error) {
	out, err := k.client().ExtensionsV1beta1().Ingresses(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("ingress resource %q", name)
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateIngress(ingress *v1beta1.Ingress) (*v1beta1.Ingress, error) {
	out, err := k.client().ExtensionsV1beta1().Ingresses(k.namespace).Update(context.TODO(), ingress, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("ingress resource %q", ingress.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deleteIngress(name string, uid k8stypes.UID) error {
	err := k.client().ExtensionsV1beta1().Ingresses(k.namespace).Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) listIngressResources(labels map[string]string) ([]v1beta1.Ingress, error) {
	listOps := v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	}
	ingList, err := k.client().ExtensionsV1beta1().Ingresses(k.namespace).List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(ingList.Items) == 0 {
		return nil, errors.NotFoundf("ingress with labels %v", labels)
	}
	return ingList.Items, nil
}

func (k *kubernetesClient) deleteIngressResources(appName string) error {
	err := k.client().ExtensionsV1beta1().Ingresses(k.namespace).DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(k.getIngressLabels(appName)).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
