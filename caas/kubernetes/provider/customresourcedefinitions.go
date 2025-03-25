// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"golang.org/x/sync/errgroup"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/crd_getter_mock.go github.com/juju/juju/caas/kubernetes/provider CRDGetterInterface

func (k *kubernetesClient) getAPIExtensionLabelsGlobal(appName string) map[string]string {
	return utils.LabelsMerge(
		utils.LabelsForApp(appName, k.LabelVersion()),
		utils.LabelsForModel(k.ModelName(), k.ModelUUID(), k.ControllerUUID(), k.LabelVersion()),
	)
}

func (k *kubernetesClient) getAPIExtensionLabelsNamespaced(appName string) map[string]string {
	return utils.LabelsForApp(appName, k.LabelVersion())
}

func (k *kubernetesClient) getCRLabels(appName string, scope apiextensionsv1.ResourceScope) map[string]string {
	if isCRDScopeNamespaced(scope) {
		return k.getAPIExtensionLabelsNamespaced(appName)
	}
	return k.getAPIExtensionLabelsGlobal(appName)
}

// ensureCustomResourceDefinitions creates or updates a custom resource definition resource.
func (k *kubernetesClient) ensureCustomResourceDefinitions(
	appName string,
	annotations map[string]string,
	crdSpecs []k8sspecs.K8sCustomResourceDefinition,
) (cleanUps []func(), _ error) {
	k8sVersion, err := k.Version()
	if err != nil {
		return nil, errors.Annotate(err, "getting k8s api version")
	}
	for _, v := range crdSpecs {
		obj := metav1.ObjectMeta{
			Name:        v.Name,
			Labels:      k8slabels.Merge(v.Labels, k.getAPIExtensionLabelsGlobal(appName)),
			Annotations: k8sannotations.New(v.Annotations).Merge(annotations),
		}
		logger.Infof("ensuring custom resource definition %q with version %q on k8s %q", obj.GetName(), v.Spec.Version, k8sVersion.String())
		var out metav1.Object
		var crdCleanUps []func()
		var err error
		switch v.Spec.Version {
		case k8sspecs.K8sCustomResourceDefinitionV1:
			if k8sVersion.Major == 1 && k8sVersion.Minor < 16 {
				return cleanUps, errors.NotSupportedf("custom resource definition version %q for k8s %q", v.Spec.Version, k8sVersion.String())
			} else {
				out, crdCleanUps, err = k.ensureCustomResourceDefinitionV1(obj, v.Spec.SpecV1)
			}
		case k8sspecs.K8sCustomResourceDefinitionV1Beta1:
			if k8sVersion.Major == 1 && k8sVersion.Minor < 22 {
				out, crdCleanUps, err = k.ensureCustomResourceDefinitionV1beta1(obj, v.Spec.SpecV1Beta1)
			} else {
				var newSpec apiextensionsv1.CustomResourceDefinitionSpec
				newSpec, err = k8sspecs.UpgradeCustomResourceDefinitionSpecV1Beta1(v.Spec.SpecV1Beta1)
				if err != nil {
					err = errors.Annotatef(err, "cannot convert v1beta1 crd to v1")
					break
				}
				out, crdCleanUps, err = k.ensureCustomResourceDefinitionV1(obj, newSpec)
			}
		default:
			// This should never happen.
			return cleanUps, errors.NotSupportedf("custom resource definition version %q", v.Spec.Version)
		}
		cleanUps = append(cleanUps, crdCleanUps...)
		if err != nil {
			return cleanUps, errors.Annotatef(err, "ensuring custom resource definition %q with version %q", obj.GetName(), v.Spec.Version)
		}
		logger.Debugf("ensured custom resource definition %q", out.GetName())
	}
	return cleanUps, nil
}

