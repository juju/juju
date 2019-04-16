// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"

	"github.com/juju/collections/set"
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
	appTopic     string
	unitTopic    string
	machineId    string
	applications map[string]appInfo
	modeler      MachineAppModeler
	metrics      *ControllerGauges
	hub          *pubsub.SimpleHub
}

func newMachineAppLXDProfileWatcher(config MachineAppLXDProfileConfig) *MachineAppLXDProfileWatcher {
	w := &MachineAppLXDProfileWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),
		applications:      config.applications,
		modeler:           config.modeler,
		machineId:         config.machineId,
		metrics:           config.metrics,
	}

	unsubApp := config.hub.Subscribe(config.appTopic, w.applicationCharmURLChange)
	unsubUnit := config.hub.Subscribe(config.unitTopic, w.unitChange)
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsubApp()
		unsubUnit()
		return nil
	})

	logger.Tracef("started MachineAppLXDProfileWatcher for machine-%s with %#v", config.machineId, config.applications)
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
		if lxdProfile.Empty() {
			info.charmProfile = lxdprofile.Profile{}
		} else {
			info.charmProfile = lxdProfile
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
			w.metrics.LXDProfileChangeHit.Inc()
		} else {
			w.metrics.LXDProfileChangeMiss.Inc()
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
		case unit.MachineId() == "" && !unit.Subordinate():
			logger.Tracef("%s has no machineId and not a sub", unit.Name())
			return
		case unit.Subordinate():
			principal, err := w.modeler.Unit(unit.Principal())
			if err != nil {
				logger.Tracef("unit %s is subordinate, principal %s not found", unit.Name(), principal)
				return
			}
			if w.machineId != principal.MachineId() {
				logger.Tracef("watching unit changes on machine-%s not machine-%s", w.machineId, unit.MachineId())
				return
			}
		case w.machineId != unit.MachineId():
			logger.Tracef("watching unit changes on machine-%s not machine-%s", w.machineId, unit.MachineId())
			return
		}
		logger.Tracef("start watching %q on machine-%s", unit.Name(), w.machineId)
		notify = w.addUnit(unit)
	default:
		w.logError("programming error, value not of type *Unit or []string")
	}
	logger.Debugf("end of unit change %#v", w.applications)
}

func (w *MachineAppLXDProfileWatcher) addUnit(unit *Unit) bool {
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

func (w *MachineAppLXDProfileWatcher) removeUnit(names []string) bool {
	if len(names) != 2 {
		w.logError("programming error, 2 values not provided")
		return false
	}
	unitName, appName := names[0], names[1]
	app, ok := w.applications[appName]
	if !ok {
		w.logError("programming error, unit removed before being added, application name not found")
		return false
	}
	if !app.units.Contains(unitName) {
		w.logError("unit not being watched for machine")
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
	if len(w.applications) > 0 && !profile.Empty() {
		return true
	}
	return false
}

func (w *MachineAppLXDProfileWatcher) logError(msg string) {
	logger.Errorf(msg)
	w.metrics.LXDProfileChangeError.Inc()
}
