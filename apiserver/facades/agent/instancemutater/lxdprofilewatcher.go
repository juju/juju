// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/state"
)

type machineLXDProfileWatcher struct {
	mu      sync.Mutex
	changes chan struct{}
	closed  bool

	initialized  chan struct{}
	applications map[string]appInfo // unit names for each application
	provisioned  bool
	machine      Machine

	backend InstanceMutaterState

	catacomb catacomb.Catacomb
}

// Kill is part of the worker.Worker interface.
func (w *machineLXDProfileWatcher) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	// The watcher must be dying or dead before we close the channel.
	// Otherwise readers could fail, but the watcher's catacomb would indicate
	// "still alive".
	w.catacomb.Kill(nil)
	w.closed = true
	close(w.changes)
}

// Wait is part of the worker.Worker interface.
func (w *machineLXDProfileWatcher) Wait() error {
	return w.catacomb.Wait()
}

// Stop is currently required by the Resources wrapper in the apiserver.
func (w *machineLXDProfileWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Err is part of the state.Watcher interface.
func (w *machineLXDProfileWatcher) Err() error {
	return w.catacomb.Err()
}

// Changes is part of the core watcher definition.
func (w *machineLXDProfileWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (w *machineLXDProfileWatcher) notify() {
	w.mu.Lock()

	if w.closed {
		w.mu.Unlock()
		return
	}

	select {
	case w.changes <- struct{}{}:
	default:
		// Already a pending change, so do nothing.
	}

	w.mu.Unlock()
}

type appInfo struct {
	charmURL     string
	charmProfile lxdprofile.Profile
	units        set.Strings
}

type MachineLXDProfileWatcherConfig struct {
	machine Machine
	backend InstanceMutaterState
}

func newMachineLXDProfileWatcher(config MachineLXDProfileWatcherConfig) (*machineLXDProfileWatcher, error) {
	w := &machineLXDProfileWatcher{
		// We use a single entry buffered channel for the changes.
		// This allows the config changed handler to send a value when there
		// is a change, but if that value hasn't been consumed before the
		// next change, the second change is discarded.
		changes:      make(chan struct{}, 1),
		initialized:  make(chan struct{}),
		applications: make(map[string]appInfo),
		machine:      config.machine,
		backend:      config.backend,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Annotate(err, "starting the lxd profile watcher worker")
	}

	// Send initial event down the channel. We know that this will
	// execute immediately because it is a buffered channel.
	w.changes <- struct{}{}

	if err := w.init(); err != nil {
		return nil, errors.Annotate(err, "seeding the lxd profile watcher cache")
	}
	close(w.initialized)

	logger.Debugf("started MachineLXDProfileWatcher for machine-%s with %#v", w.machine.Id(), w.applications)
	return w, nil
}

func (w *machineLXDProfileWatcher) loop() error {
	appWatcher := w.backend.WatchApplicationCharms()
	if err := w.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}
	charmWatcher := w.backend.WatchCharms()
	if err := w.catacomb.Add(charmWatcher); err != nil {
		return errors.Trace(err)
	}
	unitWatcher := w.backend.WatchUnits()
	if err := w.catacomb.Add(unitWatcher); err != nil {
		return errors.Trace(err)
	}
	instanceWatcher := w.machine.WatchInstanceData()
	if err := w.catacomb.Add(instanceWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case apps := <-appWatcher.Changes():
			logger.Tracef("application charm changes: %v", apps)
			for _, appName := range apps {
				if err := w.applicationCharmURLChange(appName); err != nil {
					return errors.Annotatef(err, "processing change for application %q", appName)
				}
			}
		case charms := <-charmWatcher.Changes():
			logger.Tracef("charm changes: %v", charms)
			for _, chURL := range charms {
				if err := w.charmChange(chURL); err != nil {
					return errors.Annotatef(err, "processing change for charm %q", chURL)
				}
			}
		case units := <-unitWatcher.Changes():
			logger.Debugf("unit changes on %v: %v", w.machine.Id(), units)
			for _, unitName := range units {
				u, err := w.backend.Unit(unitName)
				unitLife := state.Dead
				if err == nil {
					unitLife = u.Life()
				} else if !errors.IsNotFound(err) {
					return errors.Annotatef(err, "processing change for unit %q", unitName)
				}
				if unitLife == state.Dead {
					if err := w.removeUnit(unitName); err != nil {
						return errors.Annotatef(err, "processing change for unit %q", unitName)
					}
				} else {
					if err := w.addUnit(u); err != nil {
						return errors.Annotatef(err, "processing change for unit %q", unitName)
					}
				}
			}
		case <-instanceWatcher.Changes():
			id := w.machine.Id()
			logger.Tracef("instance changes machine-%s", id)
			if err := w.provisionedChange(); err != nil {
				return errors.Annotatef(err, "processing change for machine-%s", id)
			}
		}
	}
}

