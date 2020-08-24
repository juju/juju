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

type statefulSet struct {
	appsv1.StatefulSet
}

// NewStatefulSet creates a new statefulset resource.
func NewStatefulSet(name string, namespace string, in *appsv1.StatefulSet) Resource {
	if in == nil {
		in = &appsv1.StatefulSet{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &statefulSet{*in}
}

func (ss *statefulSet) Clone() Resource {
	clone := *ss
	return &clone
}

func (ss *statefulSet) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().StatefulSets(ss.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ss.StatefulSet)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, ss.Name, types.ApplyPatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	ss.StatefulSet = *res
	return nil
}

func (ss *statefulSet) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().StatefulSets(ss.Namespace)
	res, err := api.Get(ctx, ss.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	ss.StatefulSet = *res
	return nil
}

func (ss *statefulSet) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().StatefulSets(ss.Namespace)
	err := api.Delete(ctx, ss.Name, metav1.DeleteOptions{
		PropagationPolicy: &k8sconstants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}
