// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// StatefulSet extends the k8s statefulSet.
type CustomResourceDefinition struct {
	apiextensionsv1.CustomResourceDefinition
}

// NewStatefulSet creates a new statefulset resource.
func NewCustomResourceDefinition(name string, in *apiextensionsv1.CustomResourceDefinition) *CustomResourceDefinition {
	if in == nil {
		in = &apiextensionsv1.CustomResourceDefinition{}
	}
	in.SetName(name)
	return &CustomResourceDefinition{*in}
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
func (crd *CustomResourceDefinition) Apply(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) (err error) {
	api := extendedClient.ApiextensionsV1().CustomResourceDefinitions()
	existing, err := api.Get(ctx, crd.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Create if not found
		created, err := api.Create(ctx, &crd.CustomResourceDefinition, metav1.CreateOptions{
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
	updated, err := api.Update(ctx, &crd.CustomResourceDefinition, metav1.UpdateOptions{
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
func (crd *CustomResourceDefinition) Get(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := extendedClient.ApiextensionsV1().CustomResourceDefinitions()
	res, err := api.Get(context.TODO(), crd.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("custom resource definition: %q", crd.Name)
	} else if err != nil {
		return errors.Trace(err)
	}
	crd.CustomResourceDefinition = *res
	logger.Infof("alvin Get res: %+v", res)
	return nil
}

// Delete removes the resource.
func (crd *CustomResourceDefinition) Delete(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	logger.Infof("alvin crd in del is %+v", *crd)
	api := extendedClient.ApiextensionsV1().CustomResourceDefinitions()

	err := api.Delete(ctx, crd.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	logger.Infof("alvin logger called for %s and err is %v", crd.Name, err)

	err = crd.Get(ctx, coreClient, extendedClient)
	logger.Infof("alvin Get err in Del is %v", err)
	logger.Infof("alvin Get res after del in Del is %+v", *crd)
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Events emitted by the resource.
func (crd *CustomResourceDefinition) Events(ctx context.Context, coreClient kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, coreClient, crd.Namespace, crd.Name, "CustomResourceDefinition")
}

// ComputeStatus returns a juju status for the resource.
func (crd *CustomResourceDefinition) ComputeStatus(ctx context.Context, coreClient kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if crd.DeletionTimestamp != nil {
		return "", status.Terminated, crd.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}
