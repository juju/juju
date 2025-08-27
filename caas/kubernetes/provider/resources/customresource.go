// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
	"github.com/juju/retry"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// CustomResource extends the k8s CustomResource.
type CustomResource struct {
	client dynamic.ResourceInterface
	unstructured.Unstructured
}

// NewCustomResource creates a new CustomResource resource.
func NewCustomResource(client dynamic.ResourceInterface, name string, in *unstructured.Unstructured) *CustomResource {
	if in == nil {
		in = &unstructured.Unstructured{}
	}

	in.SetName(name)
	return &CustomResource{
		client,
		*in,
	}
}

// Clone returns a copy of the resource.
func (cr *CustomResource) Clone() Resource {
	clone := *cr
	clone.Unstructured = *cr.Unstructured.DeepCopy() // unstructured object field is a map, requires deep copy
	return &clone
}

// ID returns a comparable ID for the Resource
func (cr *CustomResource) ID() ID {
	return ID{"CustomResource", cr.GetName(), cr.GetNamespace()}
}

// Apply patches the resource change.
func (cr *CustomResource) Apply(ctx context.Context) (err error) {
	if cr.Unstructured.GetAPIVersion() == "" || cr.Unstructured.GetKind() == "" {
		return errors.NotValidf("both apiVersion and kind must be set on CustomResource %q", cr.Unstructured.GetName())
	}

	// attempt to create first, then patch if it already exists
	if _, err := cr.client.Create(ctx, &cr.Unstructured, metav1.CreateOptions{}); err == nil {
		return nil
	} else if !k8serrors.IsAlreadyExists(err) {
		return errors.Annotatef(err, "creating CustomResource %q", cr.GetName())
	}

	existing, err := cr.client.Get(ctx, cr.GetName(), metav1.GetOptions{})
	if err != nil {
		return errors.Annotatef(err, "retrieving existing CustomResource %q for patch", cr.GetName())
	}

	cr.SetResourceVersion(existing.GetResourceVersion())
	updated, err := cr.client.Update(ctx, &cr.Unstructured, metav1.UpdateOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "custom resource %q", cr.GetName())
	}
	if err != nil {
		return errors.Trace(err)
	}

	cr.Unstructured = *updated
	return nil
}

// Get refreshes the resource.
func (cr *CustomResource) Get(ctx context.Context) error {
	res, err := cr.client.Get(context.TODO(), cr.GetName(), metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("custom resource: %q", cr.GetName())
	} else if err != nil {
		return errors.Trace(err)
	}
	cr.Unstructured = *res
	return nil
}

// Delete removes the resource.
func (cr *CustomResource) Delete(ctx context.Context) error {
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			err := cr.client.Delete(ctx, cr.GetName(), metav1.DeleteOptions{
				PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
			})
			if k8serrors.IsNotFound(err) {
				return nil
			}
			if k8serrors.IsConflict(err) {
				_ = cr.Get(ctx) // refresh resource version
				return err
			}
			return errors.Annotatef(err, "deleting custom resource %q", cr.GetName())
		},
		IsFatalError: func(err error) bool {
			return !k8serrors.IsConflict(err)
		},
		Clock:       jujuclock.WallClock,
		Attempts:    5,
		Delay:       time.Second,
		BackoffFunc: retry.ExpBackoff(time.Second, 5*time.Second, 1.5, true),
	})
	return errors.Trace(err)
}

// ComputeStatus returns a juju status for the resource.
func (cr *CustomResource) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if cr.GetDeletionTimestamp() != nil {
		return "", status.Terminated, cr.GetDeletionTimestamp().Time, nil
	}
	return "", status.Active, now, nil
}

// ListCRsForCRD lists CR instances for a given CRD across all served versions.
// For namespaced CRDs, a non-empty namespace is required.
// The following error types can be returned:
// - [errors.NotValid]: When namespace is empty for a namespaced CRD.
func ListCRsForCRD(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	namespace string,
	crd *apiextv1.CustomResourceDefinition,
	opts metav1.ListOptions,
) ([]*CustomResource, error) {
	var items []*CustomResource
	// Iterate only served versions
	for _, ver := range crd.Spec.Versions {
		if !ver.Served {
			continue
		}

		gvr := schema.GroupVersionResource{
			Group:    crd.Spec.Group,
			Version:  ver.Name,
			Resource: crd.Spec.Names.Plural,
		}

		api := dynamicClient.Resource(gvr)
		var err error
		var res *unstructured.UnstructuredList

		if crd.Spec.Scope == apiextv1.NamespaceScoped {

			if namespace == "" {
				return nil, errors.NotValidf("namespace is empty for namespaced cr %q", crd.GetName())
			}

			namespacedAPI := api.Namespace(namespace)
			res, err = namespacedAPI.List(ctx, opts)
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil, errors.Annotatef(err, "listing CRs for %s of group %s and ver %s in ns %q",
					crd.GetName(), crd.Spec.Group, ver.Name, namespace)
			}

			if res == nil {
				continue
			}

			for i := range res.Items {
				item := res.Items[i]
				items = append(items, NewCustomResource(namespacedAPI, item.GetName(), &item))
			}

		} else {
			res, err = api.List(ctx, opts)
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil, errors.Annotatef(err, "listing CRs for %s of group %s and ver %s",
					crd.GetName(), crd.Spec.Group, ver.Name)
			}

			if res == nil {
				continue
			}

			for i := range res.Items {
				item := res.Items[i]
				items = append(items, NewCustomResource(api, item.GetName(), &item))
			}
		}
	}
	return items, nil
}

// GetObjectMeta returns a synthetic *metav1.ObjectMeta constructed from the unstructured object.
// This allows CustomResource to satisfy metav1.ObjectMetaAccessor.
func (cr *CustomResource) GetObjectMeta() metav1.Object {
	return &metav1.ObjectMeta{
		Name:            cr.GetName(),
		Namespace:       cr.GetNamespace(),
		Labels:          cr.GetLabels(),
		Annotations:     cr.GetAnnotations(),
		OwnerReferences: cr.GetOwnerReferences(),
		Finalizers:      cr.GetFinalizers(),
		ResourceVersion: cr.GetResourceVersion(),
		UID:             cr.GetUID(),
	}
}

func (cr *CustomResource) String() string {
	return cr.GetName()
}
