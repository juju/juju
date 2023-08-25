// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/internal/storage"
)

// checkK8sConfig checks that the attributes in a configuration
// are valid for a K8s deployment.
func checkK8sConfig(attributes map[string]any) error {
	if mediumValue, ok := attributes[storage.K8sStorageMediumConst]; ok {
		medium := fmt.Sprintf("%v", mediumValue)
		if medium != storage.K8sStorageMediumMemory && medium != storage.K8sStorageMediumHugePages {
			return errors.NotValidf("storage medium %q", mediumValue)
		}
	}

	if err := validateStorageAttributes(attributes); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func validateStorageAttributes(attributes map[string]any) error {
	if err := validateStorageConfig(attributes); err != nil {
		return errors.Trace(err)
	}
	if err := validateStorageMode(attributes); err != nil {
		return errors.Trace(err)
	}
	return nil
}

var storageConfigFields = schema.Fields{
	storage.K8sStorageClass:       schema.String(),
	storage.K8sStorageProvisioner: schema.String(),
}

var storageConfigChecker = schema.FieldMap(
	storageConfigFields,
	schema.Defaults{
		storage.K8sStorageClass:       schema.Omit,
		storage.K8sStorageProvisioner: schema.Omit,
	},
)

// validateStorageConfig returns issues in the configuration if any.
func validateStorageConfig(attrs map[string]any) error {
	out, err := storageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return errors.Annotate(err, "validating storage config")
	}
	coerced := out.(map[string]any)

	if coerced[storage.K8sStorageProvisioner] != "" && coerced[storage.K8sStorageClass] == "" {
		return errors.New("storage-class must be specified if storage-provisioner is specified")
	}

	return nil
}

var storageModeFields = schema.Fields{
	storage.K8sStorageMode: schema.String(),
}

var storageModeChecker = schema.FieldMap(
	storageModeFields,
	schema.Defaults{
		storage.K8sStorageMode: "ReadWriteOnce",
	},
)

// validateStorageMode returns an error if the K8s persistent
// volume is not configured correctly.
func validateStorageMode(attrs map[string]any) error {
	out, err := storageModeChecker.Coerce(attrs, nil)
	if err != nil {
		return errors.Annotate(err, "validating storage mode")
	}
	coerced := out.(map[string]any)
	mode := coerced[storage.K8sStorageMode]
	switch coerced[storage.K8sStorageMode] {
	case "ReadOnlyMany", "ROX":
	case "ReadWriteMany", "RWX":
	case "ReadWriteOnce", "RWO":
	default:
		return errors.NotSupportedf("storage mode %q", mode)
	}

	return nil
}
