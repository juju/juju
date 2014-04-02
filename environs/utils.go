package environs

import (
	"fmt"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/state"
)

// GetStorage creates an Environ from the config in state and returns
// its storage interface.
func GetStorage(st *state.State) (storage.Storage, error) {
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot get environment config: %v", err)
	}
	env, err := New(envConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot access environment: %v", err)
	}
	return env.Storage(), nil
}
