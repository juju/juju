// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admissionclient "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// ValidatingWebhookConfiguration extends the k8s ValidatingWebhookConfiguration.
type ValidatingWebhookConfiguration struct {
	client admissionclient.ValidatingWebhookConfigurationInterface
	admissionv1.ValidatingWebhookConfiguration
}

// NewValidatingWebhookConfig creates a new ValidatingWebhookConfiguration resource.
func NewValidatingWebhookConfig(client admissionclient.ValidatingWebhookConfigurationInterface, name string, in *admissionv1.ValidatingWebhookConfiguration) *ValidatingWebhookConfiguration {
	if in == nil {
		in = &admissionv1.ValidatingWebhookConfiguration{}
	}

	in.SetName(name)
	return &ValidatingWebhookConfiguration{
		client,
		*in,
	}
}

// Clone returns a copy of the resource.
func (v *ValidatingWebhookConfiguration) Clone() Resource {
	clone := *v
	return &clone
}

// ID returns a comparable ID for the Resource
func (v *ValidatingWebhookConfiguration) ID() ID {
	return ID{"ValidatingWebhookConfiguration", v.Name, v.Namespace}
}

// Apply patches the resource change.
func (v *ValidatingWebhookConfiguration) Apply(ctx context.Context) (err error) {
	existing, err := v.client.Get(ctx, v.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Create if not found
		created, err := v.client.Create(ctx, &v.ValidatingWebhookConfiguration, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
		if err != nil {
			return errors.Trace(err)
		}
		v.ValidatingWebhookConfiguration = *created
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// Update if exists (set ResourceVersion to prevent conflict)
	v.ResourceVersion = existing.ResourceVersion
	updated, err := v.client.Update(ctx, &v.ValidatingWebhookConfiguration, metav1.UpdateOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "ValidatingWebhookConfiguration %q", v.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}

	v.ValidatingWebhookConfiguration = *updated
	return nil
}

// Get refreshes the resource.
func (v *ValidatingWebhookConfiguration) Get(ctx context.Context) error {
	res, err := v.client.Get(context.TODO(), v.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("mutating webhook configuration: %q", v.Name)
	} else if err != nil {
		return errors.Trace(err)
	}
	v.ValidatingWebhookConfiguration = *res
	return nil
}

// Delete removes the resource.
func (v *ValidatingWebhookConfiguration) Delete(ctx context.Context) error {
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			err := v.client.Delete(ctx, v.Name, metav1.DeleteOptions{
				PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
			})
			if k8serrors.IsNotFound(err) {
				return nil
			}
			if k8serrors.IsConflict(err) {
				_ = v.Get(ctx) // refresh resource version
				return err
			}
			return errors.Annotatef(err, "deleting validating webhook configuration %q", v.GetName())
		},
		IsFatalError: func(err error) bool {
			return !k8serrors.IsConflict(err)
		},
		Clock:       jujuclock.WallClock,
		Attempts:    5,
		Delay:       time.Second,
		BackoffFunc: retry.ExpBackoff(time.Second, 5*time.Second, 1.5, true),
	})
	return errors.Trace(err)
}

// ComputeStatus returns a juju status for the resource.
func (v *ValidatingWebhookConfiguration) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if v.DeletionTimestamp != nil {
		return "", status.Terminated, v.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// ListValidatingWebhookConfigs returns a list of mutating webhook configs.
func ListValidatingWebhookConfigs(ctx context.Context, client admissionclient.ValidatingWebhookConfigurationInterface, opts metav1.ListOptions) ([]*ValidatingWebhookConfiguration, error) {
	var items []*ValidatingWebhookConfiguration
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, NewValidatingWebhookConfig(client, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
