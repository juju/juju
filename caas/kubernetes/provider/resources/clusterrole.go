// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// ClusterRole extends the k8s cluster role.
type ClusterRole struct {
	rbacv1.ClusterRole
}

// NewClusterRole creates a new cluster role resource.
func NewClusterRole(name string, in *rbacv1.ClusterRole) *ClusterRole {
	if in == nil {
		in = &rbacv1.ClusterRole{}
	}
	in.SetName(name)
	return &ClusterRole{*in}
}

// Clone returns a copy of the resource.
func (r *ClusterRole) Clone() Resource {
	clone := *r
	return &clone
}

// Apply patches the resource change.
func (r *ClusterRole) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.RbacV1().ClusterRoles()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &r.ClusterRole)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, r.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &r.ClusterRole, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if err != nil {
		return errors.Trace(err)
	}
	r.ClusterRole = *res
	return nil
}

// Get refreshes the resource.
func (r *ClusterRole) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.RbacV1().ClusterRoles()
	res, err := api.Get(ctx, r.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	r.ClusterRole = *res
	return nil
}

// Delete removes the resource.
func (r *ClusterRole) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.RbacV1().ClusterRoles()
	err := api.Delete(ctx, r.Name, metav1.DeleteOptions{
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
func (r *ClusterRole) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, client, r.Namespace, r.Name, "ClusterRole")
}

// ComputeStatus returns a juju status for the resource.
func (r *ClusterRole) ComputeStatus(_ context.Context, _ kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if r.DeletionTimestamp != nil {
		return "", status.Terminated, r.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}
