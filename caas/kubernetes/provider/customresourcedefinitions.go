// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	// "strings"
	"time"

	"github.com/juju/errors"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

func (k *kubernetesClient) getCRDLabels(appName string) map[string]string {
	return map[string]string{
		labelApplication: appName,
		labelModel:       k.namespace,
	}
}

// ensureCustomResourceDefinitions creates or updates a custom resource definition resource.
func (k *kubernetesClient) ensureCustomResourceDefinitions(appName string, crdSpecs map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec) (
	crds map[string]*apiextensionsv1beta1.CustomResourceDefinition, cleanUps []func(), _ error,
) {
	crds = make(map[string]*apiextensionsv1beta1.CustomResourceDefinition, len(crdSpecs))
	for name, spec := range crdSpecs {
		crd := &apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: k.namespace,
				Labels:    k.getCRDLabels(appName),
			},
			Spec: spec,
		}
		out, cleanUp, err := k.ensureCustomResourceDefinition(crd)
		cleanUps = append(cleanUps, cleanUp)
		if err != nil {
			return crds, cleanUps, errors.Annotate(err, fmt.Sprintf("ensuring custom resource definition %q", name))
		}
		crds[name] = out
		logger.Debugf("ensured custom resource definition %q", out.GetName())
	}
	return crds, cleanUps, nil
}

func (k *kubernetesClient) ensureCustomResourceDefinition(crd *apiextensionsv1beta1.CustomResourceDefinition) (out *apiextensionsv1beta1.CustomResourceDefinition, cleanUp func(), err error) {
	api := k.extendedCient().ApiextensionsV1beta1().CustomResourceDefinitions()
	logger.Debugf("creating custom resource definition %#v", crd)
	if out, err = api.Create(crd); err == nil {
		cleanUp = func() { k.deleteCustomResourceDefinition(out.GetName(), out.GetUID()) }
	}
	if k8serrors.IsAlreadyExists(err) {
		logger.Debugf("updating custom resource definition %q", crd.GetName())
		// TODO(caas): do label check to ensure the resource to be updated was created by Juju once caas upgrade steps of 2.7 in place.
		out, err = api.Update(crd)
	}
	return out, cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) deleteCustomResourceDefinition(name string, uid types.UID) error {
	err := k.extendedCient().ApiextensionsV1beta1().CustomResourceDefinitions().Delete(name, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteCustomResourceDefinitions(appName string) error {
	// TODO: list crds -> get crdClient, then crdClient.DeleteCollection() to delete crs first!!!!!!!!
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

func (k *kubernetesClient) ensureCustomResources(
	appName string,
	crds map[string]*apiextensionsv1beta1.CustomResourceDefinition,
	crSpecs map[string][]unstructured.Unstructured,
) (cleanUps []func(), _ error) {

	// TODO: add waiter to ensure crds are stablised.!!!!!!!!!
	time.Sleep(5 * time.Second)

	for name, crSpecs := range crSpecs {
		crd := crds[name]
		if crd == nil {
			return cleanUps, errors.NotFoundf("custom resource definition for %q", name)
		}
		for _, crSpec := range crSpecs {
			// TODO: version = 	strings.Split(crSpec.GetAPIVersion(), "/") !!!!!!!!!!!!
			version := "v1"
			crdClient, err := k.getCustomResourceDefinitionClient(*crd, version)
			if err != nil {
				return cleanUps, errors.Trace(err)
			}
			_, cleanUp, err := ensureCustomResource(crdClient, crSpec, k.getCRDLabels(appName))
			cleanUps = append(cleanUps, cleanUp)
			if err != nil {
				return cleanUps, errors.Annotate(err, fmt.Sprintf("ensuring custom resource %q", name))
			}
			logger.Debugf("ensured custom resource %q", name)
		}
	}
	return cleanUps, nil
}

func ensureCustomResource(api dynamic.ResourceInterface, cr unstructured.Unstructured, labels map[string]string) (out *unstructured.Unstructured, cleanUp func(), err error) {
	cr.SetLabels(labels)

	if out, err = api.Create(&cr); err == nil {
		cleanUp = func() { deleteCustomResourceDefinition(api, out.GetName(), out.GetUID()) }
	}
	if k8serrors.IsAlreadyExists(err) {
		logger.Debugf("updating custom resource %q", cr.GetName())
		// TODO(caas): do label check to ensure the resource to be updated was created by Juju once caas upgrade steps of 2.7 in place.
		out, err = api.Update(&cr)
	}
	return out, cleanUp, errors.Trace(err)
}

func deleteCustomResourceDefinition(api dynamic.ResourceInterface, name string, uid types.UID) error {
	err := api.Delete(name, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) getCustomResourceDefinitionClient(crd apiextensionsv1beta1.CustomResourceDefinition, version string) (dynamic.ResourceInterface, error) {
	if crd.Spec.Scope != apiextensionsv1beta1.NamespaceScoped {
		// This has already done in podspec validation, but it doesn't hurt check here again.
		return nil, errors.NewNotSupported(nil,
			fmt.Sprintf("custom resource definition %q scope %q is not supported, please use %q scope",
				crd.GetName(), crd.Spec.Scope, apiextensionsv1beta1.NamespaceScoped),
		)
	}

	checkVersion := func() error {
		if crd.Spec.Version == version {
			return nil
		}

		for _, v := range crd.Spec.Versions {
			if !v.Served {
				continue
			}
			if version == v.Name {
				return nil
			}
		}
		return errors.NotFoundf("custom resource definition %q with version %q", crd.GetName(), version)
	}

	if err := checkVersion(); err != nil {
		return nil, errors.Trace(err)
	}
	logger.Criticalf("getCustomResourceDefinitionClient version %q", version)
	return k.dynamicClient().Resource(
		schema.GroupVersionResource{
			Group:    crd.Spec.Group,
			Version:  version,
			Resource: crd.Spec.Names.Plural,
		},
	).Namespace(k.namespace), nil
}
