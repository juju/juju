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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/core/status"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
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
func (cm *ConfigMap) Clone() Resource {
	clone := *cm
	return &clone
}

// ID returns a comparable ID for the Resource.
func (cm *ConfigMap) ID() ID {
	return ID{"ConfigMap", cm.Name, cm.Namespace}
}

// Apply patches the resource change.
func (cm *ConfigMap) Apply(ctx context.Context) (err error) {
	// Attempt to create first, then patch if it already exists.
	created, err := cm.client.Create(ctx, &cm.ConfigMap, metav1.CreateOptions{
		FieldManager: JujuFieldManager,
	})
	if err == nil {
		cm.ConfigMap = *created
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return errors.Annotatef(err, "creating ConfigMap %q", cm.GetName())
	}

	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &cm.ConfigMap)
	if err != nil {
		return errors.Trace(err)
	}
	res, err := cm.client.Patch(ctx, cm.Name, types.StrategicMergePatchType, data, metav1.PatchOptions{
		FieldManager: JujuFieldManager,
	})

	if k8serrors.IsConflict(err) {
		return errors.Annotatef(errConflict, "ConfigMap %q", cm.Name)
	}
	if err != nil {
		return errors.Trace(err)
	}

	cm.ConfigMap = *res
	return nil
}

// Get refreshes the resource.
func (cm *ConfigMap) Get(ctx context.Context) error {
	res, err := cm.client.Get(ctx, cm.Name, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("ConfigMap: %q", cm.Name)
	} else if err != nil {
		return errors.Trace(err)
	}
	cm.ConfigMap = *res
	return nil
}

// Delete removes the resource.
func (cm *ConfigMap) Delete(ctx context.Context) error {
	err := cm.client.Delete(ctx, cm.Name, metav1.DeleteOptions{
		PropagationPolicy: k8sconstants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return errors.NewNotFound(err, "k8s config map for deletion")
	}
	return errors.Trace(err)
}

// ComputeStatus returns a juju status for the resource.
func (cm *ConfigMap) ComputeStatus(ctx context.Context, now time.Time) (string, status.Status, time.Time, error) {
	if cm.DeletionTimestamp != nil {
		return "", status.Terminated, cm.DeletionTimestamp.Time, nil
	}
	return "", status.Active, now, nil
}

// ListConfigMaps returns a list of configmaps.
func ListConfigMaps(ctx context.Context, client core.ConfigMapInterface, opts metav1.ListOptions) ([]ConfigMap, error) {
	var items []ConfigMap
	for {
		res, err := client.List(ctx, opts)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, item := range res.Items {
			items = append(items, *NewConfigMap(client, item.Name, &item))
		}
		if res.Continue == "" {
			break
		}
		opts.Continue = res.Continue
	}
	return items, nil
}
