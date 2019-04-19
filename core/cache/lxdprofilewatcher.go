// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/pubsub"

	"github.com/juju/juju/core/lxdprofile"
)

type MachineAppLXDProfileWatcher struct {
	*notifyWatcherBase

	metrics *ControllerGauges

	applications map[string]appInfo // unit names for each application
	machineId    string

	modeler MachineAppModeler
}

// MachineAppModeler is a point of use model for MachineAppLXDProfileWatcher to
// get Applications, Charms and Units.
type MachineAppModeler interface {
	Application(string) (*Application, error)
	Charm(string) (*Charm, error)
	Unit(string) (*Unit, error)
}

type appInfo struct {
	charmURL     string
	charmProfile lxdprofile.Profile
	units        set.Strings
}

type MachineAppLXDProfileConfig struct {
	appTopic        string
	unitAddTopic    string
	unitRemoveTopic string
	machine         *Machine
	modeler         MachineAppModeler
	metrics         *ControllerGauges
	hub             *pubsub.SimpleHub
	resident        *Resident
}

func newMachineAppLXDProfileWatcher(config MachineAppLXDProfileConfig) (*MachineAppLXDProfileWatcher, error) {
	w := &MachineAppLXDProfileWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),
		modeler:           config.modeler,
		metrics:           config.metrics,
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	deregister := config.resident.registerWorker(w)
	unsubApp := config.hub.Subscribe(config.appTopic, w.applicationCharmURLChange)
	unsubUnitAdd := config.hub.Subscribe(config.unitAddTopic, w.addUnit)
	unsubUnitRemove := config.hub.Subscribe(config.unitRemoveTopic, w.removeUnit)
	unsubFns := func() {
		unsubUnitRemove()
		unsubUnitAdd()
		unsubApp()
		deregister()
	}

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsubFns()
		return nil
	})

	err := w.init(config.machine)
	if err != nil {
		unsubFns()
		return nil, errors.Trace(err)
	}

	logger.Tracef("started MachineAppLXDProfileWatcher for machine-%s with %#v", w.machineId, w.applications)
	return w, nil
}

// init sets up the initial data used to determine when a notify occurs.
func (w *MachineAppLXDProfileWatcher) init(machine *Machine) error {
	units, err := machine.Units()
	if err != nil {
		return errors.Annotatef(err, "failed to get units to start MachineAppLXDProfileWatcher")
	}

	applications := make(map[string]appInfo)
	for _, unit := range units {
		appName := unit.Application()
		unitName := unit.Name()
		_, found := applications[appName]
		if found {
			applications[appName].units.Add(unitName)
			continue
		}

		app, err := w.modeler.Application(appName)
		if errors.IsNotFound(err) {
			// This is unlikely, but could happen because Units()
			// added the parent'd machine id to subordinates.
			// If the unit has no machineId, it will be added
			// to what is watched when the machineId is assigned.
			// Otherwise return an error.
			if unit.MachineId() != "" {
				return errors.Errorf("programming error, unit %s has machineId but not application", unitName)
			}
			logger.Errorf("unit %s has no application, nor machine id, start watching when machine id assigned.", unitName)
			w.metrics.LXDProfileChangeError.Inc()
			continue
		}

		chURL := app.CharmURL()
		info := appInfo{
			charmURL: chURL,
			units:    set.NewStrings(unitName),
		}

		ch, err := w.modeler.Charm(chURL)
		if err != nil {
			return err
		}
		lxdProfile := ch.LXDProfile()
		if !lxdProfile.Empty() {
			info.charmProfile = lxdProfile
		}

		applications[appName] = info
	}

	w.applications = applications
	w.machineId = machine.Id()
	return nil
}

// applicationCharmURLChange sends a notification if what is saved for its
// charm lxdprofile changes.  No notification is sent if the profile pointer
// begins and ends as nil.
func (w *MachineAppLXDProfileWatcher) applicationCharmURLChange(topic string, value interface{}) {
	w.notifyWatcherBase.mu.Lock()
	var notify bool
	defer func(notify *bool) {
		w.notifyWatcherBase.mu.Unlock()
		if *notify {
			w.notify()
			w.metrics.LXDProfileChangeHit.Inc()
		} else {
			w.metrics.LXDProfileChangeMiss.Inc()
		}
	}(&notify)

	values, ok := value.(appCharmUrlChange)
	if !ok {
		w.logError("programming error, value not of type appCharmUrlChange")
		return
	}
	appName, chURL := values.appName, values.chURL
	info, ok := w.applications[appName]
	if ok {
		ch, err := w.modeler.Charm(chURL)
		if err != nil {
			w.logError(fmt.Sprintf("error getting charm %s to evaluate for lxd profile notification: %s", chURL, err))
			return
		}

		// notify if:
		// 1. the prior charm had a profile and the new one does not.
		// 2. the new profile is not empty.
		lxdProfile := ch.LXDProfile()
		if (!info.charmProfile.Empty() && lxdProfile.Empty()) || !lxdProfile.Empty() {
			logger.Tracef("notifying due to change of charm lxd profile for %s, machine-%s", appName, w.machineId)
			notify = true
		} else {
			logger.Tracef("no notification of charm lxd profile needed for %s, machine-%s", appName, w.machineId)
		}

		info.charmProfile = lxdProfile
		info.charmURL = chURL
		w.applications[appName] = info
	} else {
		logger.Tracef("not watching %s on machine-%s", appName, w.machineId)
	}
	logger.Tracef("end of application charm url change %#v", w.applications)
}

