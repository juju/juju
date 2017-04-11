// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// IngressAddressWatcher reports changes to addresses
// for local units in a given relation.
// Each event contains the entire set of addresses which
// are required for ingress for the relation.
type IngressAddressWatcher struct {
	catacomb catacomb.Catacomb

	backend State
	appName string
	rel     Relation

	out chan []string

	// Channel for machineAddressWatchers to report individual machine
	// updates.
	addressChanges  chan string
	addressWatchers map[string]*machineAddressWatcher

	// A map of unit names by machine id.
	machineToUnits map[string]set.Strings
	// A map of machine id by unit name - this is needed because we
	// might not be able to retrieve the machine name when a unit
	// leaves scope if it's been completely removed by the time we look.
	unitToMachine map[string]string

	// A map of known unit addresses, keyed on unit name.
	known map[string]string
}

// NewIngressAddressWatcher creates an IngressAddressWatcher.
func NewIngressAddressWatcher(backend State, rel Relation, appName string) (*IngressAddressWatcher, error) {
	w := &IngressAddressWatcher{
		backend:         backend,
		appName:         appName,
		rel:             rel,
		known:           make(map[string]string),
		out:             make(chan []string),
		addressChanges:  make(chan string),
		addressWatchers: make(map[string]*machineAddressWatcher),
		machineToUnits:  make(map[string]set.Strings),
		unitToMachine:   make(map[string]string),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

func (w *IngressAddressWatcher) initial() (set.Strings, error) {
	app, err := w.backend.Application(w.appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	units, err := app.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(set.Strings)
	for _, u := range units {
		inScope, err := w.rel.UnitInScope(u)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !inScope {
			continue
		}

		addr, ok, err := w.unitAddress(u)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if ok {
			result.Add(addr)
		}
		if err := w.trackUnit(u); err != nil {
			return nil, errors.Trace(err)
		}

	}
	return result, nil
}

func (w *IngressAddressWatcher) loop() error {
	defer close(w.out)
	ruw, err := w.rel.WatchUnits(w.appName)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(ruw); err != nil {
		return errors.Trace(err)
	}

	var (
		sentInitial bool
		out         chan<- []string
	)
	// addresses holds the current set of known addresses.
	addresses, err := w.initial()
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	out = w.out
	for {
		newAddresses := addresses
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		// Send initial event or subsequent changes.
		case out <- formatAsCIDR(addresses.Values()):
			sentInitial = true
			out = nil
		case c, ok := <-ruw.Changes():
			if !ok {
				return watcher.EnsureErr(ruw)
			}
			// A unit has entered or left scope.
			// Get the new set of addresses resulting from that
			// change, and if different to what we know, send the change.
			newAddresses, err = w.mergeAddresses(addresses, c)
			if err != nil {
				return err
			}
		case machineId, ok := <-w.addressChanges:
			if !ok {
				return errors.Errorf("address changes channel unexpectedly closed")
			}
			changed, err := w.machineAddressChanged(machineId)
			if err != nil {
				return errors.Trace(err)
			}
			if changed {
				newAddresses = set.NewStrings()
				for _, addr := range w.known {
					newAddresses.Add(addr)
				}
			}
		}

		changed := areDifferent(newAddresses, addresses)
		if !sentInitial || changed {
			addresses = newAddresses
			out = w.out
		} else {
			out = nil
		}
	}
}

func areDifferent(s1, s2 set.Strings) bool {
	return !s1.Difference(s2).IsEmpty() || !s2.Difference(s1).IsEmpty()
}

func formatAsCIDR(addresses []string) []string {
	result := make([]string, len(addresses))
	for i, a := range addresses {
		result[i] = a + "/32"
	}
	return result
}

func (w *IngressAddressWatcher) unitAddress(unit Unit) (string, bool, error) {
	addr, err := unit.PublicAddress()
	if errors.IsNotAssigned(err) {
		logger.Debugf("unit %s is not assigned to a machine, can't get address", unit.Name())
		return "", false, nil
	}
	if network.IsNoAddressError(err) {
		logger.Debugf("unit %s has no public address", unit.Name())
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	logger.Debugf("unit %q has public address %q", unit.Name(), addr.Value)
	return addr.Value, true, nil
}

func (w *IngressAddressWatcher) mergeAddresses(addresses set.Strings, c params.RelationUnitsChange) (set.Strings, error) {
	result := set.NewStrings(addresses.Values()...)
	for name := range c.Changed {

		u, err := w.backend.Unit(name)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return result, err
		}

		if err := w.trackUnit(u); err != nil {
			return result, errors.Trace(err)
		}

		// We need to know whether to look at the public or cloud local address.
		// For now, we'll use the public address and later if needed use a watcher
		// parameter to look at the cloud local address.
		addr, ok, err := w.unitAddress(u)
		if err != nil {
			return result, err
		}
		if !ok {
			continue
		}
		result.Add(addr)
		w.known[name] = addr
	}
	for _, name := range c.Departed {
		if err := w.untrackUnit(name); err != nil {
			return result, errors.Trace(err)
		}
		// If the unit is departing and we have seen its address,
		// remove the address.
		address := w.known[name]
		delete(w.known, name)

		// See if the address is still used by another unit.
		inUse := false
		for unit, addr := range w.known {
			if name != unit && addr == address {
				inUse = true
				break
			}
		}
		if !inUse {
			result.Remove(address)
		}
	}
	return result, nil
}

func (w *IngressAddressWatcher) trackUnit(unit Unit) error {
	machine, err := w.assignedMachine(unit)
	if errors.IsNotAssigned(err) {
		logger.Errorf("unit %q entered scope without a machine assigned - addresses will not be tracked", unit)
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	if units, ok := w.machineToUnits[machine.Id()]; !ok {
		w.machineToUnits[machine.Id()] = set.NewStrings(unit.Name())
	} else {
		units.Add(unit.Name())
	}
	if _, ok := w.addressWatchers[machine.Id()]; ok {
		// We're already watching this machine.
		return nil
	}
	addressWatcher, err := newMachineAddressWatcher(machine, w.addressChanges)
	if err != nil {
		return errors.Trace(err)
	}
	err = w.catacomb.Add(addressWatcher)
	if err != nil {
		return errors.Trace(err)
	}

	w.addressWatchers[machine.Id()] = addressWatcher
	w.unitToMachine[unit.Name()] = machine.Id()
	return nil
}

func (w *IngressAddressWatcher) untrackUnit(unitName string) error {
	machineId, ok := w.unitToMachine[unitName]
	if !ok {
		logger.Errorf("missing machine id for unit %q", unitName)
		return nil
	}
	units, ok := w.machineToUnits[machineId]
	if !ok {
		// This shouldn't happen.
		return errors.Errorf("missing unit-set for machine %q (hosting unit %q)", machineId, unitName)
	}
	units.Remove(unitName)
	if units.Size() > 0 {
		// No need to stop the watcher - there are still units on the
		// machine.
		return nil
	}

	// Clean up this machine's watcher and units entries.
	watcher, ok := w.addressWatchers[machineId]
	if !ok {
		return errors.Errorf("missing watcher for machine %q (hosting unit %q)", machineId, unitName)
	}
	err := worker.Stop(watcher)
	if err != nil {
		return errors.Trace(err)
	}
	delete(w.addressWatchers, machineId)
	delete(w.machineToUnits, machineId)
	delete(w.unitToMachine, unitName)
	return nil
}

func (w *IngressAddressWatcher) assignedMachine(unit Unit) (Machine, error) {
	machineId, err := unit.AssignedMachineId()
	if err != nil {
		return nil, errors.Trace(err)
	}
	machine, err := w.backend.Machine(machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return machine, nil
}

func (w *IngressAddressWatcher) machineAddressChanged(machineId string) (bool, error) {
	unitNames, ok := w.machineToUnits[machineId]
	if !ok {
		return false, errors.Errorf("missing unit-set for machine %q", machineId)
	}
	changed := false
	for unitName := range unitNames {
		unit, err := w.backend.Unit(unitName)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return false, errors.Trace(err)
		}
		address, _, err := w.unitAddress(unit)
		if err != nil {
			return false, errors.Trace(err)
		}
		existingAddress := w.known[unitName]
		if existingAddress != address {
			w.known[unitName] = address
			changed = true
		}
	}
	return changed, nil
}

// Changes returns the event channel for this watcher.
func (w *IngressAddressWatcher) Changes() <-chan []string {
	return w.out
}

// Kill asks the watcher to stop without waiting for it do so.
func (w *IngressAddressWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *IngressAddressWatcher) Wait() error {
	return w.catacomb.Wait()
}

// Stop kills the watcher, then waits for it to die.
func (w *IngressAddressWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Err returns any error encountered while the watcher
// has been running.
func (w *IngressAddressWatcher) Err() error {
	return w.catacomb.Err()
}

func newMachineAddressWatcher(machine Machine, dest chan<- string) (*machineAddressWatcher, error) {
	w := &machineAddressWatcher{
		machine: machine,
		dest:    dest,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

// machineAddressWatcher watches for machine address changes and
// notifies the dest channel when it sees them. It isn't a watcher in
// the strict sense since it doesn't have a Changes method.
type machineAddressWatcher struct {
	catacomb catacomb.Catacomb
	machine  Machine
	dest     chan<- string
}

func (w *machineAddressWatcher) loop() error {
	aw := w.machine.WatchAddresses()
	if err := w.catacomb.Add(aw); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-aw.Changes():
			w.dest <- w.machine.Id()
		}
	}
}

func (w *machineAddressWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *machineAddressWatcher) Wait() error {
	return w.catacomb.Wait()
}
