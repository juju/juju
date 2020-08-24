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
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

type persistentVolume struct {
	corev1.PersistentVolume
}

// NewPersistentVolume creates a new persistent volume resource.
func NewPersistentVolume(name string, in *corev1.PersistentVolume) Resource {
	if in == nil {
		in = &corev1.PersistentVolume{}
	}
	in.SetName(name)
	return &persistentVolume{*in}
}

func (pv *persistentVolume) Clone() Resource {
	clone := *pv
	return &clone
}

func (pv *persistentVolume) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumes()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &pv.PersistentVolume)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, pv.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	pv.PersistentVolume = *res
	return nil
}

func (pv *persistentVolume) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumes()
	res, err := api.Get(ctx, pv.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	pv.PersistentVolume = *res
	return nil
}

func (pv *persistentVolume) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumes()
	err := api.Delete(ctx, pv.Name, metav1.DeleteOptions{
		PropagationPolicy: &k8sconstants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
