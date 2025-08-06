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
	"k8s.io/utils/pointer"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/status"
)

// ClusterRoleBinding extends the k8s cluster role binding.
type ClusterRoleBinding struct {
	rbacv1.ClusterRoleBinding
}

// NewClusterRoleBinding creates a new role resource.
func NewClusterRoleBinding(name string, in *rbacv1.ClusterRoleBinding) *ClusterRoleBinding {
	if in == nil {
		in = &rbacv1.ClusterRoleBinding{}
	}
	in.SetName(name)
	return &ClusterRoleBinding{*in}
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
func (rb *ClusterRoleBinding) Apply(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().ClusterRoleBindings()
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &rb.ClusterRoleBinding)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, rb.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &rb.ClusterRoleBinding, metav1.CreateOptions{
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
func (rb *ClusterRoleBinding) Get(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().ClusterRoleBindings()
	res, err := api.Get(ctx, rb.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	rb.ClusterRoleBinding = *res
	return nil
}

// Delete removes the resource.
func (rb *ClusterRoleBinding) Delete(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().ClusterRoleBindings()
	err := api.Delete(ctx, rb.Name, metav1.DeleteOptions{
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
// existing kubernetes.Interface object with to assert ownership before overwriting it.
func (rb *ClusterRoleBinding) Ensure(
	ctx context.Context,
	coreClient kubernetes.Interface,
	extendedClient clientset.Interface,
	claims ...Claim,
) ([]func(), error) {
	// TODO(caas): roll this into Apply()
	cleanups := []func(){}

	existing := ClusterRoleBinding{rb.ClusterRoleBinding}
	err := existing.Get(ctx, coreClient, extendedClient)
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
			if err := existing.Delete(ctx, coreClient, extendedClient); err != nil {
				return cleanups, errors.Annotatef(
					err,
					"delete cluster role binding %q because roleref has changed",
					existing.Name)
			}
			doUpdate = false
		}
	}

	cleanups = append(cleanups, func() { _ = rb.Delete(ctx, coreClient, extendedClient) })
	if !doUpdate {
		return cleanups, rb.Apply(ctx, coreClient, extendedClient)
	} else if err := rb.Update(ctx, coreClient, extendedClient); err != nil {
		return cleanups, err
	}
	return cleanups, nil
}

// Events emitted by the resource.
func (rb *ClusterRoleBinding) Events(ctx context.Context, coreClient kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, coreClient, rb.Namespace, rb.Name, "ClusterRoleBinding")
}

// ComputeStatus returns a juju status for the resource.
func (rb *ClusterRoleBinding) ComputeStatus(_ context.Context, _ kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if rb.DeletionTimestamp != nil {
		return "", status.Terminated, rb.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// Update updates the object in the kubernetes.Interface cluster to the new representation
func (rb *ClusterRoleBinding) Update(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	out, err := coreClient.RbacV1().ClusterRoleBindings().Update(
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
