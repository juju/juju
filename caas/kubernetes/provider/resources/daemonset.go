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

	"github.com/juju/juju/caas/kubernetes/provider/constants"
)

type DaemonSet struct {
	appsv1.DaemonSet
}

func NewDaemonSet(name string, namespace string) *DaemonSet {
	return &DaemonSet{
		DaemonSet: appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func (ss *DaemonSet) Clone() Resource {
	clone := *ss
	return &clone
}

func (ss *DaemonSet) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().DaemonSets(ss.Namespace)
	ss.TypeMeta.Kind = "DaemonSet"
	ss.TypeMeta.APIVersion = appsv1.SchemeGroupVersion.String()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ss.DaemonSet)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, ss.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	ss.DaemonSet = *res
	return nil
}

func (ss *DaemonSet) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().DaemonSets(ss.Namespace)
	res, err := api.Get(ctx, ss.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	ss.DaemonSet = *res
	return nil
}

func (ss *DaemonSet) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().DaemonSets(ss.Namespace)
	err := api.Delete(ctx, ss.Name, metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
