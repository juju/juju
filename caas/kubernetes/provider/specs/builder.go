// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/juju/errors"
	"github.com/kr/pretty"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apischema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	"github.com/juju/juju/caas"
	k8sannotations "github.com/juju/juju/core/annotations"
)

var (
	codec            = unstructured.UnstructuredJSONScheme
	metadataAccessor = meta.NewAccessor()
)

type metadataOnlyObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func processRawData(data []byte, defaults *apischema.GroupVersionKind, into runtime.Object) (obj runtime.Object, gvk *apischema.GroupVersionKind, err error) {
	obj, gvk, err = codec.Decode(data, defaults, into)
	if err != nil {
		return obj, gvk, errors.Trace(err)
	}

	if _, ok := obj.(runtime.Unstructured); !ok {
		return obj, gvk, errors.Trace(nil)
	}

	// Make sure the data can decode into ObjectMeta.
	v := &metadataOnlyObject{}
	if err = json.CaseSensitiveJsonIterator().Unmarshal(data, v); err != nil {
		return obj, gvk, errors.Trace(err)
	}
	return obj, gvk, nil
}

type resourceInfo struct {
	name            string
	namespace       string
	resourceVersion string
	content         *runtime.RawExtension

	mapping *meta.RESTMapping
	client  rest.Interface
}

//go:generate mockgen -package mocks -destination mocks/meta_mock.go k8s.io/apimachinery/pkg/api/meta RESTMapper
func getRestMapper(c rest.Interface) meta.RESTMapper {
	discoveryClient := discovery.NewDiscoveryClient(c)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(discoveryClient),
	)
	return restmapper.NewShortcutExpander(mapper, discoveryClient)
}

func (ri *resourceInfo) withNamespace(namespace string) *resourceInfo {
	if ri.namespace != "" && ri.namespace != namespace {
		logger.Debugf("namespace is force set from %q to %q", ri.namespace, namespace)
	}
	ri.namespace = namespace
	metadataAccessor.SetNamespace(ri.content.Object, ri.namespace)
	return ri
}

func (ri *resourceInfo) ensureLabels(labels map[string]string) error {
	providedLabels, err := metadataAccessor.Labels(ri.content.Object)
	if err != nil {
		return errors.Trace(err)
	}
	if len(providedLabels) == 0 {
		providedLabels = make(map[string]string)
	}
	for k, v := range labels {
		providedLabels[k] = v
	}
	return metadataAccessor.SetLabels(ri.content.Object, providedLabels)
}

func (ri *resourceInfo) ensureAnnotations(annoations k8sannotations.Annotation) error {
	providedAnnoations, err := metadataAccessor.Annotations(ri.content.Object)
	if err != nil {
		return errors.Trace(err)
	}
	return metadataAccessor.SetAnnotations(
		ri.content.Object, k8sannotations.New(providedAnnoations).Merge(annoations).ToMap(),
	)
}

func getWorkloadResourceType(t caas.DeploymentType) string {
	switch t {
	case caas.DeploymentDaemon:
		return "daemonsets"
	case caas.DeploymentStateless:
		return "deployments"
	case caas.DeploymentStateful:
		return "statefulsets"
	default:
		return "deployments"
	}
}

type deployer struct {
	deploymentName       string
	namespace            string
	spec                 string
	workloadResourceType string
	cfg                  *rest.Config
	labelGetter          func(isNamespaced bool) map[string]string
	annotations          k8sannotations.Annotation
	newRestClient        NewK8sRestClientFunc

	resources []resourceInfo

	restMapperGetter func(c rest.Interface) meta.RESTMapper
}

// DeployerInterface defines method to deploy a raw k8s spec.
type DeployerInterface interface {
	Deploy(context.Context, string, bool) error
}

// NewK8sRestClientFunc defines a function which returns a k8s rest client based on the supplied config.
type NewK8sRestClientFunc func(c *rest.Config) (rest.Interface, error)

// New constructs deployer interface.
func New(
	deploymentName string,
	namespace string,
	deploymentParams caas.DeploymentParams,
	cfg *rest.Config,
	labelGetter func(isNamespaced bool) map[string]string,
	annotations k8sannotations.Annotation,
	newRestClient NewK8sRestClientFunc,
) DeployerInterface {
	// TODO(caas): disable scale or parse the unstructuredJSON further to set workload resource replicas.
	return newDeployer(
		deploymentName, namespace,
		deploymentParams, cfg, labelGetter, annotations,
		newRestClient, getRestMapper,
	)
}

func newDeployer(
	deploymentName string,
	namespace string,
	deploymentParams caas.DeploymentParams,
	cfg *rest.Config,
	labelGetter func(isNamespaced bool) map[string]string,
	annotations k8sannotations.Annotation,
	newRestClient NewK8sRestClientFunc,
	restMapperGetter func(c rest.Interface) meta.RESTMapper,
) DeployerInterface {
	return &deployer{
		deploymentName:       deploymentName,
		namespace:            namespace,
		workloadResourceType: getWorkloadResourceType(deploymentParams.DeploymentType),
		cfg:                  cfg,
		labelGetter:          labelGetter,
		annotations:          annotations,
		newRestClient:        newRestClient,
		restMapperGetter:     restMapperGetter,
	}
}

func (d *deployer) validate() error {
	if len(d.namespace) == 0 {
		return errors.NotValidf("namespace is required")
	}
	if len(d.workloadResourceType) == 0 {
		return errors.NotValidf("workloadResourceType is required")
	}
	if d.cfg == nil {
		return errors.NotValidf("empty k8s config")
	}
	if d.labelGetter == nil {
		return errors.NotValidf("labelGetter is required")
	}
	if d.newRestClient == nil {
		return errors.NotValidf("newRestClient is required")
	}

	if err := d.load(); err != nil {
		return errors.Trace(err)
	}
	if err := d.validateWorkload(); err != nil {
		return errors.Trace(err)
	}
	// TODO(caas): check if service resource type matches the raw service spec.
	// TODO(caas): get the API scheme and do validation further.
	return nil
}

