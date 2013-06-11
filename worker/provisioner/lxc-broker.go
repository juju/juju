package provisioner

import (
	"fmt"
)

func newLxcBroker() Broker {
	return &lxcBroker{}
}

type lxcBroker struct {
}

func (broker *lxcBroker) StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (environs.Instance, error) {

	return nil, fmt.Errorf("Not implemented yet")
}

// StopInstances shuts down the given instances.
func (broker *lxcBroker) StopInstances([]environs.Instance) error {
	return fmt.Errorf("Not implemented yet")
}

func (broker *lxcBroker) AllInstances() ([]environs.Instance, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
