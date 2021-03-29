// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
)

// The vmware-specific config keys.
const (
	cfgPrimaryNetwork         = "primary-network"
	cfgExternalNetwork        = "external-network"
	cfgDatastore              = "datastore"
	cfgForceVMHardwareVersion = "force-vm-hardware-version"
	cfgEnableDiskUUID         = "enable-disk-uuid"
	cfgDiskProvisioningType   = "disk-provisioning-type"
)

// configFields is the spec for each vmware config value's type.
var (
	configFields = schema.Fields{
		cfgExternalNetwork:        schema.String(),
		cfgDatastore:              schema.String(),
		cfgPrimaryNetwork:         schema.String(),
		cfgForceVMHardwareVersion: schema.ForceInt(),
		cfgEnableDiskUUID:         schema.Bool(),
		cfgDiskProvisioningType:   schema.String(),
	}

	configDefaults = schema.Defaults{
		cfgExternalNetwork:        "",
		cfgDatastore:              schema.Omit,
		cfgPrimaryNetwork:         schema.Omit,
		cfgForceVMHardwareVersion: int(0),
		cfgEnableDiskUUID:         true,
		cfgDiskProvisioningType:   string(vsphereclient.DiskTypeThickEagerZero),
	}

	configRequiredFields  = []string{}
	configImmutableFields = []string{}
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

// newConfig builds a new environConfig from the provided Config and
// returns it.
func newConfig(cfg *config.Config) *environConfig {
	return &environConfig{
		Config: cfg,
		attrs:  cfg.UnknownAttrs(),
	}
}

// newValidConfig builds a new environConfig from the provided Config
// and returns it. The resulting config values are validated.
func newValidConfig(cfg *config.Config) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, nil); err != nil {
		return nil, errors.Trace(err)
	}

	// Apply the defaults and coerce/validate the custom config attrs.
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}
	validCfg, err := cfg.Apply(validated)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Build the config.
	ecfg := newConfig(validCfg)

	// Do final validation.
	if err := ecfg.validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return ecfg, nil
}

func (c *environConfig) externalNetwork() string {
	return c.attrs[cfgExternalNetwork].(string)
}

func (c *environConfig) datastore() string {
	ds, _ := c.attrs[cfgDatastore].(string)
	return ds
}

func (c *environConfig) primaryNetwork() string {
	network, _ := c.attrs[cfgPrimaryNetwork].(string)
	return network
}

func (c *environConfig) enableDiskUUID() bool {
	return c.attrs[cfgEnableDiskUUID].(bool)
}

func (c *environConfig) forceVMHardwareVersion() int64 {
	versionVal := c.attrs[cfgForceVMHardwareVersion]
	// It seems the value is properly cast to int when bootstrapping
	// but it comes back as a float64 from the database, regardless of
	// the checker used in configFields.
	switch versionVal.(type) {
	case float64:
		v := c.attrs[cfgForceVMHardwareVersion].(float64)
		return int64(v)
	case int:
		v := c.attrs[cfgForceVMHardwareVersion].(int)
		return int64(v)
	default:
		return 0
	}
}

func (c *environConfig) diskProvisioningType() vsphereclient.DiskProvisioningType {
	provType, ok := c.attrs[cfgDiskProvisioningType]
	if !ok {
		// Return the default in case none is set.
		return vsphereclient.DefaultDiskProvisioningType
	}

	provTypeStr, ok := provType.(string)
	if !ok || provTypeStr == "" {
		// We got an invalid value set, return default.
		return vsphereclient.DefaultDiskProvisioningType
	}

	return vsphereclient.DiskProvisioningType(provTypeStr)
}

// validate checks vmware-specific config values.
func (c environConfig) validate() error {
	// All fields must be populated, even with just the default.
	for _, field := range configRequiredFields {
		if c.attrs[field].(string) == "" {
			return errors.Errorf("%s: must not be empty", field)
		}
	}

	if diskProvType, ok := c.attrs[cfgDiskProvisioningType]; ok {
		diskProvTypeStr, ok := diskProvType.(string)
		if !ok {
			return errors.Errorf("%s must be a string", cfgDiskProvisioningType)
		}

		if diskProvTypeStr != "" {
			found := false
			for _, val := range vsphereclient.ValidDiskProvisioningTypes {
				if vsphereclient.DiskProvisioningType(diskProvTypeStr) == val {
					found = true
					break
				}
			}
			if !found {
				return errors.Errorf(
					"%q must be one of %q", cfgDiskProvisioningType, vsphereclient.ValidDiskProvisioningTypes)
			}
		}
	}
	return nil
}

// update applies changes from the provided config to the env config.
// Changes to any immutable attributes result in an error.
func (c *environConfig) update(cfg *config.Config) error {
	// Validate the updates. newValidConfig does not modify the "known"
	// config attributes so it is safe to call Validate here first.
	if err := config.Validate(cfg, c.Config); err != nil {
		return errors.Trace(err)
	}

	updates, err := newValidConfig(cfg)
	if err != nil {
		return errors.Trace(err)
	}

	// Check that no immutable fields have changed.
	attrs := updates.UnknownAttrs()
	for _, field := range configImmutableFields {
		if attrs[field] != c.attrs[field] {
			return errors.Errorf("%s: cannot change from %v to %v", field, c.attrs[field], attrs[field])
		}
	}

	// Apply the updates.
	c.Config = cfg
	c.attrs = cfg.UnknownAttrs()
	return nil
}
