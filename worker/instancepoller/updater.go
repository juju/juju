// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"fmt"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/watcher"
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
	Tag() names.MachineTag
	InstanceId() (instance.Id, error)
	ProviderAddresses() ([]network.Address, error)
	SetProviderAddresses(...network.Address) error
	InstanceStatus() (string, error)
	SetInstanceStatus(status string) error
	String() string
	Refresh() error
	Life() params.Life
	Status() (params.StatusResult, error)
	IsManual() (bool, error)
}

type instanceInfo struct {
	addresses []network.Address
	status    string
}

type machineContext interface {
	killAll(err error)
	instanceInfo(id instance.Id) (instanceInfo, error)
	dying() <-chan struct{}
}

type machineAddress struct {
	machine   machine
	addresses []network.Address
}

type machinesWatcher interface {
	Changes() <-chan []string
	Err() error
	Stop() error
}

type updaterContext interface {
	newMachineContext() machineContext
	getMachine(tag names.MachineTag) (machine, error)
	dying() <-chan struct{}
}

type updater struct {
	context     updaterContext
	machines    map[names.MachineTag]chan struct{}
	machineDead chan machine
}

// watchMachinesLoop watches for changes provided by the given
// machinesWatcher and starts machine goroutines to deal with them,
// using the provided newMachineContext function to create the
// appropriate context for each new machine tag.
func watchMachinesLoop(context updaterContext, w machinesWatcher) (err error) {
	p := &updater{
		context:     context,
		machines:    make(map[names.MachineTag]chan struct{}),
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
			delete(p.machines, (<-p.machineDead).Tag())
		}
	}()
	for {
		select {
		case ids, ok := <-w.Changes():
			if !ok {
				return watcher.EnsureErr(w)
			}
			tags := make([]names.MachineTag, len(ids))
			for i := range ids {
				tags[i] = names.NewMachineTag(ids[i])
			}
			if err := p.startMachines(tags); err != nil {
				return err
			}
		case m := <-p.machineDead:
			delete(p.machines, m.Tag())
		case <-p.context.dying():
			return nil
		}
	}
}

func (p *updater) startMachines(tags []names.MachineTag) error {
	for _, tag := range tags {
		if c := p.machines[tag]; c == nil {
			// We don't know about the machine - start
			// a goroutine to deal with it.
			m, err := p.context.getMachine(tag)
			if params.IsCodeNotFound(err) {
				logger.Warningf("watcher gave notification of non-existent machine %q", tag.Id())
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
			p.machines[tag] = c
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
			if err != nil && !params.IsCodeNotProvisioned(err) {
				// If the provider doesn't implement Addresses/Status now,
				// it never will until we're upgraded, so don't bother
				// asking any more. We could use less resources
				// by taking down the entire worker, but this is easier for now
				// (and hopefully the local provider will implement
				// Addresses/Status in the not-too-distant future),
				// so we won't need to worry about this case at all.
				if params.IsCodeNotImplemented(err) {
					pollInterval = 365 * 24 * time.Hour
				} else {
					return err
				}
			}
			machineStatus := params.StatusPending
			if err == nil {
				if statusInfo, err := m.Status(); err != nil {
					logger.Warningf("cannot get current machine status for machine %v: %v", m.Id(), err)
				} else {
					machineStatus = statusInfo.Status
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
			if m.Life() == params.Dead {
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
	if params.IsCodeNotProvisioned(err) {
		return instInfo, err
	}
	if err != nil {
		return instInfo, fmt.Errorf("cannot get machine's instance id: %v", err)
	}
	instInfo, err = context.instanceInfo(instId)
	if err != nil {
		if params.IsCodeNotImplemented(err) {
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
			logger.Infof("machine %q instance status changed from %q to %q", m.Id(), currentInstStatus, instInfo.status)
			if err = m.SetInstanceStatus(instInfo.status); err != nil {
				logger.Errorf("cannot set instance status on %q: %v", m, err)
			}
		}
	}
	providerAddresses, err := m.ProviderAddresses()
	if err != nil {
		return instInfo, err
	}
	if !addressesEqual(providerAddresses, instInfo.addresses) {
		logger.Infof("machine %q has new addresses: %v", m.Id(), instInfo.addresses)
		if err = m.SetProviderAddresses(instInfo.addresses...); err != nil {
			logger.Errorf("cannot set addresses on %q: %v", m, err)
		}
	}
	return instInfo, err
}

// addressesEqual compares the addresses of the machine and the instance information.
func addressesEqual(a0, a1 []network.Address) bool {
	if len(a0) != len(a1) {
		logger.Tracef("address lists have different lengths %d != %d for %v != %v",
			len(a0), len(a1), a0, a1)
		return false
	}

	ca0 := make([]network.Address, len(a0))
	copy(ca0, a0)
	network.SortAddresses(ca0, true)
	ca1 := make([]network.Address, len(a1))
	copy(ca1, a1)
	network.SortAddresses(ca1, true)

	for i := range ca0 {
		if ca0[i] != ca1[i] {
			logger.Tracef("address entry at offset %d has a different value for %v != %v",
				i, ca0, ca1)
			return false
		}
	}
	return true
}
