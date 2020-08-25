// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

// PersistentVolumeClaim extends the k8s persistentVolumeClaim.
type PersistentVolumeClaim struct {
	corev1.PersistentVolumeClaim
}

// NewPersistentVolumeClaim creates a new persistent volume claim resource.
func NewPersistentVolumeClaim(name string, namespace string, in *corev1.PersistentVolumeClaim) *PersistentVolumeClaim {
	if in == nil {
		in = &corev1.PersistentVolumeClaim{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &PersistentVolumeClaim{*in}
}

// Clone returns a copy of the resource.
func (pvc *PersistentVolumeClaim) Clone() Resource {
	clone := *pvc
	return &clone
}

// Apply patches the resource change.
func (pvc *PersistentVolumeClaim) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &pvc.PersistentVolumeClaim)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, pvc.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	pvc.PersistentVolumeClaim = *res
	return nil
}

// Get refreshes the resource.
func (pvc *PersistentVolumeClaim) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	res, err := api.Get(ctx, pvc.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	pvc.PersistentVolumeClaim = *res
	return nil
}

// Delete removes the resource.
func (pvc *PersistentVolumeClaim) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	err := api.Delete(ctx, pvc.Name, metav1.DeleteOptions{
		PropagationPolicy: &k8sconstants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Events returns events of the resource.
func (pvc *PersistentVolumeClaim) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	selector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", pvc.Name),
		fields.OneTermEqualSelector("involvedObject.kind", "PersistentVolumeClaim"),
	).String()
	eventList, err := client.CoreV1().Events(pvc.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: selector,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return eventList.Items, nil
}
