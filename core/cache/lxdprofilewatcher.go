// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/collections/set"
	"github.com/juju/pubsub"

	"github.com/juju/juju/core/lxdprofile"
)

type MachineAppLXDProfileWatcher struct {
	*notifyWatcherBase

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
	charmProfile *lxdprofile.Profile
	units        set.Strings
}

func newMachineAppLXDProfileWatcher(
	appTopic, unitTopic, machineId string,
	applications map[string]appInfo,
	modeler MachineAppModeler,
	hub *pubsub.SimpleHub,
) *MachineAppLXDProfileWatcher {
	w := &MachineAppLXDProfileWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),
		applications:      applications,
		modeler:           modeler,
		machineId:         machineId,
	}

	unsubApp := hub.Subscribe(appTopic, w.applicationCharmURLChange)
	unsubUnit := hub.Subscribe(unitTopic, w.unitChange)
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsubApp()
		unsubUnit()
		return nil
	})

	logger.Tracef("started MachineAppLXDProfileWatcher for machine-%s with %#v", machineId, applications)
	return w
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
		}
	}(&notify)

	values, ok := value.(appCharmUrlChange)
	if !ok {
		logger.Errorf("programming error, value not of type appCharmUrlChange")
		return
	}
	appName, chURL := values.appName, values.chURL
	info, ok := w.applications[appName]
	if ok {
		ch, err := w.modeler.Charm(chURL)
		if err != nil {
			logger.Errorf("error getting charm %s to evaluate for lxd profile notification: %s", chURL, err)
			return
		}
		// notify if:
		// 1. the prior charm had a profile and the new one does not.
		// 2. the new profile is not empty.
		if (info.charmProfile != nil && ch.details.LXDProfile.Empty()) ||
			!ch.details.LXDProfile.Empty() {
			logger.Tracef("notifying due to change of charm lxd profile for %s, machine-%s", appName, w.machineId)
			notify = true
		} else {
			logger.Tracef("no notification of charm lxd profile needed for %s, machine-%s", appName, w.machineId)
		}
		if ch.details.LXDProfile.Empty() {
			info.charmProfile = nil
		} else {
			info.charmProfile = &ch.details.LXDProfile
		}
		info.charmURL = chURL
		w.applications[appName] = info
	} else {
		logger.Tracef("not watching %s on machine-%s", appName, w.machineId)
	}
	logger.Tracef("end of application charm url change %#v", w.applications)
}

// unitChange modifies the map of applications being watched when a unit is
// added or removed from the machine.  Notification is sent if:
//     1. A new unit whose charm has an lxd profile is added.
//     2. A unit being removed has a profile and other units
//        exist on the machine.
func (w *MachineAppLXDProfileWatcher) unitChange(topic string, value interface{}) {
	w.notifyWatcherBase.mu.Lock()
	var notify bool
	defer func(notify *bool) {
		w.notifyWatcherBase.mu.Unlock()
		if *notify {
			logger.Tracef("notifying due to add/remove unit requires lxd profile change machine-%s", w.machineId)
			w.notify()
		}
	}(&notify)

	names, okString := value.([]string)
	unit, okUnit := value.(*Unit)
	switch {
	case okString:
		logger.Tracef("stop watching %q on machine-%s", names, w.machineId)
		notify = w.removeUnit(names)
	case okUnit:
		switch {
		case unit.details.MachineId == "" && !unit.details.Subordinate:
			logger.Tracef("%s has no machineId and not a sub", unit.details.Name)
			return
		case unit.details.Subordinate:
			principal, err := w.modeler.Unit(unit.details.Principal)
			if err != nil {
				logger.Tracef("unit %s is subordinate, principal %s not found", unit.details.Name, unit.details.Principal)
				return
			}
			if w.machineId != principal.details.MachineId {
				logger.Tracef("watching unit changes on machine-%s not machine-%s", w.machineId, unit.details.MachineId)
				return
			}
		case w.machineId != unit.details.MachineId:
			logger.Tracef("watching unit changes on machine-%s not machine-%s", w.machineId, unit.details.MachineId)
			return
		}
		logger.Tracef("start watching %q on machine-%s", unit.details.Name, w.machineId)
		notify = w.addUnit(unit)
	default:
		logger.Errorf("programming error, value not of type *Unit or []string")
	}
	logger.Tracef("end of unit change %#v", w.applications)
}

func (w *MachineAppLXDProfileWatcher) addUnit(unit *Unit) bool {
	_, ok := w.applications[unit.details.Application]
	if !ok {
		curl := unit.details.CharmURL
		if curl == "" {
			// this happens for new units to existing machines.
			app, err := w.modeler.Application(unit.details.Application)
			if err != nil {
				logger.Errorf("failed to get application %s for machine-%s", unit.details.Application, w.machineId)
				return false
			}
			curl = app.details.CharmURL
		}
		ch, err := w.modeler.Charm(curl)
		if err != nil {
			logger.Errorf("failed to get charm %q for %s on machine-%s", curl, unit.details.Name, w.machineId)
			return false
		}
		info := appInfo{
			charmURL: curl,
			units:    set.NewStrings(unit.details.Name),
		}
		if !ch.details.LXDProfile.Empty() {
			info.charmProfile = &ch.details.LXDProfile
		}
		w.applications[unit.details.Application] = info
	} else {
		w.applications[unit.details.Application].units.Add(unit.details.Name)
	}
	if w.applications[unit.details.Application].charmProfile != nil {
		return true
	}
	return false
}

func (w *MachineAppLXDProfileWatcher) removeUnit(names []string) bool {
	if len(names) != 2 {
		logger.Errorf("programming error, 2 values not provided")
		return false
	}
	unitName, appName := names[0], names[1]
	app, ok := w.applications[appName]
	if !ok {
		logger.Errorf("programming error, unit removed before being added, application name not found")
		return false
	}
	if !app.units.Contains(unitName) {
		logger.Errorf("unit not being watched for machine")
		return false
	}
	profile := app.charmProfile
	app.units.Remove(unitName)
	if app.units.Size() == 0 {
		// the application has no more units on this machine,
		// stop watching it.
		delete(w.applications, appName)
	}
	// If there are additional units on the machine and the current
	// application has an lxd profile, notify so it can be removed
	// from the machine.
	if len(w.applications) > 0 && profile != nil && !profile.Empty() {
		return true
	}
	return false
}
