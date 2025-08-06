// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// Role extends the k8s role.
type Role struct {
	rbacv1.Role
}

// NewRole creates a new role resource.
func NewRole(name string, namespace string, in *rbacv1.Role) *Role {
	if in == nil {
		in = &rbacv1.Role{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &Role{*in}
}

// Clone returns a copy of the resource.
func (r *Role) Clone() Resource {
	clone := *r
	return &clone
}

// ID returns a comparable ID for the Resource
func (r *Role) ID() ID {
	return ID{"Role", r.Name, r.Namespace}
}

// Apply patches the resource change.
func (r *Role) Apply(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().Roles(r.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &r.Role)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, r.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &r.Role, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "role %q", r.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	r.Role = *res
	return nil
}

// Get refreshes the resource.
func (r *Role) Get(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().Roles(r.Namespace)
	res, err := api.Get(ctx, r.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	r.Role = *res
	return nil
}

// Delete removes the resource.
func (r *Role) Delete(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().Roles(r.Namespace)
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
func (r *Role) Events(ctx context.Context, coreClient kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, coreClient, r.Namespace, r.Name, "Role")
}

// ComputeStatus returns a juju status for the resource.
func (r *Role) ComputeStatus(_ context.Context, _ kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if r.DeletionTimestamp != nil {
		return "", status.Terminated, r.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}
