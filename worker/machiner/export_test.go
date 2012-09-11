package machiner

import (
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/state"
)

func NewMachinerWithContainer(m *state.Machine, info *state.Info, cont container.Container, tools *state.Tools) *Machiner {
	return newMachiner(m, cont)
}
