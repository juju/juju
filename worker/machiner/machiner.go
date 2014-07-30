// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package machiner

import (
	"fmt"
	"net"

	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api/machiner"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.machiner")

// Machiner is responsible for a machine agent's lifecycle.
type Machiner struct {
	st      *machiner.State
	tag     names.MachineTag
	machine *machiner.Machine
}

// NewMachiner returns a Worker that will wait for the identified machine
// to become Dying and make it Dead; or until the machine becomes Dead by
// other means.
func NewMachiner(st *machiner.State, agentConfig agent.Config) worker.Worker {
	// TODO(dfc) clearly agentConfig.Tag() can _only_ return a machine tag
	mr := &Machiner{st: st, tag: agentConfig.Tag().(names.MachineTag)}
	return worker.NewNotifyWorker(mr)
}

func (mr *Machiner) SetUp() (watcher.NotifyWatcher, error) {
	// Find which machine we're responsible for.
	m, err := mr.st.Machine(mr.tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, worker.ErrTerminateAgent
	} else if err != nil {
		return nil, err
	}
	mr.machine = m

	// Set the addresses in state to the host's addresses.
	if err := setMachineAddresses(m); err != nil {
		return nil, err
	}

	// Mark the machine as started and log it.
	if err := m.SetStatus(params.StatusStarted, "", nil); err != nil {
		return nil, fmt.Errorf("%s failed to set status started: %v", mr.tag, err)
	}
	logger.Infof("%q started", mr.tag)

	return m.Watch()
}

var interfaceAddrs = net.InterfaceAddrs

// setMachineAddresses sets the addresses for this machine to all of the
// host's non-loopback interface IP addresses.
func setMachineAddresses(m *machiner.Machine) error {
	addrs, err := interfaceAddrs()
	if err != nil {
		return err
	}
	var hostAddresses []network.Address
	for _, addr := range addrs {
		var ip net.IP
		switch addr := addr.(type) {
		case *net.IPAddr:
			ip = addr.IP
		case *net.IPNet:
			ip = addr.IP
		default:
			continue
		}
		address := network.NewAddress(ip.String(), network.ScopeUnknown)
		hostAddresses = append(hostAddresses, address)
	}
	if len(hostAddresses) == 0 {
		return nil
	}
	logger.Infof("setting addresses for %v to %q", m.Tag(), hostAddresses)
	return m.SetMachineAddresses(hostAddresses)
}

func (mr *Machiner) Handle() error {
	if err := mr.machine.Refresh(); params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return worker.ErrTerminateAgent
	} else if err != nil {
		return err
	}
	if mr.machine.Life() == params.Alive {
		return nil
	}
	logger.Debugf("%q is now %s", mr.tag, mr.machine.Life())
	if err := mr.machine.SetStatus(params.StatusStopped, "", nil); err != nil {
		return fmt.Errorf("%s failed to set status stopped: %v", mr.tag, err)
	}

	// If the machine is Dying, it has no units,
	// and can be safely set to Dead.
	if err := mr.machine.EnsureDead(); err != nil {
		return fmt.Errorf("%s failed to set machine to dead: %v", mr.tag, err)
	}
	return worker.ErrTerminateAgent
}

func (mr *Machiner) TearDown() error {
	// Nothing to do here.
	return nil
}
