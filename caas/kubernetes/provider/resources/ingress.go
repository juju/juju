// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	netv1client "k8s.io/client-go/kubernetes/typed/networking/v1"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// Ingress extends the k8s Ingress.
type Ingress struct {
	client netv1client.IngressInterface
	netv1.Ingress
}

// NewIngress creates a new Ingress resource.
func NewIngress(client netv1client.IngressInterface, name string, in *netv1.Ingress) *Ingress {
	if in == nil {
		in = &netv1.Ingress{}
	}

	in.SetName(name)
	return &Ingress{
		client,
		*in,
	}
}

// Clone returns a copy of the resource.
func (ig *Ingress) Clone() Resource {
	clone := *ig
	return &clone
}

// ID returns a comparable ID for the Resource
func (ig *Ingress) ID() ID {
	return ID{"Ingress", ig.Name, ig.Namespace}
}

// Apply patches the resource change.
func (ig *Ingress) Apply(ctx context.Context) (err error) {
	existing, err := ig.client.Get(ctx, ig.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Create if not found
		created, err := ig.client.Create(ctx, &ig.Ingress, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
		if err != nil {
			return errors.Trace(err)
		}
		ig.Ingress = *created
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// Update if exists (set ResourceVersion to prevent conflict)
	ig.ResourceVersion = existing.ResourceVersion
	updated, err := ig.client.Update(ctx, &ig.Ingress, metav1.UpdateOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "Ingress %q", ig.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}

	ig.Ingress = *updated
	return nil
}

// Get refreshes the resource.
func (ig *Ingress) Get(ctx context.Context) error {
	res, err := ig.client.Get(context.TODO(), ig.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("ingress: %q", ig.Name)
	} else if err != nil {
		return errors.Trace(err)
	}
	ig.Ingress = *res
	return nil
}

// Delete removes the resource.
func (ig *Ingress) Delete(ctx context.Context) error {
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			err := ig.client.Delete(ctx, ig.Name, metav1.DeleteOptions{
				PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
			})
			if k8serrors.IsNotFound(err) {
				return nil
			}
			if k8serrors.IsConflict(err) {
				_ = ig.Get(ctx) // refresh resource version
				return err
			}
			return errors.Annotatef(err, "deleting ingress %q", ig.GetName())
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
func (ig *Ingress) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if ig.DeletionTimestamp != nil {
		return "", status.Terminated, ig.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// ListIngresses returns a list of ingresses.
func ListIngresses(ctx context.Context, client netv1client.IngressInterface, opts metav1.ListOptions) ([]*Ingress, error) {
	var items []*Ingress
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, NewIngress(client, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