func (k *kubernetesClient) ensureCustomResourceDefinitionV1beta1(
	obj metav1.ObjectMeta, crd apiextensionsv1beta1.CustomResourceDefinitionSpec,
) (out metav1.Object, cleanUps []func(), err error) {
	spec := &apiextensionsv1beta1.CustomResourceDefinition{ObjectMeta: obj, Spec: crd}

	api := k.extendedClient().ApiextensionsV1beta1().CustomResourceDefinitions()
	logger.Debugf("creating custom resource definition %q", spec.GetName())
	if out, err = api.Create(context.TODO(), spec, metav1.CreateOptions{}); err == nil {
		cleanUps = append(cleanUps, func() {
			_ = api.Delete(context.TODO(), out.GetName(), utils.NewPreconditionDeleteOptions(out.GetUID()))
		})
		return out, cleanUps, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return nil, cleanUps, errors.Trace(err)
	}
	logger.Debugf("updating custom resource definition %q", spec.GetName())
	// K8s complains about metadata.resourceVersion is required for an update, so get it before updating.
	existingCRD, err := api.Get(context.TODO(), spec.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	spec.SetResourceVersion(existingCRD.GetResourceVersion())
	// TODO(caas): do label check to ensure the resource to be updated was created by Juju once caas upgrade steps of 2.7 in place.
	out, err = api.Update(context.TODO(), spec, metav1.UpdateOptions{})
	return out, cleanUps, errors.Trace(err)
}

func (k *kubernetesClient) ensureCustomResourceDefinitionV1(
	obj metav1.ObjectMeta, crd apiextensionsv1.CustomResourceDefinitionSpec,
) (out metav1.Object, cleanUps []func(), err error) {
	spec := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: obj, Spec: crd}

	api := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions()
	logger.Debugf("creating custom resource definition %q", spec.GetName())
	if out, err = api.Create(context.TODO(), spec, metav1.CreateOptions{}); err == nil {
		cleanUps = append(cleanUps, func() {
			_ = api.Delete(context.TODO(), out.GetName(), utils.NewPreconditionDeleteOptions(out.GetUID()))
		})
		return out, cleanUps, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return nil, cleanUps, errors.Trace(err)
	}
	logger.Debugf("updating custom resource definition %q", spec.GetName())
	// K8s complains about metadata.resourceVersion is required for an update, so get it before updating.
	existingCRD, err := api.Get(context.TODO(), spec.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	spec.SetResourceVersion(existingCRD.GetResourceVersion())
	// TODO(caas): do label check to ensure the resource to be updated was created by Juju once caas upgrade steps of 2.7 in place.
	out, err = api.Update(context.TODO(), spec, metav1.UpdateOptions{})
	return out, cleanUps, errors.Trace(err)
}

func (k *kubernetesClient) getCustomResourceDefinition(name string) (*apiextensionsv1.CustomResourceDefinition, error) {
	crd, err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("custom resource definition %q", name)
	}
	return crd, errors.Trace(err)
}

func (k *kubernetesClient) listCustomResourceDefinitions(selector k8slabels.Selector) ([]apiextensionsv1.CustomResourceDefinition, error) {
	listOps := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), listOps)
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
		utils.LabelsToSelector(k.getAPIExtensionLabelsGlobal(appName)),
		lifecycleApplicationRemovalSelector,
	)
	return errors.Trace(k.deleteCustomResourceDefinitions(selector))
}

