// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	rbacv1client "k8s.io/client-go/kubernetes/typed/rbac/v1"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// Role extends the k8s role.
type Role struct {
	client rbacv1client.RoleInterface
	rbacv1.Role
}

// NewRole creates a new role resource.
func NewRole(client rbacv1client.RoleInterface, namespace string, name string, in *rbacv1.Role) *Role {
	if in == nil {
		in = &rbacv1.Role{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &Role{client, *in}
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
func (r *Role) Apply(ctx context.Context) error {
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &r.Role)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := r.client.Patch(ctx, r.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = r.client.Create(ctx, &r.Role, metav1.CreateOptions{
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
func (r *Role) Get(ctx context.Context) error {
	res, err := r.client.Get(ctx, r.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	r.Role = *res
	return nil
}

// Delete removes the resource.
func (r *Role) Delete(ctx context.Context) error {
	err := r.client.Delete(ctx, r.Name, metav1.DeleteOptions{
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
func (r *Role) ComputeStatus(_ context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if r.DeletionTimestamp != nil {
		return "", status.Terminated, r.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}
