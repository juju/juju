// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
	Watch() (watcher.StringsWatcher, error)
}

// EgressAddressWatcher reports changes to addresses
// for local units in a given relation.
// Each event contains the entire set of addresses which
// are required for ingress on the remote side of the relation.
type EgressAddressWatcher struct {
	catacomb catacomb.Catacomb

	backend            State
	modelConfigService ModelConfigService

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
func NewEgressAddressWatcher(backend State, modelConfigService ModelConfigService, rel Relation, appName string) (*EgressAddressWatcher, error) {
	w := &EgressAddressWatcher{
		backend:            backend,
		modelConfigService: modelConfigService,
		appName:            appName,
		rel:                rel,
		known:              make(map[string]string),
		out:                make(chan []string),
		addressChanges:     make(chan string),
		machines:           make(map[string]*machineData),
		unitToMachine:      make(map[string]string),
		knownModelEgress:   set.NewStrings(),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

func (w *EgressAddressWatcher) loop() error {
	defer close(w.out)

	ruw, err := w.rel.WatchUnits(w.appName)
	if errors.Is(err, errors.NotFound) {
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
	mw, err := w.modelConfigService.Watch()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(mw); err != nil {
		return errors.Trace(err)
	}

	// todo(gfouillet): cross model relation disabled and removed from state,
	//    Egress networks watcher has been removed.

	var (
		changed       bool
		sentInitial   bool
		out           chan<- []string
		addresses     set.Strings
		lastAddresses set.Strings
		addressesCIDR []string
	)

	// Wait for each of the watchers started above to
	// send an initial change before sending any changes
	// from this watcher.
	var haveInitialRelationUnits bool
	var haveInitialRelationEgressNetworks bool
	var haveInitialModelConfig bool

	for {
		var ready bool
		if !sentInitial {
			ready = haveInitialRelationUnits && haveInitialRelationEgressNetworks && haveInitialModelConfig
		}
		if ready || changed {
			addresses = nil
			if len(w.known) > 0 {
				// Egress CIDRs, if configured, override unit
				// machine addresses. Relation CIDRs take
				// precedence over those specified in model
				// config.
				addresses = set.NewStrings(w.knownRelationEgress.Values()...)
				if addresses.Size() == 0 {
					addresses = set.NewStrings(w.knownModelEgress.Values()...)
				}
				if addresses.Size() == 0 {
					// No user configured egress so just use the unit addresses.
					for _, addr := range w.known {
						addresses.Add(addr)
					}
				}
			}
			changed = false
			if !setEquals(addresses, lastAddresses) {
				addressesCIDR = network.SubnetsForAddresses(addresses.Values())
				ready = ready || sentInitial
			}
		}
		if ready {
			out = w.out
		}

		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case out <- addressesCIDR:
			sentInitial = true
			lastAddresses = addresses
			out = nil

		case _, ok := <-mw.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			egress, err := w.getEgressSubnets()
			if err != nil {
				return err
			}
			haveInitialModelConfig = true
			if !setEquals(egress, w.knownModelEgress) {
				logger.Debugf(context.TODO(),
					"model config egress subnets changed to %s (was %s)",
					egress.SortedValues(),
					w.knownModelEgress.SortedValues(),
				)
				changed = true
				w.knownModelEgress = egress
			}

		// todo(gfouillet): cross model relation disabled and removed from state,
		//    Egress networks watcher has been removed.

		case c, ok := <-ruw.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			// A unit has entered or left scope.
			// Get the new set of addresses resulting from that
			// change, and if different to what we know, send the change.
			haveInitialRelationUnits = true
			addressesChanged, err := w.processUnitChanges(c)
			if err != nil {
				return err
			}
			changed = changed || addressesChanged

		case machineId, ok := <-w.addressChanges:
			if !ok {
				continue
			}
			addressesChanged, err := w.processMachineAddresses(machineId)
			if err != nil {
				return errors.Trace(err)
			}
			changed = changed || addressesChanged
		}
	}
}

// scopedContext returns a context that is in the scope of the watcher lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *EgressAddressWatcher) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (w *EgressAddressWatcher) getEgressSubnets() (set.Strings, error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	cfg, err := w.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, err
	}
	return set.NewStrings(cfg.EgressSubnets()...), nil
}

func (w *EgressAddressWatcher) unitAddress(unit Unit) (string, bool, error) {
	addr, err := unit.PublicAddress()
	if errors.Is(err, errors.NotAssigned) {
		logger.Debugf(context.TODO(), "unit %s is not assigned to a machine, can't get address", unit.Name())
		return "", false, nil
	}
	if network.IsNoAddressError(err) {
		logger.Debugf(context.TODO(), "unit %s has no public address", unit.Name())
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	logger.Debugf(context.TODO(), "unit %q has public address %q", unit.Name(), addr.Value)
	return addr.Value, true, nil
}

func (w *EgressAddressWatcher) processUnitChanges(c watcher.RelationUnitsChange) (bool, error) {
	changed := false
	for name := range c.Changed {

		u, err := w.backend.Unit(name)
		if errors.Is(err, errors.NotFound) {
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
	if errors.Is(err, errors.NotAssigned) {
		logger.Errorf(context.TODO(), "unit %q entered scope without a machine assigned - addresses will not be tracked", unit)
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
		logger.Errorf(context.TODO(), "missing machine id for unit %q", unitName)
		return nil
	}
	delete(w.unitToMachine, unitName)

	mData, ok := w.machines[machineId]
	if !ok {
		logger.Debugf(context.TODO(), "missing machine data for machine %q (hosting unit %q)", machineId, unitName)
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
		if errors.Is(err, errors.NotFound) {
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

func newMachineAddressWorker(machine Machine, out chan<- string) (*machineAddressWorker, error) {
	w := &machineAddressWorker{
		machine: machine,
		out:     out,
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
	catacomb catacomb.Catacomb
	machine  Machine
	out      chan<- string
}

func (w *machineAddressWorker) loop() error {
	aw := w.machine.WatchAddresses()
	if err := w.catacomb.Add(aw); err != nil {
		return errors.Trace(err)
	}
	machineId := w.machine.Id()
	var out chan<- string
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-aw.Changes():
			out = w.out
		case out <- machineId:
			out = nil
		}
	}
}

func (w *machineAddressWorker) Kill() {
	w.catacomb.Kill(nil)
}

func (w *machineAddressWorker) Wait() error {
	return w.catacomb.Wait()
}

func setEquals(a, b set.Strings) bool {
	if a.Size() != b.Size() {
		return false
	}
	return a.Intersection(b).Size() == a.Size()
}
