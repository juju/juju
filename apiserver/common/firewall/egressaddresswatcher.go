// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// EgressAddressWatcher reports changes to addresses
// for local units in a given relation.
// Each event contains the entire set of addresses which
// are required for ingress on the remote side of the relation.
type EgressAddressWatcher struct {
	catacomb catacomb.Catacomb

	backend State
	appName string
	rel     Relation

	out chan []string

	// Channel for machineAddressWatchers to report individual machine
	// updates.
	addressChanges chan string

	// A map of machine id to machine data.
	machines map[string]*machineData

	// A map of machine id by unit name - this is needed because we
	// might not be able to retrieve the machine name when a unit
	// leaves scope if it's been completely removed by the time we look.
	unitToMachine map[string]string

	// A map of known unit addresses, keyed on unit name.
	known map[string]string

	// A set of known egress cidrs for the model.
	knownModelEgress set.Strings

	// A set of known egress cidrs for the relation.
	knownRelationEgress set.Strings
}

// machineData holds the information we track at the machine level.
type machineData struct {
	units  set.Strings
	worker *machineAddressWorker
}

// NewEgressAddressWatcher creates an EgressAddressWatcher.
func NewEgressAddressWatcher(backend State, rel Relation, appName string) (*EgressAddressWatcher, error) {
	w := &EgressAddressWatcher{
		backend:        backend,
		appName:        appName,
		rel:            rel,
		known:          make(map[string]string),
		out:            make(chan []string),
		addressChanges: make(chan string),
		machines:       make(map[string]*machineData),
		unitToMachine:  make(map[string]string),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

func (w *EgressAddressWatcher) initialise() error {
	app, err := w.backend.Application(w.appName)
	if err != nil {
		return errors.Trace(err)
	}
	units, err := app.AllUnits()
	if err != nil {
		return errors.Trace(err)
	}
	for _, u := range units {
		inScope, err := w.rel.UnitInScope(u)
		if err != nil {
			return errors.Trace(err)
		}
		if !inScope {
			continue
		}

		addr, ok, err := w.unitAddress(u)
		if err != nil {
			return errors.Trace(err)
		}
		if ok {
			w.known[u.Name()] = addr
		}
		if err := w.trackUnit(u); err != nil {
			return errors.Trace(err)
		}
	}
	cfg, err := w.backend.ModelConfig()
	if err != nil {
		return err
	}
	w.knownModelEgress = set.NewStrings(cfg.EgressSubnets()...)
	return nil
}

func (w *EgressAddressWatcher) loop() error {
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

	// TODO(wallyworld) - we just want to watch for egress
	// address changes but right now can only watch for
	// any model config change.
	mw := w.backend.WatchForModelConfigChanges()
	if err := w.catacomb.Add(mw); err != nil {
		return errors.Trace(err)
	}
	// Consume initial event.
	if _, ok := <-mw.Changes(); !ok {
		return watcher.EnsureErr(mw)
	}

	rw := w.rel.WatchRelationEgressNetworks()
	if err := w.catacomb.Add(rw); err != nil {
		return errors.Trace(err)
	}
	// Consume initial event.
	if networks, ok := <-rw.Changes(); !ok {
		return watcher.EnsureErr(rw)
	} else {
		w.knownRelationEgress = set.NewStrings(networks...)
	}

	var (
		sentInitial bool
		out         chan<- []string
		addresses   []string
	)
	err = w.initialise()
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	unitAddressesChanged := false
	userConfiguredEgressChanged := false
	for {
		if !sentInitial || unitAddressesChanged || (userConfiguredEgressChanged && len(w.known) > 0) {
			var addressSet set.Strings
			// Egress cidrs, if configured, override unit machine addresses.
			if len(w.known) > 0 {
				// Try relation cidrs first.
				addressSet = set.NewStrings(w.knownRelationEgress.Values()...)
				if addressSet.Size() == 0 {
					// If none of those, try model cidrs.
					addressSet = set.NewStrings(w.knownModelEgress.Values()...)
				}
				if addressSet.Size() == 0 {
					// No user configured egress so just use the unit addresses.
					for _, addr := range w.known {
						addressSet.Add(addr)
					}
				}
			}
			unitAddressesChanged = false
			addresses = FormatAsCIDR(addressSet.Values())
			out = w.out
		}
		userConfiguredEgressChanged = false

		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		// Send initial event or subsequent changes.
		case out <- addresses:
			sentInitial = true
			out = nil
		case _, ok := <-mw.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			cfg, err := w.backend.ModelConfig()
			if err != nil {
				return err
			}
			egress := set.NewStrings(cfg.EgressSubnets()...)
			// Have the egress addresses changed.
			if egress.Size() != w.knownModelEgress.Size() ||
				egress.Difference(w.knownModelEgress).Size() != 0 || w.knownModelEgress.Difference(egress).Size() != 0 {
				// We only care about model egress changes if there's no relation specific egress.
				userConfiguredEgressChanged = w.knownRelationEgress.Size() == 0
				w.knownModelEgress = egress
			}
		case changes, ok := <-rw.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			egress := set.NewStrings(changes...)
			// Have the egress addresses changed.
			if egress.Size() != w.knownRelationEgress.Size() ||
				egress.Difference(w.knownRelationEgress).Size() != 0 || w.knownRelationEgress.Difference(egress).Size() != 0 {
				userConfiguredEgressChanged = true
				w.knownRelationEgress = egress
			}
		case c, ok := <-ruw.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			// A unit has entered or left scope.
			// Get the new set of addresses resulting from that
			// change, and if different to what we know, send the change.
			unitAddressesChanged, err = w.processUnitChanges(c)
			if err != nil {
				return err
			}
		case machineId, ok := <-w.addressChanges:
			if !ok {
				continue
			}
			unitAddressesChanged, err = w.processMachineAddresses(machineId)
			if err != nil {
				return errors.Trace(err)
			}
		}

	}
}

// FormatAsCIDR converts the specified IP addresses to
// a slice of CIDRs.
func FormatAsCIDR(addresses []string) []string {
	result := make([]string, len(addresses))
	for i, a := range addresses {
		cidr := a
		// If address is not already a cidr, add a /32 (ipv4) or /128 (ipv6).
		if _, _, err := net.ParseCIDR(a); err != nil {
			ip := net.ParseIP(a)
			if ip.To4() != nil {
				cidr = a + "/32"
			} else {
				cidr = a + "/128"
			}
		}
		result[i] = cidr
	}
	return result
}

func (w *EgressAddressWatcher) unitAddress(unit Unit) (string, bool, error) {
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

func (w *EgressAddressWatcher) processUnitChanges(c params.RelationUnitsChange) (bool, error) {
	changed := false
	for name := range c.Changed {

		u, err := w.backend.Unit(name)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return false, err
		}

		if err := w.trackUnit(u); err != nil {
			return false, errors.Trace(err)
		}

		// We need to know whether to look at the public or cloud local address.
		// For now, we'll use the public address and later if needed use a watcher
		// parameter to look at the cloud local address.
		addr, ok, err := w.unitAddress(u)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		if w.known[name] != addr {
			w.known[name] = addr
			changed = true
		}
	}
	for _, name := range c.Departed {
		if err := w.untrackUnit(name); err != nil {
			return false, errors.Trace(err)
		}
		// If the unit is departing and we have seen its address,
		// remove the address.
		address, ok := w.known[name]
		if !ok {
			continue
		}
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
			changed = true
		}
	}
	return changed, nil
}

