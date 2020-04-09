// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"bytes"
	"context"
	"io"

	"github.com/juju/errors"
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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/kubectl/pkg/validation"
	// runtimeresource "k8s.io/cli-runtime/pkg/resource"
)

var (
	objectTyper = unstructuredscheme.NewUnstructuredObjectTyper()
	decoder     = unstructured.UnstructuredJSONScheme
)

type metadataAccessor = meta.MetadataAccessor

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
	obj, gvk, err = decoder.Decode(data, defaults, into)
	if err != nil {
		return obj, gvk, err
	}

	if _, ok := obj.(runtime.Unstructured); !ok {
		return obj, gvk, nil
	}

	v := &metadataOnlyObject{}
	if err = json.CaseSensitiveJsonIterator().Unmarshal(data, v); err != nil {
		return obj, gvk, err
	}
	return obj, gvk, err
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
	cfg     rest.Config
}

func validateSchema(data []byte, validate contentValidator) error {
	if validate == nil {
		return nil
	}
	return validate.ValidateBytes(data)
}

func (ri *resourceInfo) clientWithGroupVersion(gv schema.GroupVersion) (rest.Interface, error) {
	ri.cfg.GroupVersion = &gv
	if len(gv.Group) == 0 {
		ri.cfg.APIPath = "/api"
	} else {
		ri.cfg.APIPath = "/apis"
	}
	return rest.RESTClientFor(&ri.cfg)
}

func (ri *resourceInfo) restMapper() (meta.RESTMapper, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(&ri.cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
	return restmapper.NewShortcutExpander(mapper, discoveryClient), nil
}

func (ri *resourceInfo) withNamespace(namespace string) *resourceInfo {
	ri.namespace = namespace
	return ri
}

type deployer struct {
	spec string
	cfg  *rest.Config
	// TODO implement builder then remove ""k8s.io/cli-runtime" dep!!
	// builder *runtimeresource.Builder

	resources []resourceInfo
	sources   []string
}

func NewDeployer(cfg *rest.Config, spec string) DeployerInterface {
	return &deployer{
		spec: spec,
		cfg:  cfg,
	}
}

func (d *deployer) validate() error {
	// TODO: validate!!

	if d.load(d.spec); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (d *deployer) load(in string) (err error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewBufferString(in), 4096)
	for {
		ext := runtime.RawExtension{}
		item := resourceInfo{
			cfg: *d.cfg,
		}
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

		item.client, err = item.clientWithGroupVersion(gv)
		if err != nil {
			return nil, err
		}

		restMapper, err := item.restMapper()
		if err != nil {
			return nil, err
		}
		item.mapping, err = restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return errors.Trace(err)
		}
		d.resources = append(d.resources, item)
	}
	return nil
}

func (d deployer) apply(ctx context.Context, r resourceInfo, force bool) (runtime.Object, error) {
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, r.object)
	if err != nil {
		return nil, errors.Trace(err)
	}
	options := &metav1.PatchOptions{
		Force:        &force,
		FieldManager: "juju",
	}

	return r.client.Patch(types.ApplyPatchType).
		Context(ctx).
		NamespaceIfScoped(r.namespace, r.mapping.Scope.Name() == meta.RESTScopeNameNamespace).
		Resource(r.mapping.Resource.Resource).
		Name(r.name).
		VersionedParams(options, metav1.ParameterCodec).
		Body(data).
		Do().
		Get()

}

func (d deployer) Deploy(ctx context.Context, force bool) error {
	if err := d.validate(); err != nil {
		return errors.Trace(err)
	}
	for _, r := range d.resources {
		if _, err := d.apply(ctx, r, force); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
