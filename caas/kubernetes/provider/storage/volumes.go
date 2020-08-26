// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
)

// VolumeParams holds PV and PVC related config.
type VolumeParams struct {
	Name          string
	StorageConfig *StorageConfig
	Size          resource.Quantity
	AccessMode    corev1.PersistentVolumeAccessMode
}

// ParseVolumeParams returns a volume param.
func ParseVolumeParams(name string, size resource.Quantity, storageAttr map[string]interface{}) (*VolumeParams, error) {
	storageConfig, err := ParseStorageConfig(storageAttr)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid storage configuration for %v", name)
	}
	accessMode, err := ParseStorageMode(storageAttr)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid storage mode for %v", name)
	}
	return &VolumeParams{
		Name:          name,
		Size:          size,
		StorageConfig: storageConfig,
		AccessMode:    *accessMode,
	}, nil
}

var storageConfigFields = schema.Fields{
	k8sconstants.StorageClass:       schema.String(),
	k8sconstants.StorageProvisioner: schema.String(),
}

var storageConfigChecker = schema.FieldMap(
	storageConfigFields,
	schema.Defaults{
		k8sconstants.StorageClass:       schema.Omit,
		k8sconstants.StorageProvisioner: schema.Omit,
	},
)

// StorageConfig defines config for storage.
type StorageConfig struct {
	// StorageClass defines a storage class
	// which will be created with the specified
	// provisioner and parameters if it doesn't
	// exist.
	StorageClass string

	// StorageProvisioner is the provisioner class to use.
	StorageProvisioner string

	// Parameters define attributes of the storage class.
	Parameters map[string]string

	// ReclaimPolicy defines the volume reclaim policy.
	ReclaimPolicy corev1.PersistentVolumeReclaimPolicy
}

const (
	storageConfigParameterPrefix = "parameters."
)

// ParseStorageConfig returns storage config.
func ParseStorageConfig(attrs map[string]interface{}) (*StorageConfig, error) {
	out, err := storageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating storage config")
	}
	coerced := out.(map[string]interface{})
	storageConfig := &StorageConfig{}
	if storageClassName, ok := coerced[k8sconstants.StorageClass].(string); ok {
		storageConfig.StorageClass = storageClassName
	}
	if storageProvisioner, ok := coerced[k8sconstants.StorageProvisioner].(string); ok {
		storageConfig.StorageProvisioner = storageProvisioner
	}
	if storageConfig.StorageProvisioner != "" && storageConfig.StorageClass == "" {
		return nil, errors.New("storage-class must be specified if storage-provisioner is specified")
	}
	// By default, we'll retain volumes used for charm storage.
	storageConfig.ReclaimPolicy = corev1.PersistentVolumeReclaimRetain
	storageConfig.Parameters = make(map[string]string)
	for k, v := range attrs {
		if !strings.HasPrefix(k, storageConfigParameterPrefix) {
			continue
		}
		k = strings.TrimPrefix(k, storageConfigParameterPrefix)
		storageConfig.Parameters[k] = fmt.Sprintf("%v", v)
	}
	return storageConfig, nil
}

var storageModeFields = schema.Fields{
	k8sconstants.StorageMode: schema.String(),
}

var storageModeChecker = schema.FieldMap(
	storageModeFields,
	schema.Defaults{
		k8sconstants.StorageMode: "ReadWriteOnce",
	},
)

// ParseStorageMode returns k8s persistent volume access mode.
func ParseStorageMode(attrs map[string]interface{}) (*corev1.PersistentVolumeAccessMode, error) {
	parseMode := func(m string) (*corev1.PersistentVolumeAccessMode, error) {
		var out corev1.PersistentVolumeAccessMode
		switch m {
		case "ReadOnlyMany", "ROX":
			out = corev1.ReadOnlyMany
		case "ReadWriteMany", "RWX":
			out = corev1.ReadWriteMany
		case "ReadWriteOnce", "RWO":
			out = corev1.ReadWriteOnce
		default:
			return nil, errors.NotSupportedf("storage mode %q", m)
		}
		return &out, nil
	}

	out, err := storageModeChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating storage mode")
	}
	coerced := out.(map[string]interface{})
	return parseMode(coerced[k8sconstants.StorageMode].(string))
}
