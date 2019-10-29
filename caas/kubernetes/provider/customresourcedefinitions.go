// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (k *kubernetesClient) getCRDLabels(appName string) map[string]string {
	return map[string]string{
		labelApplication: appName,
		labelModel:       k.namespace,
	}
}

// ensureCustomResourceDefinitions creates or updates a custom resource definition resource.
func (k *kubernetesClient) ensureCustomResourceDefinitions(
	appName string,
	annotations map[string]string,
	crds map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec,
) (cleanUps []func(), _ error) {
	for name, crd := range crds {
		crd, err := k.ensureCustomResourceDefinition(name, k.getCRDLabels(appName), annotations, crd)
		if err != nil {
			return cleanUps, errors.Annotate(err, fmt.Sprintf("ensure custom resource definition %q", name))
		}
		logger.Debugf("ensured custom resource definition %q", crd.ObjectMeta.Name)
		cleanUps = append(cleanUps, func() { k.deleteCustomResourceDefinition(name) })
	}
	return cleanUps, nil
}

func (k *kubernetesClient) ensureCustomResourceDefinition(
	name string, labels map[string]string,
	annotations map[string]string,
	spec apiextensionsv1beta1.CustomResourceDefinitionSpec,
) (crd *apiextensionsv1beta1.CustomResourceDefinition, err error) {
	crdIn := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        name,
			Namespace:   k.namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: spec,
	}
	apiextensionsV1beta1 := k.extendedCient().ApiextensionsV1beta1()
	logger.Debugf("creating crd %#v", crdIn)
	crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Create(crdIn)
	if k8serrors.IsAlreadyExists(err) {
		crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Get(name, v1.GetOptions{})
		if err != nil {
			return nil, errors.Trace(err)
		}
		resourceVersion := crd.ObjectMeta.GetResourceVersion()
		crdIn.ObjectMeta.SetResourceVersion(resourceVersion)
		logger.Debugf("existing crd with resource version %q found, so update it %#v", resourceVersion, crdIn)
		crd, err = apiextensionsV1beta1.CustomResourceDefinitions().Update(crdIn)
	}
	return
}

func (k *kubernetesClient) deleteCustomResourceDefinition(name string) error {
	err := k.extendedCient().ApiextensionsV1beta1().CustomResourceDefinitions().Delete(name, &v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteCustomResourceDefinitions(appName string) error {
	err := k.extendedCient().ApiextensionsV1beta1().CustomResourceDefinitions().DeleteCollection(&v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector:        labelsToSelector(k.getCRDLabels(appName)),
		IncludeUninitialized: true,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
