// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/core/watcher"
)

type machine interface {
	CharmProfilingInfo([]string) (*instancemutater.ProfileInfo, error)
	SetCharmProfiles(profiles []string) error
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
	getBroker() environs.LXDProfiler
}

type mutater struct {
	context     mutaterContext
	logger      Logger
	machines    map[names.MachineTag]chan struct{}
	machineDead chan machine
}

func watchMachinesLoop(context mutaterContext, logger Logger, machinesWatcher watcher.StringsWatcher) (err error) {

	m := &mutater{
		context:     context,
		logger:      logger,
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
			m.logger.Debugf("startMachines added machine %s", machine.Tag().Id())
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
	logger.Tracef("runMachine(%s)", m.Tag().Id())
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
	logger.Tracef("watchUnitLoop(%s)", m.Tag().Id())
	for {
		select {
		case <-context.dying():
			return context.errDying()
		case unitNames, ok := <-unitWatcher.Changes():
			if !ok {
				return errors.New("unit watcher closed")
			}
			if len(unitNames) == 0 {
				logger.Tracef("no names reported from unit watcher for %s", m.Tag().String())
				continue
			}
			err := unitsChanged(logger, m, unitNames)
			if err != nil {
				logger.Errorf("from unitsChanged: %#v", err)
			}
			logger.Debugf("end of case unitWatcher changes for %s", m.Tag().String())
		}
	}
}

func unitsChanged(logger Logger, m machine, names []string) error {
	logger.Warningf("Received change on %s.%s", m.Tag(), names)
	info, err := m.CharmProfilingInfo(names)
	logger.Warningf("%#v", info)
	if err != nil {
		return err
	}
	if info == nil {
		logger.Debugf("returning due to nil info")
		return nil
	}
	if !info.Changes {
		// No change is necessary.
		logger.Tracef("no charm profile changes for %s necessary, based on changes to %s", m.Tag().String(),
			strings.Join(names, ", "))
		return nil
	}
	return nil
}
