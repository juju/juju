// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
)

type azureEnviron struct{}

// azureEnviron implements Environ.
var _ environs.Environ = (*azureEnviron)(nil)

// Name is specified in the Environ interface.
func (env *azureEnviron) Name() string {
	panic("unimplemented")
}

// Bootstrap is specified in the Environ interface.
func (env *azureEnviron) Bootstrap(cons constraints.Value) error {
	panic("unimplemented")
}

// StateInfo is specified in the Environ interface.
func (env *azureEnviron) StateInfo() (*state.Info, *api.Info, error) {
	panic("unimplemented")
}

// Config is specified in the Environ interface.
func (env *azureEnviron) Config() *config.Config {
	panic("unimplemented")
}

// SetConfig is specified in the Environ interface.
func (env *azureEnviron) SetConfig(cfg *config.Config) error {
	panic("unimplemented")
}

// StartInstance is specified in the Environ interface.
func (env *azureEnviron) StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (instance.Instance, error) {
	panic("unimplemented")
}

// StopInstances is specified in the Environ interface.
func (env *azureEnviron) StopInstances([]instance.Instance) error {
	panic("unimplemented")
}

// Instances is specified in the Environ interface.
func (env *azureEnviron) Instances(ids []state.InstanceId) ([]instance.Instance, error) {
	panic("unimplemented")
}

// AllInstances is specified in the Environ interface.
func (env *azureEnviron) AllInstances() ([]instance.Instance, error) {
	panic("unimplemented")
}

// Storage is specified in the Environ interface.
func (env *azureEnviron) Storage() environs.Storage {
	panic("unimplemented")
}

// PublicStorage is specified in the Environ interface.
func (env *azureEnviron) PublicStorage() environs.StorageReader {
	panic("unimplemented")
}

// Destroy is specified in the Environ interface.
func (env *azureEnviron) Destroy(insts []instance.Instance) error {
	panic("unimplemented")
}

// AssignmentPolicy is specified in the Environ interface.
func (env *azureEnviron) AssignmentPolicy() state.AssignmentPolicy {
	panic("unimplemented")
}

// OpenPorts is specified in the Environ interface.
func (env *azureEnviron) OpenPorts(ports []params.Port) error {
	panic("unimplemented")
}

// ClosePorts is specified in the Environ interface.
func (env *azureEnviron) ClosePorts(ports []params.Port) error {
	panic("unimplemented")
}

// Ports is specified in the Environ interface.
func (env *azureEnviron) Ports() ([]params.Port, error) {
	panic("unimplemented")
}

// Provider is specified in the Environ interface.
func (env *azureEnviron) Provider() environs.EnvironProvider {
	panic("unimplemented")
}
