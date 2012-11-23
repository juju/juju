// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"sync"
)

type environProvider struct{}

var _ environs.EnvironProvider = (*environProvider)(nil)

var providerInstance environProvider

func init() {
	environs.RegisterProvider("openstack", environProvider{})
}

func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	log.Printf("environs/openstack: opening environment %q", cfg.Name())
	e := new(environ)
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (p environProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	m["username"] = ecfg.username()
	m["password"] = ecfg.password()
	m["tenant-name"] = ecfg.tenantName()
	return m, nil
}

func (p environProvider) PublicAddress() (string, error) {
	panic("not implemented")
}

func (p environProvider) PrivateAddress() (string, error) {
	panic("not implemented")
}

type environ struct {
	name string

	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
}

var _ environs.Environ = (*environ)(nil)

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) Name() string {
	return e.name
}

func (e *environ) Bootstrap(uploadTools bool, certPEM, keyPEM []byte) error {
	panic("not implemented")
}

func (e *environ) StateInfo() (*state.Info, error) {
	panic("not implemented")
}

func (e *environ) Config() *config.Config {
	panic("not implemented")
}

func (e *environ) SetConfig(cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.name = ecfg.Name()
	e.ecfgUnlocked = ecfg

	// TODO(dimitern): setup the goose client auth/compute, etc. here
	return nil
}

func (e *environ) StartInstance(machineId int, info *state.Info, tools *state.Tools) (environs.Instance, error) {
	panic("not implemented")
}

func (e *environ) StopInstances([]environs.Instance) error {
	panic("not implemented")
}

func (e *environ) Instances(ids []string) ([]environs.Instance, error) {
	panic("not implemented")
}

func (e *environ) AllInstances() ([]environs.Instance, error) {
	panic("not implemented")
}

func (e *environ) Storage() environs.Storage {
	panic("not implemented")
}

func (e *environ) PublicStorage() environs.StorageReader {
	panic("not implemented")
}

func (e *environ) Destroy(insts []environs.Instance) error {
	panic("not implemented")
}

func (e *environ) AssignmentPolicy() state.AssignmentPolicy {
	panic("not implemented")
}

func (e *environ) OpenPorts(ports []state.Port) error {
	panic("not implemented")
}

func (e *environ) ClosePorts(ports []state.Port) error {
	panic("not implemented")
}

func (e *environ) Ports() ([]state.Port, error) {
	panic("not implemented")
}

func (e *environ) Provider() environs.EnvironProvider {
	return &providerInstance
}
