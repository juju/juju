package machiner
import (
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/state"
)

func NewMachinerWithContainer(m *state.Machine, cont container.Container) *Machiner {
	return newMachiner(m, cont)
}