func (k *kubernetesClient) deleteCustomResourceDefinitions(selector k8slabels.Selector) error {
	err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().DeleteCollection(context.TODO(), metav1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteCustomResourcesForApp(appName string) error {
	selectorGetter := func(crd apiextensionsv1.CustomResourceDefinition) k8slabels.Selector {
		return mergeSelectors(
			utils.LabelsToSelector(k.getCRLabels(appName, crd.Spec.Scope)),
			lifecycleApplicationRemovalSelector,
		)
	}
	return k.deleteCustomResources(selectorGetter)
}

func (k *kubernetesClient) deleteCustomResources(selectorGetter func(apiextensionsv1.CustomResourceDefinition) k8slabels.Selector) error {
	crds, err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{
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
			err = crdClient.DeleteCollection(context.TODO(), metav1.DeleteOptions{
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

func (k *kubernetesClient) listCustomResources(selectorGetter func(apiextensionsv1.CustomResourceDefinition) k8slabels.Selector) (out []unstructured.Unstructured, err error) {
	crds, err := k.extendedClient().ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), metav1.ListOptions{
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
			list, err := crdClient.List(context.TODO(), metav1.ListOptions{
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
				utils.LabelsMerge(
					crSpec.GetLabels(),
					k.getCRLabels(appName, crd.Spec.Scope),
					utils.LabelsJuju),
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
	if out, err = api.Create(context.TODO(), cr, metav1.CreateOptions{}); err == nil {
		cleanUps = append(cleanUps, func() {
			_ = deleteCustomResourceDefinition(api, out.GetName(), out.GetUID())
		})
		return out, cleanUps, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return nil, cleanUps, errors.Trace(err)
	}
	// K8s complains about metadata.resourceVersion is required for an update, so get it before updating.
	existingCR, err := api.Get(context.TODO(), cr.GetName(), metav1.GetOptions{})
	if err != nil {
		return nil, cleanUps, errors.Trace(err)
	}
	cr.SetResourceVersion(existingCR.GetResourceVersion())
	logger.Debugf("updating custom resource %q", cr.GetName())
	out, err = api.Update(context.TODO(), cr, metav1.UpdateOptions{})
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
	Get(string) (*apiextensionsv1.CustomResourceDefinition, error)
}

type crdGetter struct {
	Broker *kubernetesClient
}

func (cg *crdGetter) Get(name string) (*apiextensionsv1.CustomResourceDefinition, error) {
	crd, err := cg.Broker.getCustomResourceDefinition(name)
	if err != nil {
		return nil, errors.Annotatef(err, "getting custom resource definition %q", name)
	}
	if len(crd.Spec.Versions) == 0 {
		return nil, errors.NotValidf("custom resource definition %q without version", crd.GetName())
	}
	version := crd.Spec.Versions[0].Name
	crClient, err := cg.Broker.getCustomResourceDefinitionClient(crd, version)
	if err != nil {
		return nil, errors.Annotatef(err, "getting custom resource definition client %q", name)
	}
	if resources, err := crClient.List(context.TODO(), metav1.ListOptions{}); err != nil {
		if k8serrors.IsNotFound(err) || len(resources.Items) == 0 {
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
) (out map[string]*apiextensionsv1.CustomResourceDefinition, err error) {
	n := len(crs)
	if n == 0 {
		return
	}

	out = make(map[string]*apiextensionsv1.CustomResourceDefinition)
	crdChan := make(chan *apiextensionsv1.CustomResourceDefinition, n)

	getCRD := func(
		ctx context.Context,
		name string,
		getter CRDGetterInterface,
		resultChan chan<- *apiextensionsv1.CustomResourceDefinition,
		clk jujuclock.Clock,
	) error {
		return retry.Call(retry.CallArgs{
			Attempts: 8,
			Delay:    1 * time.Second,
			Clock:    clk,
			Stop:     ctx.Done(),
			Func: func() error {
				if crd, err := getter.Get(name); err != nil {
					return err
				} else {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case resultChan <- crd:
					}
					return nil
				}
			},
			IsFatalError: func(err error) bool {
				return err != nil && !errors.IsNotFound(err)
			},
			NotifyFunc: func(err error, attempt int) {
				logger.Debugf("fetching custom resource definition %q, err %#v, attempt %v", name, err, attempt)
			},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)
	for name := range crs {
		n := name
		g.Go(func() error {
			return getCRD(ctx, n, getter, crdChan, k.clock)
		})
	}
	if err := g.Wait(); err != nil {
		return nil, errors.Annotatef(err, "getting custom resources")
	}
	close(crdChan)
	for crd := range crdChan {
		if crd == nil {
			continue
		}
		name := crd.GetName()
		out[name] = crd
		logger.Debugf("custom resource definition %q is ready", name)
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
