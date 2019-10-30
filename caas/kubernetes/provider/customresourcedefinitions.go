// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"strings"
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

func (k *kubernetesClient) getCRLabels(appName string) map[string]string {
	return map[string]string{
		labelApplication: appName,
	}
}

// ensureCustomResourceDefinitions creates or updates a custom resource definition resource.
func (k *kubernetesClient) ensureCustomResourceDefinitions(
	appName string, crdSpecs map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec,
) (cleanUps []func(), _ error) {
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
			return cleanUps, errors.Annotate(err, fmt.Sprintf("ensuring custom resource definition %q", name))
		}
		logger.Debugf("ensured custom resource definition %q", out.GetName())
	}
	return cleanUps, nil
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

func (k *kubernetesClient) getCustomResourceDefinition(name string, includeUninitialized bool) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	crd, err := k.extendedCient().ApiextensionsV1beta1().CustomResourceDefinitions().Get(name, v1.GetOptions{IncludeUninitialized: includeUninitialized})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("custom resource definition %q", name)
	}
	return crd, errors.Trace(err)
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

type apiVersionGetter interface {
	GetAPIVersion() string
}

func getCRVersion(cr apiVersionGetter) string {
	ss := strings.Split(cr.GetAPIVersion(), "/")
	return ss[len(ss)-1]
}

func (k *kubernetesClient) ensureCustomResources(
	appName string,
	crSpecs map[string][]unstructured.Unstructured,
) (cleanUps []func(), _ error) {
	for crdName, crSpecList := range crSpecs {
		for _, crSpec := range crSpecList {
			crdClient, err := k.getCustomResourceDefinitionClient(crdName, getCRVersion(&crSpec))
			if err != nil {
				return cleanUps, errors.Trace(err)
			}
			logger.Criticalf("crSpec before -> %#v", crSpec)
			crSpec.SetLabels(k.getCRLabels(appName))
			logger.Criticalf("crSpec after -> %#v", crSpec)
			_, cleanUp, err := ensureCustomResource(crdClient, &crSpec)
			cleanUps = append(cleanUps, cleanUp)
			if err != nil {
				return cleanUps, errors.Annotate(err, fmt.Sprintf("ensuring custom resource %q", crSpec.GetName()))
			}
			logger.Debugf("ensured custom resource %q", crSpec.GetName())
		}
	}
	return cleanUps, nil
}

func ensureCustomResource(api dynamic.ResourceInterface, cr *unstructured.Unstructured) (out *unstructured.Unstructured, cleanUp func(), err error) {
	if out, err = api.Create(cr); err == nil {
		cleanUp = func() { deleteCustomResourceDefinition(api, out.GetName(), out.GetUID()) }
	}
	if k8serrors.IsAlreadyExists(err) {
		logger.Debugf("updating custom resource %q", cr.GetName())
		// TODO(caas): do label check to ensure the resource to be updated was created by Juju once caas upgrade steps of 2.7 in place.
		out, err = api.Update(cr)
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

func (k *kubernetesClient) getCustomResourceDefinitionClient(crdName, version string) (dynamic.ResourceInterface, error) {
	logger.Criticalf("getting custom resource definition client %q, version %q", crdName, version)

	// TODO: add waiter to ensure crds are stablised.!!!!!!!!!
	time.Sleep(5 * time.Second)

	crd, err := k.getCustomResourceDefinition(crdName, false)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if crd.Spec.Scope != apiextensionsv1beta1.NamespaceScoped {
		// This has already done in podspec validation for checking Juju created CRD.
		// Here, check it again for referencing exisitng CRD which was not created by Juju.
		return nil, errors.NewNotSupported(nil,
			fmt.Sprintf("custom resource definition %q scope %q is not supported, please use %q scope",
				crd.GetName(), crd.Spec.Scope, apiextensionsv1beta1.NamespaceScoped),
		)
	}
	if version == "" {
		return nil, errors.NotValidf("empty version for custom resource definition %q", crd.GetName())
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
		return errors.NotFoundf("served custom resource definition %q with version %q", crd.GetName(), version)
	}

	if err := checkVersion(); err != nil {
		return nil, errors.Trace(err)
	}
	return k.dynamicClient().Resource(
		schema.GroupVersionResource{
			Group:    crd.Spec.Group,
			Version:  version,
			Resource: crd.Spec.Names.Plural,
		},
	).Namespace(k.namespace), nil
}
