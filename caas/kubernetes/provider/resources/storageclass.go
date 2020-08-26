// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/errors"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
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
func (ss *StorageClass) Clone() Resource {
	clone := *ss
	return &clone
}

// Apply patches the resource change.
func (ss *StorageClass) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.StorageV1().StorageClasses()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ss.StorageClass)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, ss.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	ss.StorageClass = *res
	return nil
}

// Get refreshes the resource.
func (ss *StorageClass) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.StorageV1().StorageClasses()
	res, err := api.Get(ctx, ss.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	ss.StorageClass = *res
	return nil
}

// Delete removes the resource.
func (ss *StorageClass) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.StorageV1().StorageClasses()
	err := api.Delete(ctx, ss.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