// addUnit modifies the map of applications being watched when a unit is
// added to the machine.  Notification is sent if a new unit whose charm has
// an lxd profile is added.
func (w *MachineAppLXDProfileWatcher) addUnit(topic string, value interface{}) {
	w.notifyWatcherBase.mu.Lock()
	var notify bool
	defer func(notify *bool) {
		w.notifyWatcherBase.mu.Unlock()
		if *notify {
			logger.Tracef("notifying due to add unit requires lxd profile change machine-%s", w.machineId)
			w.notify()
			w.metrics.LXDProfileChangeHit.Inc()
		} else {
			w.metrics.LXDProfileChangeMiss.Inc()
		}
	}(&notify)

	unit, okUnit := value.(*Unit)
	if !okUnit {
		w.logError("programming error, value not of type *Unit")
		return
	}
	isSubordinate := unit.Subordinate()
	unitMachineId := unit.MachineId()
	unitName := unit.Name()

	switch {
	case unitMachineId == "" && !isSubordinate:
		logger.Tracef("%s has no machineId and not a sub", unitName)
		return
	case isSubordinate:
		principal, err := w.modeler.Unit(unit.Principal())
		if err != nil {
			logger.Tracef("unit %s is subordinate, principal %s not found", unitName, unit.Principal())
			return
		}
		if w.machineId != principal.MachineId() {
			logger.Tracef("watching unit changes on machine-%s not machine-%s", w.machineId, unitMachineId)
			return
		}
	case w.machineId != unitMachineId:
		logger.Tracef("watching unit changes on machine-%s not machine-%s", w.machineId, unitMachineId)
		return
	}
	logger.Tracef("start watching %q on machine-%s", unitName, w.machineId)
	notify = w.add(unit)

	logger.Debugf("end of unit change %#v", w.applications)
}

func (w *MachineAppLXDProfileWatcher) add(unit *Unit) bool {
	unitName := unit.Name()
	appName := unit.Application()

	_, ok := w.applications[appName]
	if !ok {
		curl := unit.CharmURL()
		if curl == "" {
			// this happens for new units to existing machines.
			app, err := w.modeler.Application(appName)
			if err != nil {
				w.logError(fmt.Sprintf("failed to get application %s for machine-%s", appName, w.machineId))
				return false
			}
			curl = app.CharmURL()
		}
		ch, err := w.modeler.Charm(curl)
		if err != nil {
			w.logError(fmt.Sprintf("failed to get charm %q for %s on machine-%s", curl, appName, w.machineId))
			return false
		}
		info := appInfo{
			charmURL: curl,
			units:    set.NewStrings(unitName),
		}

		lxdProfile := ch.LXDProfile()
		if !lxdProfile.Empty() {
			info.charmProfile = lxdProfile
		}
		w.applications[appName] = info
	} else {
		w.applications[appName].units.Add(unitName)
	}
	if !w.applications[appName].charmProfile.Empty() {
		return true
	}
	return false
}

// removeUnit modifies the map of applications being watched when a unit is
// removed from the machine.  Notification is sent if a unit being removed
// has a profile and other units exist on the machine.
func (w *MachineAppLXDProfileWatcher) removeUnit(topic string, value interface{}) {
	w.notifyWatcherBase.mu.Lock()
	var notify bool
	defer func(notify *bool) {
		w.notifyWatcherBase.mu.Unlock()
		if *notify {
			logger.Tracef("notifying due to remove unit requires lxd profile change machine-%s", w.machineId)
			w.notify()
			w.metrics.LXDProfileChangeHit.Inc()
		} else {
			w.metrics.LXDProfileChangeMiss.Inc()
		}
	}(&notify)

	rUnit, ok := value.(unitLXDProfileRemove)
	if !ok {
		w.logError("programming error, value not of type unitLXDProfileRemove")
		return
	}

	app, ok := w.applications[rUnit.appName]
	if !ok {
		w.logError("programming error, unit removed before being added, application name not found")
		return
	}
	if !app.units.Contains(rUnit.name) {
		return
	}
	profile := app.charmProfile
	app.units.Remove(rUnit.name)
	if app.units.Size() == 0 {
		// the application has no more units on this machine,
		// stop watching it.
		delete(w.applications, rUnit.appName)
	}
	// If there are additional units on the machine and the current
	// application has an lxd profile, notify so it can be removed
	// from the machine.
	if len(w.applications) > 0 && !profile.Empty() {
		notify = true
	}
	return
}

func (w *MachineAppLXDProfileWatcher) logError(msg string) {
	logger.Errorf(msg)
	w.metrics.LXDProfileChangeError.Inc()
}
