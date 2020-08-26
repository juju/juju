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

// StatefulSet extends the k8s statefulSet.
type StatefulSet struct {
	appsv1.StatefulSet
}

// NewStatefulSet creates a new statefulset resource.
func NewStatefulSet(name string, namespace string, in *appsv1.StatefulSet) *StatefulSet {
	if in == nil {
		in = &appsv1.StatefulSet{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &StatefulSet{*in}
}

// Clone returns a copy of the resource.
func (ss *StatefulSet) Clone() Resource {
	clone := *ss
	return &clone
}

// Apply patches the resource change.
func (ss *StatefulSet) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().StatefulSets(ss.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ss.StatefulSet)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, ss.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if err != nil {
		return errors.Trace(err)
	}
	ss.StatefulSet = *res
	return nil
}

// Get refreshes the resource.
func (ss *StatefulSet) Get(ctx context.Context, client kubernetes.Interface) error {
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

// Delete removes the resource.
func (ss *StatefulSet) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().StatefulSets(ss.Namespace)
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
