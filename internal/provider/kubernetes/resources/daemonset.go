// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// DaemonSet extends the k8s daemonset.
type DaemonSet struct {
	appsv1.DaemonSet
}

// NewDaemonSet creates a new daemonSet resource.
func NewDaemonSet(name string, namespace string, in *appsv1.DaemonSet) *DaemonSet {
	if in == nil {
		in = &appsv1.DaemonSet{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &DaemonSet{*in}
}

// Clone returns a copy of the resource.
func (ds *DaemonSet) Clone() Resource {
	clone := *ds
	return &clone
}

// ID returns a comparable ID for the Resource
func (ds *DaemonSet) ID() ID {
	return ID{"DaemonSet", ds.Name, ds.Namespace}
}

// Apply patches the resource change.
func (ds *DaemonSet) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().DaemonSets(ds.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &ds.DaemonSet)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, ds.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &ds.DaemonSet, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "daemon set %q", ds.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	ds.DaemonSet = *res
	return nil
}

// Get refreshes the resource.
func (ds *DaemonSet) Get(ctx context.Context, client kubernetes.Interface) error {
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

// Delete removes the resource.
func (ds *DaemonSet) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().DaemonSets(ds.Namespace)
	err := api.Delete(ctx, ds.Name, metav1.DeleteOptions{
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
func (ds *DaemonSet) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, client, ds.Namespace, ds.Name, "DaemonSet")
}

// ComputeStatus returns a juju status for the resource.
func (ds *DaemonSet) ComputeStatus(ctx context.Context, client kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if ds.DeletionTimestamp != nil {
		return "", status.Terminated, ds.DeletionTimestamp.Time, nil
	}
	if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
		return "", status.Active, now, nil
	}
	return "", status.Waiting, now, nil
}
