// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/core/watcher"
)

type machine interface {
	CharmProfiles() ([]string, error)
	SetUpgradeCharmProfileComplete(unitName string, message string) error
	Tag() names.MachineTag
	WatchUnits() (watcher.StringsWatcher, error)
}

// lifetimeContext was extracted to allow the various context clients to get
// the benefits of the catacomb encapsulating everything that should happen
// here. A clean implementation would almost certainly not need this.
type lifetimeContext interface {
	add(worker.Worker) error
	kill(error)
	dying() <-chan struct{}
	errDying() error
}

type machineContext interface {
	lifetimeContext
}

type mutaterContext interface {
	lifetimeContext
	newMachineContext() machineContext
	getMachine(tag names.MachineTag) (machine, error)
}

type mutater struct {
	context     mutaterContext
	logger      Logger
	machines    map[names.MachineTag]chan struct{}
	machineDead chan machine
}

func watchMachinesLoop(context mutaterContext, machinesWatcher watcher.StringsWatcher) (err error) {
	m := &mutater{
		context:     context,
		machines:    make(map[names.MachineTag]chan struct{}),
		machineDead: make(chan machine),
	}
	defer func() {
		// TODO(fwereade): is this a home-grown sync.WaitGroup or something?
		// strongly suspect these machine goroutines could be managed rather
		// less opaquely if we made them all workers.
		for len(m.machines) > 0 {
			delete(m.machines, (<-m.machineDead).Tag())
		}
	}()
	for {
		select {
		case <-m.context.dying():
			return m.context.errDying()
		case ids, ok := <-machinesWatcher.Changes():
			if !ok {
				return errors.New("machines watcher closed")
			}
			tags := make([]names.MachineTag, len(ids))
			for i := range ids {
				tags[i] = names.NewMachineTag(ids[i])
			}
			if err := m.startMachines(tags); err != nil {
				return err
			}
		case d := <-m.machineDead:
			delete(m.machines, d.Tag())
		}
	}
}

func (m *mutater) startMachines(tags []names.MachineTag) error {
	for _, tag := range tags {
		if c := m.machines[tag]; c == nil {
			m.logger.Warningf("received tag %q", tag.String())

			machine, err := m.context.getMachine(tag)
			if err != nil {
				return errors.Trace(err)
			}
			c = make(chan struct{})
			m.machines[tag] = c

			go runMachine(m.context.newMachineContext(), m.logger, machine, c, m.machineDead)
		} else {
			select {
			case <-m.context.dying():
				return m.context.errDying()
			case c <- struct{}{}:
			}
		}
	}
	return nil
}

func runMachine(context machineContext, logger Logger, m machine, changed <-chan struct{}, died chan<- machine) {
	defer func() {
		// We can't just send on the dead channel because the
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

	unitWatcher, err := m.WatchUnits()
	if err != nil {
		logger.Errorf(err.Error())
		return
	}
	if err := context.add(unitWatcher); err != nil {
		return
	}

	if err := watchUnitLoop(context, logger, m, unitWatcher); err != nil {
		context.kill(err)
	}
}

func watchUnitLoop(context machineContext, logger Logger, m machine, unitWatcher watcher.StringsWatcher) error {
	for {
		select {
		case <-context.dying():
			return context.errDying()
		case unitNames, ok := <-unitWatcher.Changes():
			if !ok {
				return errors.New("unit watcher closed")
			}
			unitsChanged(logger, m, unitNames)
		}
	}
}

func unitsChanged(logger Logger, m machine, names []string) {
	logger.Warningf("Received change on %s.%s", m.Tag(), names)
}
