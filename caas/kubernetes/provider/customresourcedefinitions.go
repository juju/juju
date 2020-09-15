// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8sannotations "github.com/juju/juju/core/annotations"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/crd_getter_mock.go github.com/juju/juju/caas/kubernetes/provider CRDGetterInterface

func (k *kubernetesClient) getAPIExtensionLabelsGlobal(appName string) map[string]string {
	labels := utils.LabelsMerge(
		utils.LabelsForApp(appName, k.IsLegacyLabels()),
		utils.LabelsForModel(k.CurrentModel(), k.IsLegacyLabels()),
	)
	if !k.IsLegacyLabels() {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}
	return labels
}

func (k *kubernetesClient) getAPIExtensionLabelsNamespaced(appName string) map[string]string {
	labels := utils.LabelsForApp(appName, k.IsLegacyLabels())
	if !k.IsLegacyLabels() {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}
	return labels
}

func (k *kubernetesClient) getCRLabels(appName string, scope apiextensionsv1beta1.ResourceScope) map[string]string {
	if isCRDScopeNamespaced(scope) {
		return k.getAPIExtensionLabelsNamespaced(appName)
	}
	return k.getAPIExtensionLabelsGlobal(appName)
}

// ensureCustomResourceDefinitions creates or updates a custom resource definition resource.
func (k *kubernetesClient) ensureCustomResourceDefinitions(
	appName string,
	annotations map[string]string,
	crdSpecs []k8sspecs.K8sCustomResourceDefinitionSpec,
) (cleanUps []func(), _ error) {
	for _, v := range crdSpecs {
		crd := &apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: v1.ObjectMeta{
				Name:        v.Name,
				Labels:      k8slabels.Merge(v.Labels, k.getAPIExtensionLabelsGlobal(appName)),
				Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
			},
			Spec: v.Spec,
		}
		out, crdCleanUps, err := k.ensureCustomResourceDefinition(crd)
		cleanUps = append(cleanUps, crdCleanUps...)
		if err != nil {
			return cleanUps, errors.Annotate(err, fmt.Sprintf("ensuring custom resource definition %q", v.Name))
		}
		logger.Debugf("ensured custom resource definition %q", out.GetName())
	}
	return cleanUps, nil
}

func (k *kubernetesClient) ensureCustomResourceDefinition(crd *apiextensionsv1beta1.CustomResourceDefinition) (out *apiextensionsv1beta1.CustomResourceDefinition, cleanUps []func(), err error) {
	api := k.extendedClient().ApiextensionsV1beta1().CustomResourceDefinitions()
	logger.Debugf("creating custom resource definition %q", crd.GetName())
	if out, err = api.Create(context.TODO(), crd, v1.CreateOptions{}); err == nil {
		cleanUps = append(cleanUps, func() { _ = k.deleteCustomResourceDefinition(out.GetName(), out.GetUID()) })
		return out, cleanUps, nil

	}
	if !k8serrors.IsAlreadyExists(err) {
		return nil, cleanUps, errors.Trace(err)
	}
	// K8s complains about metadata.resourceVersion is required for an update, so get it before updating.
	existingCRD, err := k.getCustomResourceDefinition(crd.GetName())
	logger.Debugf("updating custom resource definition %q", crd.GetName())
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	crd.SetResourceVersion(existingCRD.GetResourceVersion())
	// TODO(caas): do label check to ensure the resource to be updated was created by Juju once caas upgrade steps of 2.7 in place.
	out, err = api.Update(context.TODO(), crd, v1.UpdateOptions{})
	return out, cleanUps, errors.Trace(err)
}

func (k *kubernetesClient) deleteCustomResourceDefinition(name string, uid types.UID) error {
	err := k.extendedClient().ApiextensionsV1beta1().CustomResourceDefinitions().Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) getCustomResourceDefinition(name string) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	crd, err := k.extendedClient().ApiextensionsV1beta1().CustomResourceDefinitions().Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("custom resource definition %q", name)
	}
	return crd, errors.Trace(err)
}

func (k *kubernetesClient) listCustomResourceDefinitions(selector k8slabels.Selector) ([]apiextensionsv1beta1.CustomResourceDefinition, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := k.extendedClient().ApiextensionsV1beta1().CustomResourceDefinitions().List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(list.Items) == 0 {
		return nil, errors.NotFoundf("custom resource definitions with selector %q", selector)
	}
	return list.Items, nil
}

