// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/logger"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
)

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
	//Logger
	getBroker() environs.LXDProfiler
}

type mutaterMachine struct {
	context    machineContext
	logger     Logger
	machineApi instancemutater.MutaterMachine
	id         string
}

type mutaterContext interface {
	//lifetimeContext
	machineContext
	newMachineContext() machineContext
	getMachine(tag names.MachineTag) (instancemutater.MutaterMachine, error)
	//getBroker() environs.LXDProfiler
}

type mutater struct {
	context     mutaterContext
	logger      Logger
	machines    map[names.MachineTag]chan struct{}
	machineDead chan instancemutater.MutaterMachine
}

func (m *mutater) startMachines(tags []names.MachineTag) error {
	for _, tag := range tags {
		if c := m.machines[tag]; c == nil {
			m.logger.Warningf("received tag %q", tag.String())

			api, err := m.context.getMachine(tag)
			if err != nil {
				// If the machine is not found, don't fail in hard way and
				// continue onwards until a mach is found for a subsequent
				// tag.
				//if errors.IsNotFound(err) {
				//	continue
				//}
				return errors.Trace(err)
			}

			id := api.Tag().Id()
			m.logger.Tracef("startMachines added machine %s", id)
			c = make(chan struct{})
			m.machines[tag] = c

			machine := mutaterMachine{
				context:    m.context.newMachineContext(),
				logger:     m.logger,
				machineApi: api,
				id:         id,
			}

			go runMachine(machine, c, m.machineDead)
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

func runMachine(machine mutaterMachine, changed <-chan struct{}, died chan<- instancemutater.MutaterMachine) {
	defer func() {
		// We can't just send on the dead channel because the
		// central loop might be trying to write to us on the
		// changed channel.
		for {
			select {
			case died <- machine.machineApi:
				return
			case <-changed:
			}
		}
	}()

	profileChangeWatcher, err := machine.machineApi.WatchApplicationLXDProfiles()
	if err != nil {
		machine.logger.Errorf(errors.Annotatef(err, "failed to start watching application lxd profiles for machine-%s", machine.id).Error())
		machine.context.kill(err)
		return
	}
	if err := machine.context.add(profileChangeWatcher); err != nil {
		return
	}

	if err := machine.watchProfileChangesLoop(profileChangeWatcher); err != nil {
		machine.context.kill(err)
	}
}

func (m mutaterMachine) watchProfileChangesLoop(profileChangeWatcher watcher.NotifyWatcher) error {
	m.logger.Tracef("watching change on mutaterMachine %s", m.id)
	for {
		select {
		case <-m.context.dying():
			return m.context.errDying()
		case <-profileChangeWatcher.Changes():
			logger.Debugf("mutaterMachine-%s Received notification profile verification and change needed", m.id)

			info, err := m.machineApi.CharmProfilingInfo()
			if err != nil && !params.IsCodeNotProvisioned(err) {
				logger.Errorf("")
				continue
			}
			if err = m.processMachineProfileChanges(info); err != nil {
				return errors.Annotatef(err, "trigger worker restart, mutaterMachine-%s", m.id)
			}
		}
	}
}

func (m mutaterMachine) processMachineProfileChanges(info *instancemutater.UnitProfileInfo) error {
	m.logger.Tracef("%s.processMachineProfileChanges(%#v)", m.id, info)
	if len(info.CurrentProfiles) == 0 && len(info.ProfileChanges) == 0 {
		// no changes to be made, return now.
		return nil
	}

	expectedProfiles, post, err := m.gatherProfileData(info)
	if err != nil {
		return errors.Annotatef(err, "processMachineProfileChanges %s.gatherProfileData(info):", m.id)
	}
	for _, p := range post {
		if p.Profile != nil {
			expectedProfiles = append(expectedProfiles, p.Name)
		}
	}

	verified, err := m.verifyCurrentProfiles(string(info.InstanceId), expectedProfiles)
	if err != nil {
		return errors.Annotatef(err, "processMachineProfileChanges %s.verifyCurrentProfiles():", m.id)
	}
	if verified {
		m.logger.Tracef("no changes necessary to machine-%s lxd profiles", m.id)
		return nil
	}

	m.logger.Tracef("machine-%s (%s) assign profiles %q", m.id, string(info.InstanceId), expectedProfiles)
	broker := m.context.getBroker()
	currentProfiles, err := broker.AssignProfiles(string(info.InstanceId), expectedProfiles, post)
	if err != nil {
		return errors.Annotatef(err, "processMachineProfileChanges %s broker.AssignProfiles():", m.id)
	}

	return m.machineApi.SetCharmProfiles(currentProfiles)
}

func (m mutaterMachine) gatherProfileData(info *instancemutater.UnitProfileInfo) ([]string, []lxdprofile.ProfilePost, error) {
	expectedProfiles := []string{"default", "juju-" + info.ModelName}
	result := make([]lxdprofile.ProfilePost, len(info.ProfileChanges))
	for i, pu := range info.ProfileChanges {
		oldName, err := lxdprofile.MatchProfileNameByAppName(info.CurrentProfiles, pu.ApplicationName)
		if err != nil {
			return nil, nil, err
		}
		if pu.Profile.Empty() && oldName != "" {
			// There is no new Profile and no Profile for this application applied
			// already, move on.
			continue
		}
		name := lxdprofile.Name(info.ModelName, pu.ApplicationName, pu.Revision)
		result[i] = lxdprofile.ProfilePost{Name: name}
		if !pu.Profile.Empty() {
			result[i].Profile = &pu.Profile
			expectedProfiles = append(expectedProfiles, name)
		}
	}
	return expectedProfiles, result, nil
}

func (m mutaterMachine) verifyCurrentProfiles(instId string, expectedProfiles []string) (bool, error) {
	broker := m.context.getBroker()
	obtainedProfiles, err := broker.LXDProfileNames(instId)
	if err != nil {
		return false, nil
	}
	obtainedSet := set.NewStrings(obtainedProfiles...)

	if obtainedSet.Union(set.NewStrings(expectedProfiles...)).Size() > obtainedSet.Size() {
		return false, nil
	}

	for _, expected := range expectedProfiles {
		if !obtainedSet.Contains(expected) {
			return false, nil
		}
	}
	return true, nil
}
