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

// ID returns a comparable ID for the Resource
func (r *ClusterRole) ID() ID {
	return ID{"ClusterRole", r.Name, r.Namespace}
}

// Apply patches the resource change.
func (r *ClusterRole) Apply(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().ClusterRoles()
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
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "cluster role %q", r.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	r.ClusterRole = *res
	return nil
}

// Get refreshes the resource.
func (r *ClusterRole) Get(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().ClusterRoles()
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
func (r *ClusterRole) Delete(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	api := coreClient.RbacV1().ClusterRoles()
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

// Ensure ensures this cluster role exists in it's desired form inside the
// cluster. If the object does not exist it's updated and if the object exists
// it's updated. The method also takes an optional set of claims to test the
// exisiting kubernetes.Interface object with to assert ownership before overwriting it.
func (r *ClusterRole) Ensure(
	ctx context.Context,
	coreClient kubernetes.Interface,
	extendedClient clientset.Interface,
	claims ...Claim,
) ([]func(), error) {
	// TODO(caas): roll this into Apply()
	cleanups := []func(){}
	hasClaim := true

	existing := ClusterRole{r.ClusterRole}
	err := existing.Get(ctx, coreClient, extendedClient)
	if err == nil {
		hasClaim, err = RunClaims(claims...).Assert(&existing.ClusterRole)
	}
	if err != nil && !errors.IsNotFound(err) {
		return cleanups, errors.Annotatef(
			err,
			"checking for existing cluster role %q",
			existing.Name,
		)
	}

	if !hasClaim {
		return cleanups, errors.AlreadyExistsf(
			"cluster role %q not controlled by juju", r.Name)
	}

	cleanups = append(cleanups, func() { _ = r.Delete(ctx, coreClient, extendedClient) })
	if errors.IsNotFound(err) {
		return cleanups, r.Apply(ctx, coreClient, extendedClient)
	}

	if err := r.Update(ctx, coreClient, extendedClient); err != nil {
		return cleanups, err
	}
	return cleanups, nil
}

// Events emitted by the resource.
func (r *ClusterRole) Events(ctx context.Context, coreClient kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, coreClient, r.Namespace, r.Name, "ClusterRole")
}

// ComputeStatus returns a juju status for the resource.
func (r *ClusterRole) ComputeStatus(_ context.Context, _ kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if r.DeletionTimestamp != nil {
		return "", status.Terminated, r.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// Update updates the object in the kubernetes.Interface cluster to the new representation
func (r *ClusterRole) Update(ctx context.Context, coreClient kubernetes.Interface, extendedClient clientset.Interface) error {
	out, err := coreClient.RbacV1().ClusterRoles().Update(
		ctx,
		&r.ClusterRole,
		metav1.UpdateOptions{
			FieldManager: JujuFieldManager,
		},
	)
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "updating cluster role")
	} else if err != nil {
		return errors.Trace(err)
	}
	r.ClusterRole = *out
	return nil
}
