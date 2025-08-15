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

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// RoleBinding extends the k8s role binding.
type RoleBinding struct {
	client rbacv1client.RoleBindingInterface
	rbacv1.RoleBinding
}

// NewRoleBinding creates a new role resource.
func NewRoleBinding(client rbacv1client.RoleBindingInterface, namespace string, name string, in *rbacv1.RoleBinding) *RoleBinding {
	if in == nil {
		in = &rbacv1.RoleBinding{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &RoleBinding{client, *in}
}

// Clone returns a copy of the resource.
func (rb *RoleBinding) Clone() Resource {
	clone := *rb
	return &clone
}

// ID returns a comparable ID for the Resource
func (rb *RoleBinding) ID() ID {
	return ID{"RoleBinding", rb.Name, rb.Namespace}
}

// Apply patches the resource change.
func (rb *RoleBinding) Apply(ctx context.Context) error {
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &rb.RoleBinding)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := rb.client.Patch(ctx, rb.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = rb.client.Create(ctx, &rb.RoleBinding, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "role binding %q", rb.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	rb.RoleBinding = *res
	return nil
}

// Get refreshes the resource.
func (rb *RoleBinding) Get(ctx context.Context) error {
	res, err := rb.client.Get(ctx, rb.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	rb.RoleBinding = *res
	return nil
}

// Delete removes the resource.
func (rb *RoleBinding) Delete(ctx context.Context) error {
	err := rb.client.Delete(ctx, rb.Name, metav1.DeleteOptions{
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
func (rb *RoleBinding) ComputeStatus(_ context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if rb.DeletionTimestamp != nil {
		return "", status.Terminated, rb.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}