func (w *machineLXDProfileWatcher) unitMachineID(u Unit) (string, error) {
	principalName, isSubordinate := u.PrincipalName()
	machineID, err := u.AssignedMachineId()
	if err == nil || !errors.IsNotAssigned(err) {
		return machineID, errors.Trace(err)
	}
	if !isSubordinate {
		logger.Warningf("unit %s has no machine id, start watching when machine id assigned.", u.Name())
		return machineID, errors.Trace(err)
	}
	principal, err := w.backend.Unit(principalName)
	if errors.IsNotFound(err) {
		logger.Warningf("unit %s is subordinate, principal %s not found", u.Name(), principalName)
		return "", errors.NotFoundf("principal unit %q", principalName)
	} else if err != nil {
		return "", errors.Trace(err)
	}
	machineID, err = principal.AssignedMachineId()
	if errors.IsNotAssigned(err) {
		logger.Warningf("principal unit %s has no machine id, start watching when machine id assigned.", principalName)
	}
	return machineID, errors.Trace(err)
}

// init sets up the initial data used to determine when a notify occurs.
func (w *machineLXDProfileWatcher) init() error {
	units, err := w.machine.Units()
	if err != nil {
		return errors.Annotatef(err, "failed to get units to start machineLXDProfileWatcher")
	}

	for _, unit := range units {
		appName := unit.ApplicationName()
		unitName := unit.Name()

		if info, found := w.applications[appName]; found {
			info.units.Add(unitName)
			continue
		}
		app, err := unit.Application()
		if errors.IsNotFound(err) {
			// This is unlikely, but could happen because Units()
			// added the parent'd machine id to subordinates.
			// If the unit has no machineId, it will be added
			// to what is watched when the machineId is assigned.
			// Otherwise return an error.
			if _, err := unit.AssignedMachineId(); errors.IsNotAssigned(err) {
				logger.Warningf("unit %s has no application, nor machine id, start watching when machine id assigned.", unitName)
				continue
			} else if err != nil {
				return errors.Trace(err)
			}
		}

		cURL := app.CharmURL()
		info := appInfo{
			charmURL: *cURL,
			units:    set.NewStrings(unitName),
		}

		ch, err := w.backend.Charm(*cURL)
		if err != nil {
			return err
		}
		lxdProfile := ch.LXDProfile()
		if !lxdProfile.Empty() {
			info.charmProfile = lxdProfile
		}

		w.applications[appName] = info
	}
	return nil
}

