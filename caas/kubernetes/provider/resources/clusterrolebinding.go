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
	"k8s.io/utils/pointer"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/status"
)

// ClusterRoleBinding extends the k8s cluster role binding.
type ClusterRoleBinding struct {
	client rbacv1client.ClusterRoleBindingInterface
	rbacv1.ClusterRoleBinding
}

// NewClusterRoleBinding creates a new role resource.
func NewClusterRoleBinding(client rbacv1client.ClusterRoleBindingInterface, name string, in *rbacv1.ClusterRoleBinding) *ClusterRoleBinding {
	if in == nil {
		in = &rbacv1.ClusterRoleBinding{}
	}
	in.SetName(name)
	return &ClusterRoleBinding{client, *in}
}

// Clone returns a copy of the resource.
func (rb *ClusterRoleBinding) Clone() Resource {
	clone := *rb
	return &clone
}

// ID returns a comparable ID for the Resource
func (rb *ClusterRoleBinding) ID() ID {
	return ID{"ClusterRoleBinding", rb.Name, rb.Namespace}
}

// Apply patches the resource change.
func (rb *ClusterRoleBinding) Apply(ctx context.Context) error {
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &rb.ClusterRoleBinding)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := rb.client.Patch(ctx, rb.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = rb.client.Create(ctx, &rb.ClusterRoleBinding, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "cluster role binding %q", rb.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	rb.ClusterRoleBinding = *res
	return nil
}

// Get refreshes the resource.
func (rb *ClusterRoleBinding) Get(ctx context.Context) error {
	res, err := rb.client.Get(ctx, rb.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	rb.ClusterRoleBinding = *res
	return nil
}

// Delete removes the resource.
func (rb *ClusterRoleBinding) Delete(ctx context.Context) error {
	err := rb.client.Delete(ctx, rb.Name, metav1.DeleteOptions{
		PropagationPolicy:  k8sconstants.DeletePropagationBackground(),
		GracePeriodSeconds: pointer.Int64Ptr(0),
		Preconditions:      utils.NewUIDPreconditions(rb.UID),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// shouldDelete checks if there are any changes in the immutable field to decide
// if the existing cluster role binding should be deleted or not.
func shouldDelete(existing, new rbacv1.ClusterRoleBinding) bool {
	return existing.RoleRef.APIGroup != new.RoleRef.APIGroup ||
		existing.RoleRef.Kind != new.RoleRef.Kind ||
		existing.RoleRef.Name != new.RoleRef.Name
}

// Ensure ensures this cluster role exists in it's desired form inside the
// cluster. If the object does not exist it's updated and if the object exists
// it's updated. The method also takes an optional set of claims to test the
// existing Kubernetes object with to assert ownership before overwriting it.
func (rb *ClusterRoleBinding) Ensure(
	ctx context.Context,
	claims ...Claim,
) ([]func(), error) {
	// TODO(caas): roll this into Apply()
	cleanups := []func(){}

	existing := ClusterRoleBinding{rb.client, rb.ClusterRoleBinding}
	err := existing.Get(ctx)
	if err != nil && !errors.IsNotFound(err) {
		return cleanups, errors.Annotatef(err, "getting existing cluster role binding %q", rb.Name)
	}
	doUpdate := err == nil
	if err == nil {
		hasClaim, err := RunClaims(claims...).Assert(&existing.ClusterRoleBinding)
		if err != nil {
			return cleanups, errors.Annotatef(err, "checking for existing cluster role binding %q", rb.Name)
		}
		if !hasClaim {
			return cleanups, errors.AlreadyExistsf(
				"cluster role binding %q not controlled by juju", rb.Name)
		}
		if shouldDelete(existing.ClusterRoleBinding, rb.ClusterRoleBinding) {
			// RoleRef is immutable, delete the cluster role binding then re-create it.
			if err := existing.Delete(ctx); err != nil {
				return cleanups, errors.Annotatef(
					err,
					"delete cluster role binding %q because roleref has changed",
					existing.Name)
			}
			doUpdate = false
		}
	}

	cleanups = append(cleanups, func() { _ = rb.Delete(ctx) })
	if !doUpdate {
		return cleanups, rb.Apply(ctx)
	} else if err := rb.Update(ctx); err != nil {
		return cleanups, err
	}
	return cleanups, nil
}

// ComputeStatus returns a juju status for the resource.
func (rb *ClusterRoleBinding) ComputeStatus(_ context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if rb.DeletionTimestamp != nil {
		return "", status.Terminated, rb.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// Update updates the object in the Kubernetes cluster to the new representation
func (rb *ClusterRoleBinding) Update(ctx context.Context) error {
	out, err := rb.client.Update(
		ctx,
		&rb.ClusterRoleBinding,
		metav1.UpdateOptions{
			FieldManager: JujuFieldManager,
		},
	)
	if k8serrors.IsNotFound(err) {
		return errors.Annotatef(err, "updating cluster role binding %q", rb.Name)
	} else if err != nil {
		return errors.Trace(err)
	}
	rb.ClusterRoleBinding = *out
	return nil
}
