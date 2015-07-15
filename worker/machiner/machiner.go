// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package machiner

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.machiner")

// Machiner is responsible for a machine agent's lifecycle.
type Machiner struct {
	st                     MachineAccessor
	tag                    names.MachineTag
	machine                Machine
	ignoreAddressesOnStart bool
}

// NewMachiner returns a Worker that will wait for the identified machine
// to become Dying and make it Dead; or until the machine becomes Dead by
// other means.
func NewMachiner(st MachineAccessor, agentConfig agent.Config, ignoreAddressesOnStart bool) worker.Worker {
	mr := &Machiner{st: st, tag: agentConfig.Tag().(names.MachineTag), ignoreAddressesOnStart: ignoreAddressesOnStart}
	return worker.NewNotifyWorker(mr)
}

func (mr *Machiner) SetUp() (watcher.NotifyWatcher, error) {
	// Find which machine we're responsible for.
	m, err := mr.st.Machine(mr.tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, worker.ErrTerminateAgent
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	mr.machine = m

	if mr.ignoreAddressesOnStart {
		logger.Debugf("machine addresses ignored on start - resetting machine addresses")
		if err := m.SetMachineAddresses(nil); err != nil {
			return nil, errors.Annotate(err, "reseting machine addresses")
		}
	} else {
		// Set the addresses in state to the host's addresses.
		if err := setMachineAddresses(mr.tag, m); err != nil {
			return nil, errors.Annotate(err, "setting machine addresses")
		}
	}

	// Mark the machine as started and log it.
	if err := m.SetStatus(params.StatusStarted, "", nil); err != nil {
		return nil, errors.Annotatef(err, "%s failed to set status started", mr.tag)
	}
	logger.Infof("%q started", mr.tag)

	return m.Watch()
}

var interfaceAddrs = net.InterfaceAddrs

// setMachineAddresses sets the addresses for this machine to all of the
// host's non-loopback interface IP addresses.
func setMachineAddresses(tag names.MachineTag, m Machine) error {
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
		address := network.NewAddress(ip.String())
		// Filter out link-local addresses as we cannot reliably use them.
		if address.Scope == network.ScopeLinkLocal {
			continue
		}
		hostAddresses = append(hostAddresses, address)
	}
	if len(hostAddresses) == 0 {
		return nil
	}
	// Filter out any LXC bridge addresses.
	hostAddresses = network.FilterLXCAddresses(hostAddresses)
	logger.Infof("setting addresses for %v to %q", tag, hostAddresses)
	return m.SetMachineAddresses(hostAddresses)
}

func (mr *Machiner) Handle(_ <-chan struct{}) error {
	if err := mr.machine.Refresh(); params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return worker.ErrTerminateAgent
	} else if err != nil {
		return err
	}
	life := mr.machine.Life()
	if life == params.Alive {
		return nil
	}
	logger.Debugf("%q is now %s", mr.tag, life)
	if err := mr.machine.SetStatus(params.StatusStopped, "", nil); err != nil {
		return errors.Annotatef(err, "%s failed to set status stopped", mr.tag)
	}

	// Attempt to mark the machine Dead. If the machine still has units
	// assigned, or storage attached, this will fail with
	// CodeHasAssignedUnits or CodeMachineHasAttachedStorage respectively.
	// Once units or storage are removed, the watcher will trigger again
	// and we'll reattempt.
	if err := mr.machine.EnsureDead(); err != nil {
		if params.IsCodeHasAssignedUnits(err) {
			return nil
		}
		if params.IsCodeMachineHasAttachedStorage(err) {
			logger.Tracef("machine still has storage attached")
			return nil
		}
		return errors.Annotatef(err, "%s failed to set machine to dead", mr.tag)
	}
	return worker.ErrTerminateAgent
}

func (mr *Machiner) TearDown() error {
	// Nothing to do here.
	return nil
}
