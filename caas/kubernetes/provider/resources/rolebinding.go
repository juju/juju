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

// RoleBinding extends the k8s role binding.
type RoleBinding struct {
	rbacv1.RoleBinding
}

// NewRoleBinding creates a new role resource.
func NewRoleBinding(name string, namespace string, in *rbacv1.RoleBinding) *RoleBinding {
	if in == nil {
		in = &rbacv1.RoleBinding{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &RoleBinding{*in}
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
func (rb *RoleBinding) Apply(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().RoleBindings(rb.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &rb.RoleBinding)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, rb.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &rb.RoleBinding, metav1.CreateOptions{
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
func (rb *RoleBinding) Get(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().RoleBindings(rb.Namespace)
	res, err := api.Get(ctx, rb.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	rb.RoleBinding = *res
	return nil
}

// Delete removes the resource.
func (rb *RoleBinding) Delete(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().RoleBindings(rb.Namespace)
	err := api.Delete(ctx, rb.Name, metav1.DeleteOptions{
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
func (rb *RoleBinding) Events(ctx context.Context, coreClient kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, coreClient, rb.Namespace, rb.Name, "RoleBinding")
}

// ComputeStatus returns a juju status for the resource.
func (rb *RoleBinding) ComputeStatus(_ context.Context, _ kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if rb.DeletionTimestamp != nil {
		return "", status.Terminated, rb.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}
