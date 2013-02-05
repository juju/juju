package maas

import (
	"errors"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"sync"
)

type maasEnviron struct {
	name string

	// ecfgMutext protects the *Unlocked fields below.
	ecfgMutex sync.Mutex

	ecfgUnlocked        *maasEnvironConfig
	_MAASServerUnlocked gomaasapi.MAASObject
}

var _ environs.Environ = (*maasEnviron)(nil)

var couldNotAllocate = errors.New("Could not allocate MAAS environment object.")

func NewEnviron(cfg *config.Config) (*maasEnviron, error) {
	env := new(maasEnviron)
	if env == nil {
		return nil, couldNotAllocate
	}
	err := env.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (env *maasEnviron) Name() string {
	return env.name
}

func (env *maasEnviron) Bootstrap(uploadTools bool, stateServerCert, stateServerKey []byte) error {
	log.Printf("environs/maas: bootstrapping environment %q.", env.Name())
	panic("Not implemented.")
}

func (*maasEnviron) StateInfo() (*state.Info, *api.Info, error) {
	panic("Not implemented.")
}

// ecfg returns the environment's maasEnvironConfig, and protects it with a
// mutex.
func (env *maasEnviron) ecfg() *maasEnvironConfig {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.ecfgUnlocked
}

func (env *maasEnviron) Config() *config.Config {
	return env.ecfg().Config
}

func (env *maasEnviron) SetConfig(cfg *config.Config) error {
	ecfg, err := env.Provider().(*maasEnvironProvider).newConfig(cfg)
	if err != nil {
		return err
	}

	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	env.name = cfg.Name()
	env.ecfgUnlocked = ecfg

	authClient, err := gomaasapi.NewAuthenticatedClient(ecfg.MAASServer(), ecfg.MAASOAuth())
	if err != nil {
		return err
	}
	env._MAASServerUnlocked = gomaasapi.NewMAAS(*authClient)

	return nil
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

func (env *maasEnviron) Destroy([]environs.Instance) error {
	log.Printf("environs/maas: destroying environment %q", env.name)
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
	return &providerInstance
}
