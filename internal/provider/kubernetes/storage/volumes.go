// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/schema"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	internallogger "github.com/juju/juju/internal/logger"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

var logger = internallogger.GetLogger("juju.kubernetes.provider.storage")

// VolumeParams holds PV and PVC related config.
type VolumeParams struct {
	Name          string
	StorageConfig *StorageConfig
	Size          resource.Quantity
	AccessMode    corev1.PersistentVolumeAccessMode
}

// ParseVolumeParams returns a volume param.
func ParseVolumeParams(name string, size resource.Quantity, storageAttr map[string]any) (*VolumeParams, error) {
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
func ParseStorageConfig(attrs map[string]any) (*StorageConfig, error) {
	out, err := storageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating storage config")
	}
	coerced := out.(map[string]any)
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
func ParseStorageMode(attrs map[string]any) (*corev1.PersistentVolumeAccessMode, error) {
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
	coerced := out.(map[string]any)
	return parseMode(coerced[k8sconstants.StorageMode].(string))
}

// PushUniqueVolume ensures to only add unique volumes because k8s will not schedule pods if it has duplicated volumes.
// The existing volume will be replaced if force sets to true.
func PushUniqueVolume(podSpec *corev1.PodSpec, vol corev1.Volume, force bool) error {
	for i, v := range podSpec.Volumes {
		if v.Name != vol.Name {
			continue
		}
		if reflect.DeepEqual(v, vol) {
			return nil
		}
		if force {
			podSpec.Volumes[i] = vol
			return nil
		}
		return errors.NotValidf("duplicated volume %q", vol.Name)
	}
	podSpec.Volumes = append(podSpec.Volumes, vol)
	return nil
}

// PushUniqueVolumeMount ensures to only add unique volume mount to a container.
func PushUniqueVolumeMount(container *corev1.Container, volMount corev1.VolumeMount) {
	for _, v := range container.VolumeMounts {
		if reflect.DeepEqual(v, volMount) {
			return
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, volMount)
}

// PushUniqueVolumeClaimTemplate ensures to only add unique volume claim template to a statefulset.
func PushUniqueVolumeClaimTemplate(existing *[]corev1.PersistentVolumeClaim, pvc corev1.PersistentVolumeClaim) error {
	for _, v := range *existing {
		if v.Name == pvc.Name {
			// Let's reuse the existing PVC. This is a valid scenario because
			// a workload and charm container may share the same PVC but with
			// different mount points in their respective containers.
			if isSamePVC(v, pvc) {
				return nil
			}
			// PVC name has to be unique.
			return errors.NotValidf("duplicated PVC %q", pvc.Name)
		}
	}
	*existing = append(*existing, pvc)
	return nil
}

func isSamePVC(pvc1, pvc2 corev1.PersistentVolumeClaim) bool {
	sameObjectMeta := pvc1.Name == pvc2.Name && maps.Equal(pvc1.Labels, pvc2.Labels) &&
		maps.Equal(pvc1.Annotations, pvc2.Annotations)

	storage1 := pvc1.Spec.Resources.Requests.Storage()
	storage2 := pvc2.Spec.Resources.Requests.Storage()

	sameStorage := (storage1 == nil && storage1 == storage2) ||
		(storage1 != nil && storage2 != nil && storage1.Equal(*storage2))
	sameAccessModes := slices.Equal(pvc1.Spec.AccessModes, pvc2.Spec.AccessModes)

	sameStorageClassName := (pvc1.Spec.StorageClassName == nil &&
		pvc1.Spec.StorageClassName == pvc2.Spec.StorageClassName) ||
		(pvc1.Spec.StorageClassName != nil && pvc2.Spec.StorageClassName != nil &&
			*pvc1.Spec.StorageClassName == *pvc2.Spec.StorageClassName)

	sameSpec := sameStorage &&
		sameAccessModes &&
		sameStorageClassName

	return sameObjectMeta && sameSpec
}
