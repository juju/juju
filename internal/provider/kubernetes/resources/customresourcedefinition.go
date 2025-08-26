// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	v1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// CustomResourceDefinition extends the k8s customresourcedefinition.
type CustomResourceDefinition struct {
	client v1.CustomResourceDefinitionInterface
	apiextensionsv1.CustomResourceDefinition
}

// CustomResourceDefinition creates a new customresourcedefinition resource.
func NewCustomResourceDefinition(client v1.CustomResourceDefinitionInterface, name string, in *apiextensionsv1.CustomResourceDefinition) *CustomResourceDefinition {
	if in == nil {
		in = &apiextensionsv1.CustomResourceDefinition{}
	}

	in.SetName(name)
	return &CustomResourceDefinition{
		client,
		*in,
	}
}

// Clone returns a copy of the resource.
func (crd *CustomResourceDefinition) Clone() Resource {
	clone := *crd
	return &clone
}

// ID returns a comparable ID for the Resource
func (crd *CustomResourceDefinition) ID() ID {
	return ID{"CustomResourceDefinition", crd.Name, crd.Namespace}
}

// Apply patches the resource change.
func (crd *CustomResourceDefinition) Apply(ctx context.Context) (err error) {
	existing, err := crd.client.Get(ctx, crd.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Create if not found
		created, err := crd.client.Create(ctx, &crd.CustomResourceDefinition, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
		if err != nil {
			return errors.Trace(err)
		}
		crd.CustomResourceDefinition = *created
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// Update if exists (set ResourceVersion to prevent conflict)
	crd.ResourceVersion = existing.ResourceVersion
	updated, err := crd.client.Update(ctx, &crd.CustomResourceDefinition, metav1.UpdateOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "customresourcedefinition %q", crd.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}

	crd.CustomResourceDefinition = *updated
	return nil
}

// Get refreshes the resource.
func (crd *CustomResourceDefinition) Get(ctx context.Context) error {
	res, err := crd.client.Get(ctx, crd.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("custom resource definition: %q", crd.Name)
	} else if err != nil {
		return errors.Trace(err)
	}
	crd.CustomResourceDefinition = *res
	return nil
}

// Delete removes the resource.
func (crd *CustomResourceDefinition) Delete(ctx context.Context) error {
	err := crd.client.Delete(ctx, crd.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ComputeStatus returns a juju status for the resource.
func (crd *CustomResourceDefinition) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if crd.DeletionTimestamp != nil {
		return "", status.Terminated, crd.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// ListCRDs returns a list of CRDs.
func ListCRDs(ctx context.Context, extendedClient clientset.Interface, opts metav1.ListOptions) ([]*CustomResourceDefinition, error) {
	api := extendedClient.ApiextensionsV1().CustomResourceDefinitions()
	var items []*CustomResourceDefinition
	for {
		res, err := api.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, NewCustomResourceDefinition(api, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
