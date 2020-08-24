// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/errors"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

type daemonSet struct {
	appsv1.DaemonSet
}

// NewDaemonSet creates a new daemonSet resource.
func NewDaemonSet(name string, namespace string, in *appsv1.DaemonSet) Resource {
	if in == nil {
		in = &appsv1.DaemonSet{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &daemonSet{*in}
}

func (ds *daemonSet) Clone() Resource {
	clone := *ds
	return &clone
}

func (ds *daemonSet) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().DaemonSets(ds.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ds.DaemonSet)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, ds.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	ds.DaemonSet = *res
	return nil
}

func (ds *daemonSet) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().DaemonSets(ds.Namespace)
	res, err := api.Get(ctx, ds.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	ds.DaemonSet = *res
	return nil
}

func (ds *daemonSet) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().DaemonSets(ds.Namespace)
	err := api.Delete(ctx, ds.Name, metav1.DeleteOptions{
		PropagationPolicy: &k8sconstants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
