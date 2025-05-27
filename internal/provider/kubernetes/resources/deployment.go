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

// Deployment extends the k8s deployment.
type Deployment struct {
	appsv1.Deployment
}

// NewDeployment creates a new deployment resource.
func NewDeployment(name string, namespace string, in *appsv1.Deployment) *Deployment {
	if in == nil {
		in = &appsv1.Deployment{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &Deployment{*in}
}

// Clone returns a copy of the resource.
func (d *Deployment) Clone() Resource {
	clone := *d
	return &clone
}

// ID returns a comparable ID for the Resource
func (d *Deployment) ID() ID {
	return ID{"Deployment", d.Name, d.Namespace}
}

// Apply patches the resource change.
func (d *Deployment) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().Deployments(d.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &d.Deployment)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, d.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &d.Deployment, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "deployment %q", d.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	d.Deployment = *res
	return nil
}

// Get refreshes the resource.
func (d *Deployment) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().Deployments(d.Namespace)
	res, err := api.Get(ctx, d.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	d.Deployment = *res
	return nil
}

// Delete removes the resource.
func (d *Deployment) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.AppsV1().Deployments(d.Namespace)
	err := api.Delete(ctx, d.Name, metav1.DeleteOptions{
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
func (d *Deployment) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, client, d.Namespace, d.Name, "Deployment")
}

// ComputeStatus returns a juju status for the resource.
func (d *Deployment) ComputeStatus(ctx context.Context, client kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if d.DeletionTimestamp != nil {
		return "", status.Terminated, d.DeletionTimestamp.Time, nil
	}
	if d.Status.ReadyReplicas == d.Status.Replicas {
		return "", status.Active, now, nil
	}
	return "", status.Waiting, now, nil
}
