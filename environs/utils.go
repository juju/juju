package environs

import (
	"fmt"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
)

// GetStorage creates an Environ from the config in state and returns
// its storage interface.
func GetStorage(e interface {
	EnvironConfig() (*config.Config, error)
}) (storage.Storage, error) {
	envConfig, err := e.EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot get environment config: %v", err)
	}
	env, err := New(envConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot access environment: %v", err)
	}
	return env.Storage(), nil
}
