// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/json"
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

<<<<<<< HEAD
func (k *kubernetesClient) listCustomResources(ctx context.Context, selectorGetter func(apiextensionsv1.CustomResourceDefinition) k8slabels.Selector) (out []unstructured.Unstructured, err error) {
	crds, err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{
		// CRDs might be provisioned by another application/charm from a different model.
=======
// getAllNamespacesCustomResourceDefinitionClient returns a dynamic resource
// client for the given CRD and version that operates everywhere. For namespaced
// CRDs this returns the unscoped NamespaceableResourceInterface.
func (k *kubernetesClient) getAllNamespacesCustomResourceDefinitionClient(
	crd *apiextensionsv1.CustomResourceDefinition,
	version string,
) (dynamic.NamespaceableResourceInterface, error) {
	if version == "" {
		return nil, errors.NotValidf(
			"empty version for custom resource definition %q", crd.GetName(),
		)
	}
	found := false
	for _, v := range crd.Spec.Versions {
		if !v.Served {
			continue
		}
		if version == v.Name {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.NotValidf(
			"custom resource definition %s %s is not a supported and served version",
			crd.GetName(), version,
		)
	}
	return k.dynamicClient().Resource(schema.GroupVersionResource{
		Group:    crd.Spec.Group,
		Version:  version,
		Resource: crd.Spec.Names.Plural,
	}), nil
}

// removeAllCustomResourceFinalizers lists all CRs everywhere that matches
// the selector, and patches each one to remove all finalisers. This must be
// done before deletion so that resources with finalisers are not left stuck
// in a terminating state.
func (k *kubernetesClient) removeAllCustomResourceFinalizers(
	ctx context.Context, selector k8slabels.Selector,
) error {
	client := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions()
	crds, err := client.List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return errors.Trace(err)
	}
	// finalizersPatch is the merge-patch payload that clears all finalizers.
	finalizersPatch, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"finalizers": []string{},
		},
	})
	if err != nil {
		return errors.Trace(err)
	}
	patchAll := func(
		crd *apiextensionsv1.CustomResourceDefinition, versionName string,
	) error {
		crdClient, err := k.getAllNamespacesCustomResourceDefinitionClient(
			crd, versionName)
		if err != nil {
			return errors.Trace(err)
		}
		list, err := crdClient.List(ctx, metav1.ListOptions{
			// CRs might be provisioned by another application/charm from a different model.
			LabelSelector: "",
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if list == nil {
			return nil
		}
		for _, cr := range list.Items {
			if len(cr.GetFinalizers()) == 0 {
				continue
			}
			client := dynamic.ResourceInterface(crdClient)
			if isCRDScopeNamespaced(crd.Spec.Scope) && cr.GetNamespace() != "" {
				client = crdClient.Namespace(cr.GetNamespace())
			}
			_, err = client.Patch(
				context.TODO(),
				cr.GetName(),
				types.MergePatchType,
				finalizersPatch,
				metav1.PatchOptions{},
			)
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Annotatef(
					err, "removing finalizers from custom resource %q (namespace %q)",
					cr.GetName(), cr.GetNamespace(),
				)
			}
		}
		return nil
	}
	for _, crd := range crds.Items {
		if selector.Empty() {
			continue
		}
		for _, version := range crd.Spec.Versions {
			if !version.Served {
				continue
			}
			err := patchAll(&crd, version.Name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteAllCustomResourcesAllNamespaces deletes custom resources matching the
// supplied selector everywhere.
func (k *kubernetesClient) deleteAllCustomResourcesAllNamespaces(
	ctx context.Context, selector k8slabels.Selector,
) error {
	client := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions()
	crds, err := client.List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, crd := range crds.Items {
		if selector.Empty() {
			continue
		}
		for _, version := range crd.Spec.Versions {
			crdClient, err := k.getAllNamespacesCustomResourceDefinitionClient(
				&crd, version.Name)
			if err != nil {
				return errors.Trace(err)
			}
			err = crdClient.DeleteCollection(ctx, metav1.DeleteOptions{
				PropagationPolicy: constants.DefaultPropagationPolicy(),
			}, metav1.ListOptions{
				// CRs might be provisioned by another application/charm from a different model.
				LabelSelector: "",
			})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

// listAllCustomResourcesAllNamespaces lists custom resources matching the
// selector everywhere.
func (k *kubernetesClient) listAllCustomResourcesAllNamespaces(
	ctx context.Context, selector k8slabels.Selector,
) (out []unstructured.Unstructured, err error) {
	client := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions()
	crds, err := client.List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
>>>>>>> 3.6
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, crd := range crds.Items {
		if selector.Empty() {
			continue
		}
		for _, version := range crd.Spec.Versions {
			crdClient, err := k.getAllNamespacesCustomResourceDefinitionClient(
				&crd, version.Name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			list, err := crdClient.List(ctx, metav1.ListOptions{
<<<<<<< HEAD
				LabelSelector: selector.String(),
=======
				// CRs might be provisioned by another application/charm from a different model.
				LabelSelector: "",
>>>>>>> 3.6
			})
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
			if list != nil {
				out = append(out, list.Items...)
			}
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
