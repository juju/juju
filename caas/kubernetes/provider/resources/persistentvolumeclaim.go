// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// PersistentVolumeClaim extends the k8s persistentVolumeClaim.
type PersistentVolumeClaim struct {
	corev1.PersistentVolumeClaim
}

// NewPersistentVolumeClaim creates a new persistent volume claim resource.
func NewPersistentVolumeClaim(name string, namespace string, in *corev1.PersistentVolumeClaim) *PersistentVolumeClaim {
	if in == nil {
		in = &corev1.PersistentVolumeClaim{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &PersistentVolumeClaim{*in}
}

// ListPersistentVolumeClaims returns a list of persistent volume claims.
func ListPersistentVolumeClaims(ctx context.Context, coreClient kubernetes.Interface, namespace string, opts metav1.ListOptions) ([]PersistentVolumeClaim, error) {
	api := coreClient.CoreV1().PersistentVolumeClaims(namespace)
	var items []PersistentVolumeClaim
	for {
		res, err := api.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, PersistentVolumeClaim{PersistentVolumeClaim: item})
		}
		if res.RemainingItemCount == nil || *res.RemainingItemCount == 0 {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}

// Clone returns a copy of the resource.
func (pvc *PersistentVolumeClaim) Clone() Resource {
	clone := *pvc
	return &clone
}

// ID returns a comparable ID for the Resource
func (pvc *PersistentVolumeClaim) ID() ID {
	return ID{"PersistentVolumeClaim", pvc.Name, pvc.Namespace}
}

// Apply patches the resource change.
func (pvc *PersistentVolumeClaim) Apply(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &pvc.PersistentVolumeClaim)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, pvc.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &pvc.PersistentVolumeClaim, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "persistent volume claim %q", pvc.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	pvc.PersistentVolumeClaim = *res
	return nil
}

// Get refreshes the resource.
func (pvc *PersistentVolumeClaim) Get(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	res, err := api.Get(ctx, pvc.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	pvc.PersistentVolumeClaim = *res
	return nil
}

// Delete removes the resource.
func (pvc *PersistentVolumeClaim) Delete(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.CoreV1().PersistentVolumeClaims(pvc.Namespace)
	logger.Infof("deleting PVC %s due to call to PersistentVolumeClaim.Delete", pvc.Name)
	err := api.Delete(ctx, pvc.Name, metav1.DeleteOptions{
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
func (pvc *PersistentVolumeClaim) Events(ctx context.Context, coreClient kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, coreClient, pvc.Namespace, pvc.Name, "PersistentVolumeClaim")
}

// ComputeStatus returns a juju status for the resource.
func (pvc *PersistentVolumeClaim) ComputeStatus(ctx context.Context, coreClient kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if pvc.DeletionTimestamp != nil {
		return "", status.Terminated, pvc.DeletionTimestamp.Time, nil
	}
	if pvc.Status.Phase == corev1.ClaimBound {
		return string(pvc.Status.Phase), status.Active, now, nil
	}
	return string(pvc.Status.Phase), status.Waiting, now, nil
}
