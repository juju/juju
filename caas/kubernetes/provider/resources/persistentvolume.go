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
	types "k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// PersistentVolume extends the k8s persistentVolume.
type PersistentVolume struct {
	client v1.PersistentVolumeInterface
	corev1.PersistentVolume
}

// NewPersistentVolume creates a new persistent volume resource.
func NewPersistentVolume(client v1.PersistentVolumeInterface, name string, in *corev1.PersistentVolume) *PersistentVolume {
	if in == nil {
		in = &corev1.PersistentVolume{}
	}
	in.SetName(name)
	return &PersistentVolume{client, *in}
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
func (pv *PersistentVolume) Apply(ctx context.Context) error {
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &pv.PersistentVolume)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := pv.client.Patch(ctx, pv.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = pv.client.Create(ctx, &pv.PersistentVolume, metav1.CreateOptions{
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
func (pv *PersistentVolume) Get(ctx context.Context) error {
	res, err := pv.client.Get(ctx, pv.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	pv.PersistentVolume = *res
	return nil
}

// Delete removes the resource.
func (pv *PersistentVolume) Delete(ctx context.Context) error {
	logger.Infof("deleting PV %s due to call to PersistentVolume.Delete", pv.Name)
	err := pv.client.Delete(ctx, pv.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ComputeStatus returns a juju status for the resource.
func (pv *PersistentVolume) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
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
