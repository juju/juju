// Copyright 2020 Canonical Ltd.
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

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// Secret extends the k8s secret.
type Secret struct {
	corev1.Secret
}

// NewSecret creates a new secret resource.
func NewSecret(name string, namespace string, in *corev1.Secret) *Secret {
	if in == nil {
		in = &corev1.Secret{}
	}
	in.SetName(name)
	in.SetNamespace(namespace)
	return &Secret{*in}
}

// ListSecrets returns a list of Secrets.
func ListSecrets(ctx context.Context, client kubernetes.Interface, namespace string, opts metav1.ListOptions) ([]Secret, error) {
	api := client.CoreV1().Secrets(namespace)
	var items []Secret
	for {
		res, err := api.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, v := range res.Items {
			items = append(items, Secret{Secret: v})
		}
		if res.RemainingItemCount == nil || *res.RemainingItemCount == 0 {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}

// Clone returns a copy of the resource.
func (s *Secret) Clone() Resource {
	clone := *s
	return &clone
}

// ID returns a comparable ID for the Resource
func (s *Secret) ID() ID {
	return ID{"Secret", s.Name, s.Namespace}
}

// Apply patches the resource change.
func (s *Secret) Apply(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Secrets(s.Namespace)
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &s.Secret)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := api.Patch(ctx, s.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsNotFound(err) {
		res, err = api.Create(ctx, &s.Secret, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
	}
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "secret %q", s.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}
	s.Secret = *res
	return nil
}

// Get refreshes the resource.
func (s *Secret) Get(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Secrets(s.Namespace)
	res, err := api.Get(ctx, s.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s")
	} else if err != nil {
		return errors.Trace(err)
	}
	s.Secret = *res
	return nil
}

// Delete removes the resource.
func (s *Secret) Delete(ctx context.Context, client kubernetes.Interface) error {
	api := client.CoreV1().Secrets(s.Namespace)
	err := api.Delete(ctx, s.Name, metav1.DeleteOptions{
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
func (s *Secret) Events(ctx context.Context, client kubernetes.Interface) ([]corev1.Event, error) {
	return ListEventsForObject(ctx, client, s.Namespace, s.Name, "Secret")
}

// ComputeStatus returns a juju status for the resource.
func (s *Secret) ComputeStatus(ctx context.Context, client kubernetes.Interface, now time.Time) (string, status.Status, time.Time, error) {
	if s.DeletionTimestamp != nil {
		return "", status.Terminated, s.DeletionTimestamp.Time, nil
	}
	return "", status.Active, s.CreationTimestamp.Time, nil
}