// applicationCharmURLChange sends a notification if what is saved for its
// charm lxdprofile changes.  No notification is sent if the profile pointer
// begins and ends as nil.
func (w *machineLXDProfileWatcher) applicationCharmURLChange(appName string) error {
	// We don't want to respond to any events until we have been fully initialized.
	select {
	case <-w.initialized:
	case <-w.catacomb.Dying():
		return nil
	}

	var notify bool
	defer func(notify *bool) {
		if *notify {
			w.notify()
		}
	}(&notify)

	app, err := w.backend.Application(appName)
	if errors.IsNotFound(err) {
		logger.Debugf("not watching removed %s on machine-%s", appName, w.machine.Id())
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	info, ok := w.applications[appName]
	if ok {
		cURL := app.CharmURL()
		// Have we already seen this charm URL change?
		if info.charmURL == *cURL {
			return nil
		}
		ch, err := w.backend.Charm(*cURL)
		if errors.IsNotFound(err) {
			logger.Debugf("not watching %s with removed charm %s on machine-%s", appName, *cURL, w.machine.Id())
			return nil
		} else if err != nil {
			return errors.Annotatef(err, "error getting charm %s to evaluate for lxd profile notification", *cURL)
		}

		// notify if:
		// 1. the prior charm had a profile and the new one does not.
		// 2. the new profile is not empty.
		lxdProfile := ch.LXDProfile()
		if (!info.charmProfile.Empty() && lxdProfile.Empty()) || !lxdProfile.Empty() {
			logger.Debugf("notifying due to change of charm lxd profile for app %s, machine-%s", appName, w.machine.Id())
			notify = true
		} else {
			logger.Debugf("no notification of charm lxd profile needed for %s, machine-%s", appName, w.machine.Id())
		}

		info.charmProfile = lxdProfile
		info.charmURL = *cURL
		w.applications[appName] = info
	} else {
		logger.Tracef("not watching %s on machine-%s", appName, w.machine.Id())
	}
	logger.Tracef("end of application charm url change %#v", w.applications)
	return nil
}

// charmChange sends a notification if there is a mismatch in the lxd profile
// pointer for any application on the machine that references the charm URL
// included in the change. No notification is sent if the profile pointer
// begins and ends as nil.
func (w *machineLXDProfileWatcher) charmChange(chURL string) error {
	// We don't want to respond to any events until we have been fully initialized.
	select {
	case <-w.initialized:
	case <-w.catacomb.Dying():
		return nil
	}

	var notify bool
	defer func(notify *bool) {
		if *notify {
			w.notify()
		}
	}(&notify)

	// Scan the list of applications assigned to the machine and only
	// consider the ones whose charm URL matches the one we observed.
	//
	// In a typical scenario, the number of applications associated with
	// a machine will be small so we can simply iterate the list.
	for appName, info := range w.applications {
		if info.charmURL != chURL {
			continue
		}

		ch, err := w.backend.Charm(chURL)
		if errors.IsNotFound(err) {
			logger.Debugf("charm %s removed for %s on machine-%s", chURL, appName, w.machine.Id())
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
		lxdProfile := ch.LXDProfile()
		// notify if:
		// 1. the prior charm had a profile and the new one does not.
		// 2. the new profile is not empty.
		if (!info.charmProfile.Empty() && lxdProfile.Empty()) || !lxdProfile.Empty() {
			logger.Debugf("notifying due to change of charm lxd profile for charm %s, machine-%s", chURL, w.machine.Id())
			notify = true
		} else {
			logger.Debugf("no notification of charm lxd profile needed for %s, machine-%s", appName, w.machine.Id())
		}

		info.charmProfile = lxdProfile
		info.charmURL = chURL
		w.applications[appName] = info
	}
	logger.Tracef("end of charm metadata change")
	return nil
}

// addUnit modifies the map of applications being watched when a unit is
// added to the machine.  Notification is sent if a new unit whose charm has
// an lxd profile is added.
func (w *machineLXDProfileWatcher) addUnit(unit Unit) error {
	// We don't want to respond to any events until we have been fully initialized.
	select {
	case <-w.initialized:
	case <-w.catacomb.Dying():
		return nil
	}

	var notify bool
	defer func(notify *bool) {
		if *notify {
			logger.Debugf("notifying due to add unit requires lxd profile change machine-%s", w.machine.Id())
			w.notify()
		}
	}(&notify)

	unitName := unit.Name()
	unitMachineId, err := w.unitMachineID(unit)
	if errors.IsNotAssigned(err) || errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "finding assigned machine for unit %q", unitName)
	}
	if unitMachineId != w.machine.Id() {
		logger.Debugf("ignoring unit change on machine-%s as it is not machine-%s", unitMachineId, w.machine.Id())
		return nil
	}
	logger.Debugf("start watching %q on machine-%s", unitName, w.machine.Id())
	notify, err = w.add(unit)
	if err != nil {
		return errors.Trace(err)
	}

	logger.Tracef("end of unit change %#v", w.applications)
	return nil
}

