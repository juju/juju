// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// StorageClass extends the k8s storageClass.
type StorageClass struct {
	storagev1.StorageClass
}

// NewStorageClass creates a new storage class resource.
func NewStorageClass(name string, in *storagev1.StorageClass) *StorageClass {
	if in == nil {
		in = &storagev1.StorageClass{}
	}
	in.SetName(name)
	return &StorageClass{*in}
}

// ListStorageClass returns a list of storage classes.
func ListStorageClass(ctx context.Context, client kubernetes.Interface, opts metav1.ListOptions) ([]StorageClass, error) {
	api := client.StorageV1().StorageClasses()
	var items []StorageClass
	for {
		res, err := api.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, v := range res.Items {
			items = append(items, StorageClass{StorageClass: v})
		}
		if res.RemainingItemCount == nil || *res.RemainingItemCount == 0 {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}

// Clone returns a copy of the resource.
func (sc *StorageClass) Clone() Resource {
	clone := *sc
	return &clone
}

// ID returns a comparable ID for the Resource
func (sc *StorageClass) ID() ID {
	return ID{"StorageClass", sc.Name, sc.Namespace}
}

// Apply patches the resource change.
func (sc *StorageClass) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.StorageV1().StorageClasses()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &sc.StorageClass)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, sc.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &sc.StorageClass, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "storage class %q", sc.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	sc.StorageClass = *res
	return nil
}

// Get refreshes the resource.
func (sc *StorageClass) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.StorageV1().StorageClasses()
	res, err := api.Get(ctx, sc.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	sc.StorageClass = *res
	return nil
}

// Delete removes the resource.
func (sc *StorageClass) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.StorageV1().StorageClasses()
	err := api.Delete(ctx, sc.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Events emitted by the resource.
func (sc *StorageClass) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, client, "", sc.Name, "StorageClass")
}

// ComputeStatus returns a juju status for the resource.
func (sc *StorageClass) ComputeStatus(ctx context.Context, client kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if sc.DeletionTimestamp != nil {
		return "", status.Terminated, sc.DeletionTimestamp.Time, nil
	}
	return "", status.Active, sc.CreationTimestamp.Time, nil
}
