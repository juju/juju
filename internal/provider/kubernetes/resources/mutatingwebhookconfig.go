// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	admissionclient "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// MutatingWebhookConfiguration extends the k8s MutatingWebhookConfiguration.
type MutatingWebhookConfiguration struct {
	client admissionclient.MutatingWebhookConfigurationInterface
	admissionv1.MutatingWebhookConfiguration
}

// NewMutatingWebhookConfig creates a new MutatingWebhookConfiguration resource.
func NewMutatingWebhookConfig(client admissionclient.MutatingWebhookConfigurationInterface, name string, in *admissionv1.MutatingWebhookConfiguration) *MutatingWebhookConfiguration {
	if in == nil {
		in = &admissionv1.MutatingWebhookConfiguration{}
	}

	in.SetName(name)
	return &MutatingWebhookConfiguration{
		client,
		*in,
	}
}

// Clone returns a copy of the resource.
func (m *MutatingWebhookConfiguration) Clone() Resource {
	clone := *m
	return &clone
}

// ID returns a comparable ID for the Resource.
func (m *MutatingWebhookConfiguration) ID() ID {
	return ID{"MutatingWebhookConfiguration", m.Name, m.Namespace}
}

// Apply patches the resource change.
func (m *MutatingWebhookConfiguration) Apply(ctx context.Context) (err error) {
	// Attempt to create first, then patch if it already exists.
	created, err := m.client.Create(ctx, &m.MutatingWebhookConfiguration, metav1.CreateOptions{
		FieldManager: JujuFieldManager,
	})
	if err == nil {
		m.MutatingWebhookConfiguration = *created
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return errors.Annotatef(err, "creating MutatingWebhookConfiguration %q", m.GetName())
	}

	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &m.MutatingWebhookConfiguration)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := m.client.Patch(ctx, m.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})

	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "MutatingWebhookConfiguration %q", m.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}

	m.MutatingWebhookConfiguration = *res
	return nil
}

// Get refreshes the resource.
func (m *MutatingWebhookConfiguration) Get(ctx context.Context) error {
	res, err := m.client.Get(ctx, m.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("MutatingWebhookConfiguration: %q", m.Name)
	} else if err != nil {
		return errors.Trace(err)
	}
	m.MutatingWebhookConfiguration = *res
	return nil
}

// Delete removes the resource.
func (m *MutatingWebhookConfiguration) Delete(ctx context.Context) error {
	err := m.client.Delete(ctx, m.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s mutating web hook config for deletion")
	}
	return errors.Trace(err)
}

// ComputeStatus returns a juju status for the resource.
func (m *MutatingWebhookConfiguration) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if m.DeletionTimestamp != nil {
		return "", status.Terminated, m.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// ListMutatingWebhookConfigs returns a list of mutating webhook configs.
func ListMutatingWebhookConfigs(ctx context.Context, client admissionclient.MutatingWebhookConfigurationInterface, opts metav1.ListOptions) ([]MutatingWebhookConfiguration, error) {
	var items []MutatingWebhookConfiguration
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, *NewMutatingWebhookConfig(client, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
