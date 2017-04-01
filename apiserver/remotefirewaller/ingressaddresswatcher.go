// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

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

	// A map of known unit addresses, keyed on unit name.
	known map[string]string
}

// NewIngressAddressWatcher creates an IngressAddressWatcher.
func NewIngressAddressWatcher(backend State, rel Relation, appName string) (*IngressAddressWatcher, error) {
	w := &IngressAddressWatcher{
		backend: backend,
		appName: appName,
		rel:     rel,
		known:   make(map[string]string),
		out:     make(chan []string),
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
		if inScope {
			addr, ok, err := w.unitAddress(u)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if !ok {
				continue
			}
			result.Add(addr)
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
			merged, err := w.mergeAddresses(addresses, c)
			if err != nil {
				return err
			}
			changed := !merged.Difference(addresses).IsEmpty() || !addresses.Difference(merged).IsEmpty()
			if !sentInitial || changed {
				addresses = merged
				out = w.out
			} else {
				out = nil
			}
		}
	}
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

		// TODO - start watcher to pick up machine address changes
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