func (w *machineLXDProfileWatcher) add(unit Unit) (bool, error) {
	unitName := unit.Name()
	appName := unit.ApplicationName()

	_, ok := w.applications[appName]
	if !ok {
		curl := unit.CharmURL()
		if curl == nil {
			// this happens for new units to existing machines.
			app, err := unit.Application()
			if errors.IsNotFound(err) {
				logger.Debugf("failed to process new unit %s for %s on machine-%s; application removed", unitName, appName, w.machine.Id())
				return false, nil
			} else if err != nil {
				return false, errors.Annotatef(err, "failed to get application %s for machine-%s", appName, w.machine.Id())
			}
			curl = app.CharmURL()
		}

		ch, err := w.backend.Charm(*curl)
		if errors.IsNotFound(err) {
			logger.Debugf("charm %s removed for %s on machine-%s", *curl, unitName, w.machine.Id())
			return false, nil
		} else if err != nil {
			return false, errors.Annotatef(err, "failed to get charm %q for %s on machine-%s", *curl, appName, w.machine.Id())
		}
		info := appInfo{
			charmURL: *curl,
			units:    set.NewStrings(unitName),
		}

		lxdProfile := ch.LXDProfile()
		if !lxdProfile.Empty() {
			info.charmProfile = lxdprofile.Profile{
				Config:      lxdProfile.Config,
				Description: lxdProfile.Description,
				Devices:     lxdProfile.Devices,
			}
		}
		w.applications[appName] = info
	} else {
		if w.applications[appName].units.Contains(unitName) {
			return false, nil
		}
		w.applications[appName].units.Add(unitName)
	}
	if !w.applications[appName].charmProfile.Empty() {
		return true, nil
	}
	return false, nil
}

// removeUnit modifies the map of applications being watched when a unit is
// removed from the machine.  Notification is sent if a unit being removed
// has a profile and other units exist on the machine.
func (w *machineLXDProfileWatcher) removeUnit(unitName string) error {
	// We don't want to respond to any events until we have been fully initialized.
	select {
	case <-w.initialized:
	case <-w.catacomb.Dying():
		return nil
	}
	var notify bool
	defer func(notify *bool) {
		if *notify {
			logger.Debugf("notifying due to remove unit requires lxd profile change machine-%s", w.machine.Id())
			w.notify()
		}
	}(&notify)

	appName, err := names.UnitApplication(unitName)
	if err != nil {
		return errors.Trace(err)
	}
	app, ok := w.applications[appName]
	if !ok {
		// Previously this was a logError with a programming error, but in
		// actual fact you can bump into this more often than not in legitimate
		// circumstances. So instead of being an error, this should just be a
		// debug log.
		logger.Debugf("unit removed before being added, application name not found")
		return nil
	}
	if !app.units.Contains(unitName) {
		return nil
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
		notify = true
	}
	return nil
}

// provisionedChanged notifies when called.  Topic subscribed to is specific to
// this machine.
func (w *machineLXDProfileWatcher) provisionedChange() error {
	// We don't want to respond to any events until we have been fully initialized.
	select {
	case <-w.initialized:
	case <-w.catacomb.Dying():
		return nil
	}

	if w.provisioned {
		return nil
	}

	m, err := w.backend.Machine(w.machine.Id())
	if err != nil {
		return err
	}
	_, err = m.InstanceId()
	if errors.IsNotProvisioned(err) {
		logger.Debugf("machine-%s not provisioned yet", w.machine.Id())
		return nil
	} else if err != nil {
		logger.Criticalf("%q.provisionedChange error getting instanceID: %s", w.machine.Id(), err)
		return err
	}
	w.provisioned = true

	logger.Debugf("notifying due to machine-%s now provisioned", w.machine.Id())
	w.notify()
	return nil
}
