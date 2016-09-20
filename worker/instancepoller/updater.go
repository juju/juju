// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
)

var logger = loggo.GetLogger("juju.worker.instancepoller")

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
	InstanceStatus() (params.StatusResult, error)
	SetInstanceStatus(status.Status, string, map[string]interface{}) error
	String() string
	Refresh() error
	Life() params.Life
	Status() (params.StatusResult, error)
	IsManual() (bool, error)
}

type instanceInfo struct {
	addresses []network.Address
	status    instance.InstanceStatus
}

// lifetimeContext was extracted to allow the various context clients to get
// the benefits of the catacomb encapsulating everything that should happen
// here. A clean implementation would almost certainly not need this.
type lifetimeContext interface {
	kill(error)
	dying() <-chan struct{}
	errDying() error
}

type machineContext interface {
	lifetimeContext
	instanceInfo(id instance.Id) (instanceInfo, error)
}

type updaterContext interface {
	lifetimeContext
	newMachineContext() machineContext
	getMachine(tag names.MachineTag) (machine, error)
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
func watchMachinesLoop(context updaterContext, machinesWatcher watcher.StringsWatcher) (err error) {
	p := &updater{
		context:     context,
		machines:    make(map[names.MachineTag]chan struct{}),
		machineDead: make(chan machine),
	}
	defer func() {
		// TODO(fwereade): is this a home-grown sync.WaitGroup or something?
		// strongly suspect these machine goroutines could be managed rather
		// less opaquely if we made them all workers.
		for len(p.machines) > 0 {
			delete(p.machines, (<-p.machineDead).Tag())
		}
	}()
	for {
		select {
		case <-p.context.dying():
			return p.context.errDying()
		case ids, ok := <-machinesWatcher.Changes():
			if !ok {
				return errors.New("machines watcher closed")
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
		}
	}
}

func (p *updater) startMachines(tags []names.MachineTag) error {
	for _, tag := range tags {
		if c := p.machines[tag]; c == nil {
			// We don't know about the machine - start
			// a goroutine to deal with it.
			m, err := p.context.getMachine(tag)
			if err != nil {
				return errors.Trace(err)
			}
			// We don't poll manual machines.
			isManual, err := m.IsManual()
			if err != nil {
				return errors.Trace(err)
			}
			if isManual {
				continue
			}
			c = make(chan struct{})
			p.machines[tag] = c
			// TODO(fwereade): 2016-03-17 lp:1558657
			go runMachine(p.context.newMachineContext(), m, c, p.machineDead, clock.WallClock)
		} else {
			select {
			case <-p.context.dying():
				return p.context.errDying()
			case c <- struct{}{}:
			}
		}
	}
	return nil
}

// runMachine processes the address and status publishing for a given machine.
// We assume that the machine is alive when this is first called.
func runMachine(context machineContext, m machine, changed <-chan struct{}, died chan<- machine, clock clock.Clock) {
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
	if err := machineLoop(context, m, changed, clock); err != nil {
		context.kill(err)
	}
}

func machineLoop(context machineContext, m machine, lifeChanged <-chan struct{}, clock clock.Clock) error {
	// Use a short poll interval when initially waiting for
	// a machine's address and machine agent to start, and a long one when it already
	// has an address and the machine agent is started.
	pollInterval := ShortPoll
	pollInstance := func() error {
		instInfo, err := pollInstanceInfo(context, m)
		if err != nil {
			return err
		}

		machineStatus := status.Pending
		if err == nil {
			if statusInfo, err := m.Status(); err != nil {
				logger.Warningf("cannot get current machine status for machine %v: %v", m.Id(), err)
			} else {
				// TODO(perrito666) add status validation.
				machineStatus = status.Status(statusInfo.Status)
			}
		}

		// the extra condition below (checking allocating/pending) is here to improve user experience
		// without it the instance status will say "pending" for +10 minutes after the agent comes up to "started"
		if instInfo.status.Status != status.Allocating && instInfo.status.Status != status.Pending {
			if len(instInfo.addresses) > 0 && machineStatus == status.Started {
				// We've got at least one address and a status and instance is started, so poll infrequently.
				pollInterval = LongPoll
			} else if pollInterval < LongPoll {
				// We have no addresses or not started - poll increasingly rarely
				// until we do.
				pollInterval = time.Duration(float64(pollInterval) * ShortPollBackoff)
			}
		}
		return nil
	}

	shouldPollInstance := true
	for {
		if shouldPollInstance {
			if err := pollInstance(); err != nil {
				if !params.IsCodeNotProvisioned(err) {
					return errors.Trace(err)
				}
			}
			shouldPollInstance = false
		}
		select {
		case <-context.dying():
			return context.errDying()
		case <-clock.After(pollInterval):
			shouldPollInstance = true
		case <-lifeChanged:
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
		return instanceInfo{}, err
	}
	if err != nil {
		return instanceInfo{}, errors.Annotate(err, "cannot get machine's instance id")
	}
	instInfo, err = context.instanceInfo(instId)
	if err != nil {
		// TODO (anastasiamac 2016-02-01) This does not look like it needs to be removed now.
		if params.IsCodeNotImplemented(err) {
			return instanceInfo{}, err
		}
		logger.Warningf("cannot get instance info for instance %q: %v", instId, err)
		return instInfo, nil
	}
	if instStat, err := m.InstanceStatus(); err != nil {
		// This should never occur since the machine is provisioned.
		// But just in case, we reset polled status so we try again next time.
		logger.Warningf("cannot get current instance status for machine %v: %v", m.Id(), err)
		instInfo.status = instance.InstanceStatus{status.Unknown, ""}
	} else {
		// TODO(perrito666) add status validation.
		currentInstStatus := instance.InstanceStatus{
			Status:  status.Status(instStat.Status),
			Message: instStat.Info,
		}
		if instInfo.status != currentInstStatus {
			logger.Infof("machine %q instance status changed from %q to %q", m.Id(), currentInstStatus, instInfo.status)
			if err = m.SetInstanceStatus(instInfo.status.Status, instInfo.status.Message, nil); err != nil {
				logger.Errorf("cannot set instance status on %q: %v", m, err)
				return instanceInfo{}, err
			}
		}
	}
	if m.Life() != params.Dead {
		providerAddresses, err := m.ProviderAddresses()
		if err != nil {
			return instanceInfo{}, err
		}
		if !addressesEqual(providerAddresses, instInfo.addresses) {
			logger.Infof("machine %q has new addresses: %v", m.Id(), instInfo.addresses)
			if err := m.SetProviderAddresses(instInfo.addresses...); err != nil {
				logger.Errorf("cannot set addresses on %q: %v", m, err)
				return instanceInfo{}, err
			}
		}
	}
	return instInfo, nil
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
	network.SortAddresses(ca0)
	ca1 := make([]network.Address, len(a1))
	copy(ca1, a1)
	network.SortAddresses(ca1)

	for i := range ca0 {
		if ca0[i] != ca1[i] {
			logger.Tracef("address entry at offset %d has a different value for %v != %v",
				i, ca0, ca1)
			return false
		}
	}
	return true
}
