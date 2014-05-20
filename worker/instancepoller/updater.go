// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
)

var logger = loggo.GetLogger("juju.worker.instanceupdater")

// ShortPoll and LongPoll hold the polling intervals for the instance
// updater. When a machine has no address or is not started, it will be
// polled at ShortPoll intervals until it does, exponentially backing off
// with an exponent of ShortPollBackoff until a maximum(ish) of LongPoll.
//
// When a machine has an address and is started LongPoll will be used to
// check that the instance address or status has not changed.
var (
	ShortPoll        = 1 * time.Second
	ShortPollBackoff = 2.0
	LongPoll         = 15 * time.Minute
)

type machine interface {
	Id() string
	InstanceId() (instance.Id, error)
	Addresses() []instance.Address
	SetAddresses(...instance.Address) error
	InstanceStatus() (string, error)
	SetInstanceStatus(status string) error
	String() string
	Refresh() error
	Life() state.Life
	Status() (status params.Status, info string, data params.StatusData, err error)
	IsManual() (bool, error)
}

type instanceInfo struct {
	addresses []instance.Address
	status    string
}

type machineContext interface {
	killAll(err error)
	instanceInfo(id instance.Id) (instanceInfo, error)
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
			if errors.IsNotFound(err) {
				logger.Warningf("watcher gave notification of non-existent machine %q", id)
				continue
			}
			if err != nil {
				return err
			}
			// We don't poll manual machines.
			isManual, err := m.IsManual()
			if err != nil {
				return err
			}
			if isManual {
				continue
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

// runMachine processes the address and status publishing for a given machine.
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
	// a machine's address and machine agent to start, and a long one when it already
	// has an address and the machine agent is started.
	pollInterval := ShortPoll
	pollInstance := true
	for {
		if pollInstance {
			instInfo, err := pollInstanceInfo(context, m)
			if err != nil && !state.IsNotProvisionedError(err) {
				// If the provider doesn't implement Addresses/Status now,
				// it never will until we're upgraded, so don't bother
				// asking any more. We could use less resources
				// by taking down the entire worker, but this is easier for now
				// (and hopefully the local provider will implement
				// Addresses/Status in the not-too-distant future),
				// so we won't need to worry about this case at all.
				if errors.IsNotImplemented(err) {
					pollInterval = 365 * 24 * time.Hour
				} else {
					return err
				}
			}
			machineStatus := params.StatusPending
			if err == nil {
				if machineStatus, _, _, err = m.Status(); err != nil {
					logger.Warningf("cannot get current machine status for machine %v: %v", m.Id(), err)
				}
			}
			if len(instInfo.addresses) > 0 && instInfo.status != "" && machineStatus == params.StatusStarted {
				// We've got at least one address and a status and instance is started, so poll infrequently.
				pollInterval = LongPoll
			} else if pollInterval < LongPoll {
				// We have no addresses or not started - poll increasingly rarely
				// until we do.
				pollInterval = time.Duration(float64(pollInterval) * ShortPollBackoff)
			}
			pollInstance = false
		}
		select {
		case <-time.After(pollInterval):
			pollInstance = true
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

// pollInstanceInfo checks the current provider addresses and status
// for the given machine's instance, and sets them on the machine if they've changed.
func pollInstanceInfo(context machineContext, m machine) (instInfo instanceInfo, err error) {
	instInfo = instanceInfo{}
	instId, err := m.InstanceId()
	// We can't ask the machine for its addresses if it isn't provisioned yet.
	if state.IsNotProvisionedError(err) {
		return instInfo, err
	}
	if err != nil {
		return instInfo, fmt.Errorf("cannot get machine's instance id: %v", err)
	}
	instInfo, err = context.instanceInfo(instId)
	if err != nil {
		if errors.IsNotImplemented(err) {
			return instInfo, err
		}
		logger.Warningf("cannot get instance info for instance %q: %v", instId, err)
		return instInfo, nil
	}
	currentInstStatus, err := m.InstanceStatus()
	if err != nil {
		// This should never occur since the machine is provisioned.
		// But just in case, we reset polled status so we try again next time.
		logger.Warningf("cannot get current instance status for machine %v: %v", m.Id(), err)
		instInfo.status = ""
	} else {
		if instInfo.status != currentInstStatus {
			logger.Infof("machine %q has new instance status: %v", m.Id(), instInfo.status)
			if err = m.SetInstanceStatus(instInfo.status); err != nil {
				logger.Errorf("cannot set instance status on %q: %v", m, err)
			}
		}
	}
	if !addressesEqual(m.Addresses(), instInfo.addresses) {
		logger.Infof("machine %q has new addresses: %v", m.Id(), instInfo.addresses)
		if err = m.SetAddresses(instInfo.addresses...); err != nil {
			logger.Errorf("cannot set addresses on %q: %v", m, err)
		}
	}
	return instInfo, err
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