// Deploy deploys raw k8s spec to the cluster.
func (d *deployer) Deploy(ctx context.Context, spec string, force bool) error {
	d.spec = spec

	if err := d.validate(); err != nil {
		return errors.Trace(err)
	}
	var wg sync.WaitGroup
	wg.Add(len(d.resources))

	errChan := make(chan error)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	for _, r := range d.resources {
		info := r
		go func() { _ = d.apply(ctx, &wg, info, force, errChan) }()
	}

	for {
		select {
		case err := <-errChan:
			if err != nil {
				return errors.Trace(err)
			}
		case <-done:
			return nil
		}
	}
}

func (d *deployer) validateWorkload() error {
	for _, resource := range d.resources {
		if resource.mapping.Resource.Resource == d.workloadResourceType {
			return nil
		}
	}
	return errors.NotValidf("empty %q resource definition", d.workloadResourceType)
}

func setConfigDefaults(config *rest.Config) {
	if config.ContentConfig.NegotiatedSerializer == nil {
		config.ContentConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}
	if len(config.UserAgent) == 0 {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
}

func (d *deployer) clientWithGroupVersion(gv apischema.GroupVersion) (rest.Interface, error) {
	cfg := rest.CopyConfig(d.cfg)
	setConfigDefaults(cfg)

	cfg.APIPath = "/apis"
	if len(gv.Group) == 0 {
		cfg.APIPath = "/api"
	}
	cfg.GroupVersion = &gv

	logger.Debugf("constructing rest client for resource %s for %q", pretty.Sprint(cfg.GroupVersion), d.deploymentName)
	return d.newRestClient(cfg)
}

// load parses the raw k8s spec into a slice of resource info.
func (d *deployer) load() (err error) {
	defer func() {
		logger.Debugf("processing %d resources for %q, err -> %#v", len(d.resources), d.deploymentName, err)
	}()

	d.resources = []resourceInfo{}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(d.spec), len(d.spec))
	for {
		ext := &runtime.RawExtension{}
		if err = decoder.Decode(ext); err != nil {
			if err == io.EOF {
				return nil
			}
			return errors.Trace(err)
		}
		ext.Raw = bytes.TrimSpace(ext.Raw)
		if len(ext.Raw) == 0 || bytes.Equal(ext.Raw, []byte("null")) {
			continue
		}

		var gvk *apischema.GroupVersionKind
		ext.Object, gvk, err = processRawData(ext.Raw, nil, nil)
		if err != nil {
			return errors.Trace(err)
		}

		item := resourceInfo{
			content: ext,
		}

		item.name, err = metadataAccessor.Name(item.content.Object)
		if err != nil {
			return errors.Trace(err)
		}

		item.namespace, err = metadataAccessor.Namespace(item.content.Object)
		if err != nil {
			return errors.Trace(err)
		}

		item.resourceVersion, err = metadataAccessor.ResourceVersion(item.content.Object)
		if err != nil {
			return errors.Trace(err)
		}

		if item.client, err = d.clientWithGroupVersion(gvk.GroupVersion()); err != nil {
			return errors.Trace(err)
		}
		item.mapping, err = d.restMapperGetter(item.client).RESTMapping(gvk.GroupKind(), gvk.Version)
		logger.Tracef("gvk.GroupKind() %s, gvk.Version %q, item.mapping %s", pretty.Sprint(gvk.GroupKind()), gvk.Version, pretty.Sprint(item.mapping))
		if err != nil {
			return errors.Trace(err)
		}

		d.resources = append(d.resources, item)
	}
}

// apply deploys the resource info to the k8s cluster.
func (d deployer) apply(ctx context.Context, wg *sync.WaitGroup, info resourceInfo, force bool, errChan chan<- error) (err error) {
	defer wg.Done()

	defer func() {
		if err != nil {
			select {
			case errChan <- err:
			default:
			}
		}
	}()

	isNameSpaced := info.mapping.Scope.Name() == meta.RESTScopeNameNamespace

	// Ensures namespace is set.
	_ = info.withNamespace(d.namespace)
	// Ensure Juju labels are set.
	if err = info.ensureLabels(d.labelGetter(isNameSpaced)); err != nil {
		return errors.Trace(err)
	}
	// Ensure annotations are set.
	if err = info.ensureAnnotations(d.annotations); err != nil {
		return errors.Trace(err)
	}

	var data []byte
	data, err = runtime.Encode(codec, info.content.Object)
	if err != nil {
		return errors.Trace(err)
	}
	options := &metav1.PatchOptions{
		Force:        &force,
		FieldManager: "juju",
	}

	doRequest := func(r *rest.Request) error {
		err := r.Context(ctx).
			NamespaceIfScoped(info.namespace, isNameSpaced).
			Resource(info.mapping.Resource.Resource).
			Name(info.name).
			VersionedParams(options, metav1.ParameterCodec).
			Body(data).
			Do().
			Error()
		errMsg := fmt.Sprintf("resource %s/%s in namespace %q", info.mapping.GroupVersionKind.Kind, info.name, d.namespace)
		if k8serrors.IsNotFound(err) {
			return errors.NotFoundf(errMsg)
		}
		if k8serrors.IsAlreadyExists(err) {
			return errors.AlreadyExistsf(errMsg)
		}
		return errors.Trace(err)
	}

	err = doRequest(info.client.Patch(types.ApplyPatchType))
	if errors.IsNotFound(err) {
		err = doRequest(info.client.Post())
	}
	return errors.Trace(err)
}
