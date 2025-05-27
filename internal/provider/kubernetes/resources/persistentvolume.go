// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
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

// PersistentVolume extends the k8s persistentVolume.
type PersistentVolume struct {
	corev1.PersistentVolume
}

// NewPersistentVolume creates a new persistent volume resource.
func NewPersistentVolume(name string, in *corev1.PersistentVolume) *PersistentVolume {
	if in == nil {
		in = &corev1.PersistentVolume{}
	}
	in.SetName(name)
	return &PersistentVolume{*in}
}

// Clone returns a copy of the resource.
func (pv *PersistentVolume) Clone() Resource {
	clone := *pv
	return &clone
}

// ID returns a comparable ID for the Resource
func (pv *PersistentVolume) ID() ID {
	return ID{"PersistentVolume", pv.Name, pv.Namespace}
}

// Apply patches the resource change.
func (pv *PersistentVolume) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumes()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &pv.PersistentVolume)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, pv.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &pv.PersistentVolume, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "persistent volume %q", pv.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	pv.PersistentVolume = *res
	return nil
}

// Get refreshes the resource.
func (pv *PersistentVolume) Get(ctx context.Context, client kubernetes.Interface) error {
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

// Delete removes the resource.
func (pv *PersistentVolume) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().PersistentVolumes()
	logger.Infof(context.TODO(), "deleting PV %s due to call to PersistentVolume.Delete", pv.Name)
	err := api.Delete(ctx, pv.Name, metav1.DeleteOptions{
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
func (pv *PersistentVolume) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, client, pv.Namespace, pv.Name, "PersistentVolume")
}

// ComputeStatus returns a juju status for the resource.
func (pv *PersistentVolume) ComputeStatus(ctx context.Context, client kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if pv.DeletionTimestamp != nil {
		return "", status.Terminated, pv.DeletionTimestamp.Time, nil
	}
	if pv.Status.Phase == corev1.VolumeBound {
		return string(pv.Status.Phase), status.Active, now, nil
	}
	if pv.Status.Phase == corev1.VolumeAvailable {
		return string(pv.Status.Phase), status.Active, now, nil
	}
	return string(pv.Status.Phase), status.Waiting, now, nil
}
