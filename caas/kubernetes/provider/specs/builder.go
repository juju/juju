// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bytes"
	"context"
	// "fmt"
	"io"
	"sync"
	// "runtime/debug"

	"github.com/juju/errors"
	"github.com/kr/pretty"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured/unstructuredscheme"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	// "k8s.io/client-go/kubernetes"
	// runtimeresource "k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/kubectl/pkg/validation"
	// "github.com/juju/collections/set"

	"github.com/juju/juju/caas"
	k8sannotations "github.com/juju/juju/core/annotations"
)

var (
	objectTyper      = unstructuredscheme.NewUnstructuredObjectTyper()
	decoder          = unstructured.UnstructuredJSONScheme
	metadataAccessor = meta.NewAccessor()
)

func getValidator(c discovery.CachedDiscoveryInterface) (validation.Schema, error) {
	// TODO: get schema for validation

	_, err := c.OpenAPISchema()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return nil, nil
}

func validateSchema(data []byte, schema validation.Schema) error {
	if schema == nil {
		return nil
	}
	return schema.ValidateBytes(data)
}

type metadataOnlyObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

func processRawData(data []byte, defaults *schema.GroupVersionKind, into runtime.Object) (obj runtime.Object, gvk *schema.GroupVersionKind, err error) {
	logger.Criticalf("processRawData data -> %s", string(data))
	obj, gvk, err = decoder.Decode(data, defaults, into)
	logger.Criticalf("processRawData 1 err -> %#v obj -> %s, gvk -> %s", err, pretty.Sprint(obj), pretty.Sprint(gvk))
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
	logger.Criticalf("processRawData 2 err -> %#v obj -> %s, gvk -> %s", err, pretty.Sprint(obj), pretty.Sprint(gvk))
	return obj, gvk, nil
}

type DeployerInterface interface {
	Deploy(context.Context, bool) error
}

type resourceInfo struct {
	name            string
	namespace       string
	resourceVersion string
	content         *runtime.RawExtension

	mapping         *meta.RESTMapping
	schema          validation.Schema
	client          rest.Interface
	discoveryClient discovery.CachedDiscoveryInterface
}

func (ri *resourceInfo) restMapper() meta.RESTMapper {
	discoveryClient := discovery.NewDiscoveryClient(ri.client)
	ri.discoveryClient = memory.NewMemCacheClient(discoveryClient)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(
		ri.discoveryClient,
	)
	return restmapper.NewShortcutExpander(mapper, discoveryClient)
}

func (ri *resourceInfo) validateSchema() error {
	schema, err := getValidator(ri.discoveryClient)
	if err != nil {
		return errors.Trace(err)
	}
	return validateSchema(ri.content.Raw, schema)
}

func (ri *resourceInfo) withNamespace(namespace string) *resourceInfo {
	if ri.namespace != "" && ri.namespace != namespace {
		logger.Criticalf("namespace is force set from %q to %q", ri.namespace, namespace)
	}
	ri.namespace = namespace
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

type deployer struct {
	deploymentName       string
	namespace            string
	spec                 string
	workloadResourceType string
	cfg                  *rest.Config
	labelGetter          func(isNamespaced bool) map[string]string
	annotations          k8sannotations.Annotation

	resources []resourceInfo
	sources   []string
}

func NewDeployer(
	deploymentName string,
	namespace string,
	spec string,
	deploymentParams caas.DeploymentParams,
	cfg *rest.Config,
	labelGetter func(isNamespaced bool) map[string]string,
	annotations k8sannotations.Annotation,
) DeployerInterface {
	return &deployer{
		deploymentName:       deploymentName,
		namespace:            namespace,
		spec:                 spec,
		workloadResourceType: getWorkloadResourceType(deploymentParams.DeploymentType),
		cfg:                  cfg,
		labelGetter:          labelGetter,
		annotations:          annotations,
	}
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

func (d *deployer) validate() error {
	// TODO: validate!!
	if d.cfg == nil {
		return errors.NotValidf("empty k8s config")
	}

	if err := d.load(); err != nil {
		return errors.Trace(err)
	}
	if err := d.validateWorkload(); err != nil {
		return errors.Trace(err)
	}

	// TODO: do we need to check if service resource type matches the raw service spec?
	return nil
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

func (d *deployer) clientWithGroupVersion(gv schema.GroupVersion) (c rest.Interface, err error) {
	cfg := rest.CopyConfig(d.cfg)
	// cfg, _ := rest.InClusterConfig()
	setConfigDefaults(cfg)

	cfg.APIPath = "/apis"
	if len(gv.Group) == 0 {
		cfg.APIPath = "/api"
	}
	cfg.GroupVersion = &gv

	logger.Criticalf("clientWithGroupVersion 0 err -> %#v, c -> %#v, cfg.GroupVersion -> %s", err, c, pretty.Sprint(cfg.GroupVersion))
	if c, err = rest.RESTClientFor(cfg); err != nil {
		return nil, errors.Trace(err)
	}
	return c, nil
}

func (d *deployer) load() (err error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(d.spec), len(d.spec))
	defer func() {
		logger.Criticalf("load len(d.resources) -> %d, err -> %#v", len(d.resources), err)
	}()
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

		var gvk *schema.GroupVersionKind
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

		item.mapping, err = item.restMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return errors.Trace(err)
		}

		if err = item.validateSchema(); err != nil {
			return errors.Trace(err)
		}

		d.resources = append(d.resources, item)
	}
}

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

	logger.Criticalf("apply namespace -> %q, r -> %#v", d.namespace, info)

	var data []byte
	data, err = runtime.Encode(unstructured.UnstructuredJSONScheme, info.content.Object)
	if err != nil {
		return errors.Trace(err)
	}
	options := &metav1.PatchOptions{
		Force:        &force,
		FieldManager: "juju",
	}

	doRequest := func(r *rest.Request) error {
		_, err := r.Context(ctx).
			NamespaceIfScoped(info.namespace, isNameSpaced).
			Resource(info.mapping.Resource.Resource).
			Name(info.name).
			VersionedParams(options, metav1.ParameterCodec).
			Body(data).
			Do().
			Get()
		return errors.Trace(err)
	}

	err = doRequest(info.client.Patch(types.ApplyPatchType))
	if k8serrors.IsNotFound(err) {
		err = doRequest(info.client.Post())
	}
	return errors.Trace(err)
}

func (d deployer) Deploy(ctx context.Context, force bool) error {
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
