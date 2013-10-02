// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addressupdater

import (
	"fmt"
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
)

var logger = loggo.GetLogger("juju.worker.addressupdater")

var (
	longPoll  = 10 * time.Second
	shortPoll = 500 * time.Millisecond
)

type machine interface {
	Id() string
	Addresses() []instance.Address
	InstanceId() (instance.Id, error)
	SetAddresses([]instance.Address) error
	String() string
	Refresh() error
	Life() state.Life
}

type machineContext interface {
	killAll(err error)
	addresses(id instance.Id) ([]instance.Address, error)
	dying() <-chan struct{}
}

type machineAddress struct {
	machine   machine
	addresses []instance.Address
}

var _ machine = (*state.Machine)(nil)

type machinesWatcher interface {
	Changes() <-chan []string
	Err() error
	Stop() error
}

type updaterContext interface {
	newMachineContext() machineContext
	getMachine(id string) (machine, error)
	dying() <-chan struct{}
}

type updater struct {
	context     updaterContext
	machines    map[string]chan struct{}
	machineDead chan machine
}

// watchMachinesLoop watches for changes provided by the given
// machinesWatcher and starts machine goroutines to deal
// with them, using the provided newMachineContext
// function to create the appropriate context for each new machine id.
func watchMachinesLoop(context updaterContext, w machinesWatcher) (err error) {
	p := &updater{
		context:     context,
		machines:    make(map[string]chan struct{}),
		machineDead: make(chan machine),
	}
	defer func() {
		if stopErr := w.Stop(); stopErr != nil {
			if err == nil {
				err = fmt.Errorf("error stopping watcher: %v", stopErr)
			} else {
				logger.Warningf("ignoring error when stopping watcher: %v", stopErr)
			}
		}
		for len(p.machines) > 0 {
			delete(p.machines, (<-p.machineDead).Id())
		}
	}()
	for {
		select {
		case ids, ok := <-w.Changes():
			if !ok {
				return watcher.MustErr(w)
			}
			if err := p.startMachines(ids); err != nil {
				return err
			}
		case m := <-p.machineDead:
			delete(p.machines, m.Id())
		case <-p.context.dying():
			return nil
		}
	}
}

func (p *updater) startMachines(ids []string) error {
	for _, id := range ids {
		if c := p.machines[id]; c == nil {
			// We don't know about the machine - start
			// a goroutine to deal with it.
			m, err := p.context.getMachine(id)
			if errors.IsNotFoundError(err) {
				logger.Warningf("watcher gave notification of non-existent machine %q", id)
				continue
			}
			if err != nil {
				return err
			}
			c = make(chan struct{})
			p.machines[id] = c
			go runMachine(p.context.newMachineContext(), m, c, p.machineDead)
		} else {
			c <- struct{}{}
		}
	}
	return nil
}

// runMachine processes the address publishing for a given machine.
// We assume that the machine is alive when this is first called.
func runMachine(context machineContext, m machine, changed <-chan struct{}, died chan<- machine) {
	defer func() {
		// We can't just send on the died channel because the
		// central loop might be trying to write to us on the
		// changed channel.
		for {
			select {
			case died <- m:
				return
			case <-changed:
			}
		}
	}()
	if err := machineLoop(context, m, changed); err != nil {
		context.killAll(err)
	}
}

func machineLoop(context machineContext, m machine, changed <-chan struct{}) error {
	// Use a short poll interval when initially waiting for
	// a machine's address, and a long one when it already
	// has an address.
	pollInterval := shortPoll
	checkAddress := true
	for {
		if checkAddress {
			if err := checkMachineAddresses(context, m); err != nil {
				return err
			}
			if len(m.Addresses()) > 0 {
				pollInterval = longPoll
			}
			checkAddress = false
		}
		select {
		case <-time.After(pollInterval):
			checkAddress = true
		case <-context.dying():
			return nil
		case <-changed:
			if err := m.Refresh(); err != nil {
				return err
			}
			if m.Life() == state.Dead {
				return nil
			}
		}
	}
}

// checkMachineAddresses checks the current provider addresses
// for the given machine's instance, and sets them
// on the machine if they've changed.
func checkMachineAddresses(context machineContext, m machine) error {
	instId, err := m.InstanceId()
	if err != nil && !state.IsNotProvisionedError(err) {
		return fmt.Errorf("cannot get machine's instance id: %v", err)
	}
	var newAddrs []instance.Address
	if err == nil {
		newAddrs, err = context.addresses(instId)
		if err != nil {
			logger.Warningf("cannot get addresses for instance %q: %v", instId, err)
			return nil
		}
	}
	if addressesEqual(m.Addresses(), newAddrs) {
		return nil
	}
	if err := m.SetAddresses(newAddrs); err != nil {
		return fmt.Errorf("cannot set addresses on %q: %v", m, err)
	}
	return nil
}

func addressesEqual(a0, a1 []instance.Address) bool {
	if len(a0) != len(a1) {
		return false
	}
	for i := range a0 {
		if a0[i] != a1[i] {
			return false
		}
	}
	return true
}
