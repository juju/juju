// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bytes"
	"context"
	// "fmt"
	"io"
	// "runtime/debug"

	"github.com/juju/errors"
	"github.com/kr/pretty"
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
)

var (
	objectTyper      = unstructuredscheme.NewUnstructuredObjectTyper()
	decoder          = unstructured.UnstructuredJSONScheme
	metadataAccessor = meta.NewAccessor()
)

func validator(c discovery.CachedDiscoveryInterface) (validation.Schema, error) {
	_, err := c.OpenAPISchema()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return nil, nil
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

type contentValidator interface {
	ValidateBytes(data []byte) error
}

type resourceInfo struct {
	name            string
	namespace       string
	resourceVersion string
	object          runtime.Object

	mapping *meta.RESTMapping
	schema  contentValidator
	client  rest.Interface
	cfg     *rest.Config
}

func validateSchema(data []byte, validate contentValidator) error {
	if validate == nil {
		return nil
	}
	return validate.ValidateBytes(data)
}

func (ri *resourceInfo) restMapper() meta.RESTMapper {
	discoveryClient := discovery.NewDiscoveryClient(ri.client)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(discoveryClient),
	)
	return restmapper.NewShortcutExpander(mapper, discoveryClient)
}

func (ri *resourceInfo) withNamespace(namespace string) *resourceInfo {
	if ri.namespace != "" && ri.namespace != namespace {
		logger.Criticalf("namespace is force set from %q to %q", ri.namespace, namespace)
	}
	ri.namespace = namespace
	return ri
}

type deployer struct {
	namespace     string
	spec          string
	cfg           *rest.Config
	newRestClient NewRestClient
	// TODO implement builder then remove ""k8s.io/cli-runtime" dep!!
	// builder *runtimeresource.Builder

	resources []resourceInfo
	sources   []string
}

type NewRestClient func(config *rest.Config) (rest.Interface, error)

func NewDeployer(
	namespace string, // TODO: force set namespace for all resources!!!
	cfg *rest.Config,
	newRestClient NewRestClient,
	spec string,
) DeployerInterface {
	return &deployer{
		namespace:     namespace,
		spec:          spec,
		cfg:           cfg,
		newRestClient: newRestClient,
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
	return nil
}

func setConfigDefaults(config *rest.Config) {
	// gv := metav1.SchemeGroupVersion
	gv := schema.GroupVersion{Group: "", Version: "v1"}
	config.GroupVersion = &gv
	config.APIPath = "/api"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
}

func (d *deployer) clientWithGroupVersion(gv schema.GroupVersion) (c rest.Interface, err error) {
	// cfg := rest.CopyConfig(d.cfg)
	cfg, _ := rest.InClusterConfig()
	setConfigDefaults(cfg)

	// cfg.GroupVersion = &gv
	// cfg.APIPath = "/apis"
	// if len(gv.Group) == 0 {
	// 	cfg.APIPath = "/api"
	// }
	// if cfg.ContentConfig.NegotiatedSerializer == nil {
	// 	cfg.ContentConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	// }
	// if len(cfg.UserAgent) == 0 {
	// 	cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	// }
	logger.Criticalf("clientWithGroupVersion 0 err -> %#v, c -> %s, cfg.GroupVersion -> %s", err, pretty.Sprint(c), pretty.Sprint(cfg.GroupVersion))
	if c, err = rest.RESTClientFor(cfg); err != nil {
		logger.Criticalf("clientWithGroupVersion 1 err -> %#v, c -> %s", err, pretty.Sprint(c))
		panic(err)
		// return nil, errors.Trace(err)
	}
	logger.Criticalf("clientWithGroupVersion 2 c -> %s", pretty.Sprint(c))
	return c, nil
}

func (d *deployer) load() (err error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(d.spec), len(d.spec))
	defer func() {
		logger.Criticalf("load d.resources -> %s, err -> %#v", pretty.Sprint(d.resources), err)
	}()
	for {
		cfg := *d.cfg
		item := resourceInfo{
			cfg: &cfg,
			// TODO: schema: xxxx for validation
		}
		ext := runtime.RawExtension{}
		if err = decoder.Decode(&ext); err != nil {
			if err == io.EOF {
				return nil
			}
			return errors.Trace(err)
		}
		ext.Raw = bytes.TrimSpace(ext.Raw)
		if len(ext.Raw) == 0 || bytes.Equal(ext.Raw, []byte("null")) {
			continue
		}
		if err = validateSchema(ext.Raw, item.schema); err != nil {
			return errors.Trace(err)
		}

		var gvk *schema.GroupVersionKind
		item.object, gvk, err = processRawData(ext.Raw, nil, nil)
		if err != nil {
			return errors.Trace(err)
		}

		item.name, err = metadataAccessor.Name(item.object)
		if err != nil {
			return errors.Trace(err)
		}

		item.namespace, err = metadataAccessor.Namespace(item.object)
		if err != nil {
			return errors.Trace(err)
		}

		item.resourceVersion, err = metadataAccessor.ResourceVersion(item.object)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Criticalf("load 3 gvk.GroupVersion() -> %s", pretty.Sprint(gvk.GroupVersion()))
		if item.client, err = d.clientWithGroupVersion(gvk.GroupVersion()); err != nil {
			return errors.Trace(err)
		}
		logger.Criticalf("load 4 item -> %s, err -> %#v", pretty.Sprint(item), err)
		item.mapping, err = item.restMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
		logger.Criticalf("load 5 item -> %s, err -> %#v", pretty.Sprint(item), err)
		if err != nil {
			return errors.Trace(err)
		}
		d.resources = append(d.resources, item)
	}
}

func apply(ctx context.Context, namespace string, r resourceInfo, force bool) (runtime.Object, error) {
	// Ensures namespace are correctly set.
	_ = r.withNamespace(namespace)

	logger.Criticalf("apply namespace -> %q, r -> %s", namespace, pretty.Sprint(r))

	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, r.object)
	if err != nil {
		return nil, errors.Trace(err)
	}
	options := &metav1.PatchOptions{
		Force:        &force,
		FieldManager: "juju",
	}

	o, err := r.client.Patch(types.ApplyPatchType).
		Context(ctx).
		NamespaceIfScoped(r.namespace, r.mapping.Scope.Name() == meta.RESTScopeNameNamespace).
		Resource(r.mapping.Resource.Resource).
		Name(r.name).
		VersionedParams(options, metav1.ParameterCodec).
		Body(data).
		Do().
		Get()
	logger.Criticalf("apply o -> %s, err -> %#v", pretty.Sprint(o), err)
	return o, errors.Trace(err)
}

func (d deployer) Deploy(ctx context.Context, force bool) error {
	if err := d.validate(); err != nil {
		return errors.Trace(err)
	}
	for _, r := range d.resources {
		if _, err := apply(ctx, d.namespace, r, force); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
