// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	core "k8s.io/client-go/kubernetes/typed/core/v1"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/core/status"
)

// ConfigMap extends the k8s ConfigMap.
type ConfigMap struct {
	client core.ConfigMapInterface
	corev1.ConfigMap
}

// NewConfigMap creates a new ConfigMap resource.
func NewConfigMap(client core.ConfigMapInterface, name string, in *corev1.ConfigMap) *ConfigMap {
	if in == nil {
		in = &corev1.ConfigMap{}
	}

	in.SetName(name)
	return &ConfigMap{
		client,
		*in,
	}
}

// Clone returns a copy of the resource.
func (crd *ConfigMap) Clone() Resource {
	clone := *crd
	return &clone
}

// ID returns a comparable ID for the Resource
func (crd *ConfigMap) ID() ID {
	return ID{"ConfigMap", crd.Name, crd.Namespace}
}

// Apply patches the resource change.
func (crd *ConfigMap) Apply(ctx context.Context) (err error) {
	existing, err := crd.client.Get(ctx, crd.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Create if not found
		created, err := crd.client.Create(ctx, &crd.ConfigMap, metav1.CreateOptions{
			FieldManager: JujuFieldManager,
		})
		if err != nil {
			return errors.Trace(err)
		}
		crd.ConfigMap = *created
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// Update if exists (set ResourceVersion to prevent conflict)
	crd.ResourceVersion = existing.ResourceVersion
	updated, err := crd.client.Update(ctx, &crd.ConfigMap, metav1.UpdateOptions{
		FieldManager: JujuFieldManager,
	})
	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "ConfigMap %q", crd.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}

	crd.ConfigMap = *updated
	return nil
}

// Get refreshes the resource.
func (crd *ConfigMap) Get(ctx context.Context) error {
	res, err := crd.client.Get(context.TODO(), crd.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("config map: %q", crd.Name)
	} else if err != nil {
		return errors.Trace(err)
	}
	crd.ConfigMap = *res
	return nil
}

// Delete removes the resource.
func (crd *ConfigMap) Delete(ctx context.Context) error {
	err := crd.client.Delete(ctx, crd.Name, metav1.DeleteOptions{
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
func (crd *ConfigMap) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if crd.DeletionTimestamp != nil {
		return "", status.Terminated, crd.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// ListConfigMaps returns a list of configmaps.
func ListConfigMaps(ctx context.Context, client core.ConfigMapInterface, opts metav1.ListOptions) ([]*ConfigMap, error) {
	var items []*ConfigMap
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, NewConfigMap(client, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
