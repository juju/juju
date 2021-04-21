// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// ServiceAccount extends the k8s service account.
type ServiceAccount struct {
	corev1.ServiceAccount
}

// NewServiceAccount creates a new service account resource.
func NewServiceAccount(name string, namespace string, in *corev1.ServiceAccount) *ServiceAccount {
	if in == nil {
		in = &corev1.ServiceAccount{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &ServiceAccount{*in}
}

// Clone returns a copy of the resource.
func (sa *ServiceAccount) Clone() Resource {
	clone := *sa
	return &clone
}

// Apply patches the resource change.
func (sa *ServiceAccount) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().ServiceAccounts(sa.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &sa.ServiceAccount)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, sa.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &sa.ServiceAccount, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if err != nil {
		return errors.Trace(err)
	}
	sa.ServiceAccount = *res
	return nil
}

// Get refreshes the resource.
func (sa *ServiceAccount) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().ServiceAccounts(sa.Namespace)
	res, err := api.Get(ctx, sa.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	sa.ServiceAccount = *res
	return nil
}

// Delete removes the resource.
func (sa *ServiceAccount) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().ServiceAccounts(sa.Namespace)
	err := api.Delete(ctx, sa.Name, metav1.DeleteOptions{
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
func (sa *ServiceAccount) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, client, sa.Namespace, sa.Name, "ServiceAccount")
}

// ComputeStatus returns a juju status for the resource.
func (sa *ServiceAccount) ComputeStatus(_ context.Context, _ kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if sa.DeletionTimestamp != nil {
		return "", status.Terminated, sa.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}
