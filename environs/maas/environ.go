package maas

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

type maasEnviron struct{}

var _ environs.Environ = (*maasEnviron)(nil)

func (*maasEnviron) Name() string {
	panic("Not implemented.")
}

func (*maasEnviron) Bootstrap(uploadTools bool, stateServerCert, stateServerKey []byte) error {
	panic("Not implemented.")
}

func (*maasEnviron) StateInfo() (*state.Info, *api.Info, error) {
	panic("Not implemented.")
}

func (*maasEnviron) Config() *config.Config {
	panic("Not implemented.")
}

func (*maasEnviron) SetConfig(*config.Config) error {
	panic("Not implemented.")
}

func (*maasEnviron) StartInstance(machineId string, info *state.Info, apiInfo *api.Info, tools *state.Tools) (environs.Instance, error) {
	panic("Not implemented.")
}

func (*maasEnviron) StopInstances([]environs.Instance) error {
	panic("Not implemented.")
}

func (*maasEnviron) Instances([]state.InstanceId) ([]environs.Instance, error) {
	panic("Not implemented.")
}

func (*maasEnviron) AllInstances() ([]environs.Instance, error) {
	panic("Not implemented.")
}

func (*maasEnviron) Storage() environs.Storage {
	panic("Not implemented.")
}

func (*maasEnviron) PublicStorage() environs.StorageReader {
	panic("Not implemented.")
}

func (*maasEnviron) Destroy([]environs.Instance) error {
	panic("Not implemented.")
}

func (*maasEnviron) AssignmentPolicy() state.AssignmentPolicy {
	panic("Not implemented.")
}

func (*maasEnviron) OpenPorts([]state.Port) error {
	panic("Not implemented.")
}

func (*maasEnviron) ClosePorts([]state.Port) error {
	panic("Not implemented.")
}

func (*maasEnviron) Ports() ([]state.Port, error) {
	panic("Not implemented.")
}

func (*maasEnviron) Provider() environs.EnvironProvider {
	panic("Not implemented.")
}