func (w *EgressAddressWatcher) trackUnit(unit Unit) error {
	machine, err := w.assignedMachine(unit)
	if errors.IsNotAssigned(err) {
		logger.Errorf("unit %q entered scope without a machine assigned - addresses will not be tracked", unit)
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	w.unitToMachine[unit.Name()] = machine.Id()
	mData, ok := w.machines[machine.Id()]
	if ok {
		// We're already watching the machine, just add this unit.
		mData.units.Add(unit.Name())
		return nil
	}

	addressWorker, err := newMachineAddressWorker(machine, w.addressChanges)
	if err != nil {
		return errors.Trace(err)
	}
	w.machines[machine.Id()] = &machineData{
		units:  set.NewStrings(unit.Name()),
		worker: addressWorker,
	}
	err = w.catacomb.Add(addressWorker)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (w *EgressAddressWatcher) untrackUnit(unitName string) error {
	machineId, ok := w.unitToMachine[unitName]
	if !ok {
		logger.Errorf("missing machine id for unit %q", unitName)
		return nil
	}
	delete(w.unitToMachine, unitName)

	mData, ok := w.machines[machineId]
	if !ok {
		logger.Debugf("missing machine data for machine %q (hosting unit %q)", machineId, unitName)
		return nil
	}
	mData.units.Remove(unitName)
	if mData.units.Size() > 0 {
		// No need to stop the watcher - there are still units on the
		// machine.
		return nil
	}

	err := worker.Stop(mData.worker)
	if err != nil {
		return errors.Trace(err)
	}
	delete(w.machines, machineId)
	return nil
}

func (w *EgressAddressWatcher) assignedMachine(unit Unit) (Machine, error) {
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

func (w *EgressAddressWatcher) processMachineAddresses(machineId string) (changed bool, err error) {
	mData, ok := w.machines[machineId]
	if !ok {
		return false, errors.Errorf("missing machineData for machine %q", machineId)
	}
	for unitName := range mData.units {
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
func (w *EgressAddressWatcher) Changes() <-chan []string {
	return w.out
}

// Kill asks the watcher to stop without waiting for it do so.
func (w *EgressAddressWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *EgressAddressWatcher) Wait() error {
	return w.catacomb.Wait()
}

// Stop kills the watcher, then waits for it to die.
func (w *EgressAddressWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Err returns any error encountered while the watcher
// has been running.
func (w *EgressAddressWatcher) Err() error {
	return w.catacomb.Err()
}

func newMachineAddressWorker(machine Machine, addressChanges chan<- string) (*machineAddressWorker, error) {
	w := &machineAddressWorker{
		machine:        machine,
		addressChanges: addressChanges,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

// machineAddressWorker watches for machine address changes and
// notifies the dest channel when it sees them.
type machineAddressWorker struct {
	catacomb       catacomb.Catacomb
	machine        Machine
	addressChanges chan<- string
}

func (w *machineAddressWorker) loop() error {
	aw := w.machine.WatchAddresses()
	if err := w.catacomb.Add(aw); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-aw.Changes():
			w.addressChanges <- w.machine.Id()
		}
	}
}

func (w *machineAddressWorker) Kill() {
	w.catacomb.Kill(nil)
}

func (w *machineAddressWorker) Wait() error {
	return w.catacomb.Wait()
}
