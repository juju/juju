// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package k8sresources

import (
	"context"

	"github.com/juju/errors"
	"github.com/kr/pretty"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

type StorageClass struct {
	storagev1.StorageClass
}

func NewStorageClass(name string) *StorageClass {
	return &StorageClass{
		StorageClass: storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

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

func (ss *StorageClass) Clone() Resource {
	clone := *ss
	return &clone
}

func (ss *StorageClass) Apply(ctx context.Context, client kubernetes.Interface) error {
	logger.Errorf("StorageClass.Apply %s", pretty.Sprint(ss))
	api := client.StorageV1().StorageClasses()
	ss.TypeMeta.Kind = "StorageClass"
	ss.TypeMeta.APIVersion = storagev1.SchemeGroupVersion.String()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ss.StorageClass)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, ss.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	ss.StorageClass = *res
	return nil
}

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