func (k *kubernetesClient) deleteCustomResourceDefinitionsForApp(appName string) error {
	selector := mergeSelectors(
		utils.LabelSetToSelector(k.getAPIExtensionLabelsGlobal(appName)),
		lifecycleApplicationRemovalSelector,
	)
	return errors.Trace(k.deleteCustomResourceDefinitions(selector))
}

func (k *kubernetesClient) deleteCustomResourceDefinitions(selector k8slabels.Selector) error {
	err := k.extendedClient().ApiextensionsV1beta1().CustomResourceDefinitions().DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: &constants.DefaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteCustomResourcesForApp(appName string) error {
	selectorGetter := func(crd apiextensionsv1beta1.CustomResourceDefinition) k8slabels.Selector {
		return mergeSelectors(
			utils.LabelSetToSelector(k.getCRLabels(appName, crd.Spec.Scope)),
			lifecycleApplicationRemovalSelector,
		)
	}
	return k.deleteCustomResources(selectorGetter)
}

func (k *kubernetesClient) deleteCustomResources(selectorGetter func(apiextensionsv1beta1.CustomResourceDefinition) k8slabels.Selector) error {
	crds, err := k.extendedClient().ApiextensionsV1beta1().CustomResourceDefinitions().List(context.TODO(), v1.ListOptions{
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
			err = crdClient.DeleteCollection(context.TODO(), v1.DeleteOptions{
				PropagationPolicy: &constants.DefaultPropagationPolicy,
			}, v1.ListOptions{
				LabelSelector: selector.String(),
			})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (k *kubernetesClient) listCustomResources(selectorGetter func(apiextensionsv1beta1.CustomResourceDefinition) k8slabels.Selector) (out []unstructured.Unstructured, err error) {
	crds, err := k.extendedClient().ApiextensionsV1beta1().CustomResourceDefinitions().List(context.TODO(), v1.ListOptions{
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
			list, err := crdClient.List(context.TODO(), v1.ListOptions{
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

type apiVersionGetter interface {
	GetAPIVersion() string
}

func getCRVersion(cr apiVersionGetter) string {
	ss := strings.Split(cr.GetAPIVersion(), "/")
	return ss[len(ss)-1]
}

func (k *kubernetesClient) ensureCustomResources(
	appName string,
	annotations map[string]string,
	crSpecs map[string][]unstructured.Unstructured,
) (cleanUps []func(), _ error) {
	crds, err := k.getCRDsForCRs(crSpecs, &crdGetter{k})
	if err != nil {
		return cleanUps, errors.Trace(err)
	}

	for crdName, crSpecList := range crSpecs {
		crd, ok := crds[crdName]
		if !ok {
			// This should not happen.
			return cleanUps, errors.NotFoundf("custom resource definition %q", crdName)
		}
		for _, crSpec := range crSpecList {
			crdClient, err := k.getCustomResourceDefinitionClient(crd, getCRVersion(&crSpec))
			if err != nil {
				return cleanUps, errors.Trace(err)
			}
			crSpec.SetLabels(
				k8slabels.Merge(crSpec.GetLabels(), k.getCRLabels(appName, crd.Spec.Scope)),
			)
			crSpec.SetAnnotations(
				k8sannotations.New(crSpec.GetAnnotations()).
					Merge(k8sannotations.New(annotations)).
					ToMap(),
			)
			_, crCleanUps, err := ensureCustomResource(crdClient, &crSpec)
			cleanUps = append(cleanUps, crCleanUps...)
			if err != nil {
				return cleanUps, errors.Annotate(err, fmt.Sprintf("ensuring custom resource %q", crSpec.GetName()))
			}
			logger.Debugf("ensured custom resource %q", crSpec.GetName())
		}
	}
	return cleanUps, nil
}

func ensureCustomResource(api dynamic.ResourceInterface, cr *unstructured.Unstructured) (out *unstructured.Unstructured, cleanUps []func(), err error) {
	logger.Debugf("creating custom resource %q", cr.GetName())
	if out, err = api.Create(context.TODO(), cr, v1.CreateOptions{}); err == nil {
		cleanUps = append(cleanUps, func() {
			deleteCustomResourceDefinition(api, out.GetName(), out.GetUID())
		})
		return out, cleanUps, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return nil, cleanUps, errors.Trace(err)
	}
	// K8s complains about metadata.resourceVersion is required for an update, so get it before updating.
	existingCR, err := api.Get(context.TODO(), cr.GetName(), v1.GetOptions{})
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cr.SetResourceVersion(existingCR.GetResourceVersion())
	logger.Debugf("updating custom resource %q", cr.GetName())
	out, err = api.Update(context.TODO(), cr, v1.UpdateOptions{})
	return out, cleanUps, errors.Trace(err)
}

func deleteCustomResourceDefinition(api dynamic.ResourceInterface, name string, uid types.UID) error {
	err := api.Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

type CRDGetterInterface interface {
	Get(string) (*apiextensionsv1beta1.CustomResourceDefinition, error)
}

type crdGetter struct {
	Broker *kubernetesClient
}

func (cg *crdGetter) Get(
	name string,
) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	crd, err := cg.Broker.getCustomResourceDefinition(name)
	if err != nil {
		return nil, errors.Annotatef(err, "getting custom resource definition %q", name)
	}
	version := crd.Spec.Version
	if version == "" {
		if len(crd.Spec.Versions) == 0 {
			return nil, errors.NotValidf("custom resource definition %q without version", crd.GetName())
		}
		version = crd.Spec.Versions[0].Name
	}
	crClient, err := cg.Broker.getCustomResourceDefinitionClient(crd, version)
	if err != nil {
		return nil, errors.Annotatef(err, "getting custom resource definition client %q", name)
	}
	if _, err := crClient.List(context.TODO(), v1.ListOptions{}); err != nil {
		if k8serrors.IsNotFound(err) {
			// CRD already exists, but the resource type does not exist yet.
			return nil, errors.NewNotFound(err, fmt.Sprintf("custom resource definition %q resource type", crd.GetName()))
		}
		return nil, errors.Trace(err)
	}
	return crd, nil
}

func (k *kubernetesClient) getCRDsForCRs(
	crs map[string][]unstructured.Unstructured,
	getter CRDGetterInterface,
) (out map[string]*apiextensionsv1beta1.CustomResourceDefinition, err error) {

	out = make(map[string]*apiextensionsv1beta1.CustomResourceDefinition)
	crdChan := make(chan *apiextensionsv1beta1.CustomResourceDefinition, 1)
	errChan := make(chan error, 1)

	n := len(crs)
	if n == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(n)
	defer wg.Wait()

	getCRD := func(
		ctx context.Context,
		name string,
		getter CRDGetterInterface,
		resultChan chan<- *apiextensionsv1beta1.CustomResourceDefinition,
		errChan chan<- error,
		clk jujuclock.Clock,
	) {
		var crd *apiextensionsv1beta1.CustomResourceDefinition
		var err error
		err = retry.Call(retry.CallArgs{
			Attempts: 8,
			Delay:    1 * time.Second,
			Clock:    clk,
			Stop:     ctx.Done(),
			Func: func() error {
				crd, err = getter.Get(name)
				return errors.Trace(err)
			},
			IsFatalError: func(err error) bool {
				return err != nil && !errors.IsNotFound(err)
			},
			NotifyFunc: func(err error, attempt int) {
				logger.Debugf("fetching custom resource definition %q, err %#v, attempt %v", name, err, attempt)
			},
		})
		if err == nil {
			select {
			case resultChan <- crd:
			}
		} else {
			select {
			case errChan <- err:
			}
		}
		wg.Done()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for name := range crs {
		go getCRD(ctx, name, getter, crdChan, errChan, k.clock)
	}

	for range crs {
		select {
		case crd := <-crdChan:
			if crd == nil {
				continue
			}
			name := crd.GetName()
			out[name] = crd
			logger.Debugf("custom resource definition %q is ready", name)
		case err := <-errChan:
			if err != nil {
				return nil, errors.Annotatef(err, "getting custom resources")
			}
		}
	}
	return out, nil
}

func isCRDScopeNamespaced(scope apiextensionsv1beta1.ResourceScope) bool {
	return scope == apiextensionsv1beta1.NamespaceScoped
}

func (k *kubernetesClient) getCustomResourceDefinitionClient(crd *apiextensionsv1beta1.CustomResourceDefinition, version string) (dynamic.ResourceInterface, error) {
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
	return client.Namespace(k.namespace), nil
}
