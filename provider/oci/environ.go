// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
)

type Environ struct{}

var _ common.ZonedEnviron = (*Environ)(nil)
var _ storage.ProviderRegistry = (*Environ)(nil)
var _ environs.Environ = (*Environ)(nil)
var _ environs.Firewaller = (*Environ)(nil)
var _ environs.Networking = (*Environ)(nil)
var _ environs.NetworkingEnviron = (*Environ)(nil)

// AvailabilityZones is defined in the common.ZonedEnviron interface
func (e *Environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	return nil, errors.NotImplementedf("AvailabilityZones")
}

// InstanceAvailabilityzoneNames implements common.ZonedEnviron.
func (e *Environ) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	return nil, errors.NotImplementedf("InstanceAvailabilityZoneNames")
}

// DeriveAvailabilityZones implements common.ZonedEnviron.
func (e *Environ) DeriveAvailabilityZones(args environs.StartInstanceParams) ([]string, error) {
	return nil, errors.NotImplementedf("DeriveAvailabilityZones")
}

// Instances implements environs.Environ.
func (e *Environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return nil, errors.NotImplementedf("Instances")
}

// PrepareForBootstrap implements environs.Environ.
func (e *Environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	return errors.NotImplementedf("PrepareForBootstrap")
}

// Bootstrap implements environs.Environ.
func (e *Environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, e, params)
}

// Create implements environs.Environ.
func (e *Environ) Create(params environs.CreateParams) error {
	return errors.NotImplementedf("Create")
}

// AdoptResources implements environs.Environ.
func (e *Environ) AdoptResources(controllerUUID string, fromVersion version.Number) error {
	return errors.NotImplementedf("AdoptResources")
}

// ConstraintsValidator implements environs.Environ.
func (e *Environ) ConstraintsValidator() (constraints.Validator, error) {
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

// SetConfig implements environs.Environ.
func (e *Environ) SetConfig(cfg *config.Config) error {
	return errors.NotImplementedf("SetConfig")
}

// ControllerInstances implements environs.Environ.
func (e *Environ) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
	return nil, errors.NotImplementedf("ControllerInstances")
}

// Destroy implements environs.Environ.
func (e *Environ) Destroy() error {
	return common.Destroy(e)
}

// DestroyController implements environs.Environ.
func (e *Environ) DestroyController(controllerUUID string) error {
	return errors.NotImplementedf("DestroyController")
}

// Provider implements environs.Environ.
func (e *Environ) Provider() environs.EnvironProvider {
	return nil
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (e *Environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{}, errors.NotImplementedf("StorageProviderTypes")
}

// StorageProvider implements storage.ProviderRegistry.
func (e *Environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}

// StartInstance implements environs.InstanceBroker.
func (e *Environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	return nil, errors.NotImplementedf("StartInstance")
}

// StopInstances implements environs.InstanceBroker.
func (e *Environ) StopInstances(ids ...instance.Id) error {
	return errors.NotImplementedf("StopInstances")
}

// AllInstances implements environs.InstanceBroker.
func (e *Environ) AllInstances() ([]instance.Instance, error) {
	return nil, errors.NotImplementedf("AllInstances")
}

// MaintainInstance implements environs.InstanceBroker.
func (e *Environ) MaintainInstance(args environs.StartInstanceParams) error {
	return errors.NotImplementedf("MaintainInstance")
}

// Config implements environs.ConfigGetter.
func (e *Environ) Config() *config.Config {
	return nil
}

// PrecheckInstance implements environs.InstancePrechecker.
func (e *Environ) PrecheckInstance(environs.PrecheckInstanceParams) error {
	return errors.NotImplementedf("PrecheckInstance")
}

// InstanceTypes implements environs.InstancePrechecker.
func (e *Environ) InstanceTypes(constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	return instances.InstanceTypesWithCostMetadata{}, errors.NotImplementedf("InstanceTypes")
}
