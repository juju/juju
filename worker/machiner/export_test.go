package machiner

import (
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/state"
)

func NewMachinerWithContainer(m *state.Machine, info *state.Info, dataDir string, cont container.Container) *Machiner {
	return newMachiner(m, info, dataDir, cont)
}
