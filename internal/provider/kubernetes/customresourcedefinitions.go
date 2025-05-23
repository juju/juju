// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

func (k *kubernetesClient) listCustomResourceDefinitions(ctx context.Context, selector k8slabels.Selector) ([]apiextensionsv1.CustomResourceDefinition, error) {
	listOps := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().List(ctx, listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(list.Items) == 0 {
		return nil, errors.NotFoundf("custom resource definitions with selector %q", selector)
	}
	return list.Items, nil
}

func (k *kubernetesClient) deleteCustomResourceDefinitions(ctx context.Context, selector k8slabels.Selector) error {
	err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().DeleteCollection(ctx, metav1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteCustomResources(ctx context.Context, selectorGetter func(apiextensionsv1.CustomResourceDefinition) k8slabels.Selector) error {
	crds, err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{
		// CRDs might be provisioned by another application/charm from a different model.
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, crd := range crds.Items {
		selector := selectorGetter(crd)
		if selector.Empty() {
			continue
		}
		for _, version := range crd.Spec.Versions {
			crdClient, err := k.getCustomResourceDefinitionClient(&crd, version.Name)
			if err != nil {
				return errors.Trace(err)
			}
			err = crdClient.DeleteCollection(ctx, metav1.DeleteOptions{
				PropagationPolicy: constants.DefaultPropagationPolicy(),
			}, metav1.ListOptions{
				LabelSelector: selector.String(),
			})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (k *kubernetesClient) listCustomResources(ctx context.Context, selectorGetter func(apiextensionsv1.CustomResourceDefinition) k8slabels.Selector) (out []unstructured.Unstructured, err error) {
	crds, err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{
		// CRDs might be provisioned by another application/charm from a different model.
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, crd := range crds.Items {
		selector := selectorGetter(crd)
		if selector.Empty() {
			continue
		}
		for _, version := range crd.Spec.Versions {
			crdClient, err := k.getCustomResourceDefinitionClient(&crd, version.Name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			list, err := crdClient.List(ctx, metav1.ListOptions{
				LabelSelector: selector.String(),
			})
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
			out = append(out, list.Items...)
		}
	}
	if len(out) == 0 {
		return nil, errors.NewNotFound(nil, "no custom resource found")
	}
	return out, nil
}

func isCRDScopeNamespaced(scope apiextensionsv1.ResourceScope) bool {
	return scope == apiextensionsv1.NamespaceScoped
}

func (k *kubernetesClient) getCustomResourceDefinitionClient(crd *apiextensionsv1.CustomResourceDefinition, version string) (dynamic.ResourceInterface, error) {
	if version == "" {
		return nil, errors.NotValidf("empty version for custom resource definition %q", crd.GetName())
	}

	checkVersion := func() error {
		for _, v := range crd.Spec.Versions {
			if !v.Served {
				continue
			}
			if version == v.Name {
				return nil
			}
		}
		return errors.NewNotValid(nil, fmt.Sprintf("custom resource definition %s %s is not a supported and served version", crd.GetName(), version))
	}

	if err := checkVersion(); err != nil {
		return nil, errors.Trace(err)
	}
	client := k.dynamicClient().Resource(
		schema.GroupVersionResource{
			Group:    crd.Spec.Group,
			Version:  version,
			Resource: crd.Spec.Names.Plural,
		},
	)
	if !isCRDScopeNamespaced(crd.Spec.Scope) {
		return client, nil
	}

	if k.namespace == "" {
		return nil, errNoNamespace
	}
	return client.Namespace(k.namespace), nil
}
