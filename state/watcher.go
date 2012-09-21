package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
	"strings"
)

type watcherStubs interface {
	Stop() error
	Wait() error
	Err() error
}

type ServiceCharmWatcher struct {
	watcherStubs
}

// ServiceCharmChange describes a change to the service's charm, and
// whether units should upgrade to that charm in spite of error states.
type ServiceCharmChange struct {
	Charm *Charm
	Force bool
}

// Changes returns a channel that will receive notifications
// about changes to the service's charm. The first event on the
// channel hold the initial state of the charm.
func (w *ServiceCharmWatcher) Changes() <-chan ServiceCharmChange {
	panic("not implemented")
}

// WatchCharm returns a watcher that sends notifications of changes to the
// service's charm.
func (s *Service) WatchCharm() *ServiceCharmWatcher {
	panic("not implemented")
}

// WatchResolved returns a watcher that fires when the unit
// is marked as having had its problems resolved. See
// SetResolved for details.
func (u *Unit) WatchResolved() *ResolvedWatcher {
	panic("not implemented")
}

// ResolvedWatcher observes changes to a unit's resolved
// mode. See SetResolved for details.
type ResolvedWatcher struct {
	watcherStubs
}

// Changes returns a channel that will receive the new
// resolved mode when a change is detected. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial
// state as returned by Unit.Resolved.
func (w *ResolvedWatcher) Changes() <-chan ResolvedMode {
	panic("not implemented")
}

type RelationUnitsWatcher struct {
	watcherStubs
}

type RelationUnitsChange struct {
	Changed  map[string]UnitSettings
	Departed []string
}

func (ru *RelationUnit) Watch() *RelationUnitsWatcher {
	panic("not implemented")
}

// Changes returns a channel that will receive the changes to
// the relation when detected.
// The first event on the channel holds the initial state of the
// relation in its Changed field.
func (w *RelationUnitsWatcher) Changes() <-chan RelationUnitsChange {
	panic("not implemented")
}

// commonWatcher is part of all client watchers.
type commonWatcher struct {
	st   *State
	tomb tomb.Tomb
}

// Stop stops the watcher, and returns any error encountered while running
// or shutting down.
func (w *commonWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// Err returns any error encountered while running or shutting down, or
// tomb.ErrStillAlive if the watcher is still running.
func (w *commonWatcher) Err() error {
	return w.tomb.Err()
}

// MachineWatcher observes changes to the settings of a machine.
type MachineWatcher struct {
	commonWatcher
	changeChan chan *Machine
}

// MachinesWatcher notifies about machines being added or removed
// from the environment.
type MachinesWatcher struct {
	commonWatcher
	changeChan    chan *MachinesChange
	knownMachines map[int]*Machine
}

// MachinesChange contains information about
// machines that have been added or deleted.
type MachinesChange struct {
	Added   []*Machine
	Removed []*Machine
}

// ServicesWatcher observes the addition and removal of services.
type ServicesWatcher struct {
	commonWatcher
	changeChan    chan *ServicesChange
	knownServices map[string]*Service
}

// ServicesChange holds services that were added or removed
// from the environment.
type ServicesChange struct {
	Added   []*Service
	Removed []*Service
}

type ServiceUnitsWatcher struct {
	commonWatcher
	service    *Service
	prefix     string
	changeChan chan *ServiceUnitsChange
	knownUnits map[string]*Unit
}

// ServiceUnitsChange contains information about
// units that have been added to or removed from
// services.
type ServiceUnitsChange struct {
	Added   []*Unit
	Removed []*Unit
}

type ServiceRelationsWatcher struct {
	commonWatcher
	service        *Service
	changeChan     chan *RelationsChange
	knownRelations map[string]*Relation
}

// ServiceRelationChange contains information about
// relations that have been added to or removed from
// a service.
type RelationsChange struct {
	Added   []*Relation
	Removed []*Relation
}

// RelationScopeWatcher observes changes to the set of units
// in a particular relation scope.
type RelationScopeWatcher struct {
	commonWatcher
	prefix     string
	ignore     string
	knownUnits map[string]bool
	changeChan chan *RelationScopeChange
}

// RelationScopeChange contains information about units that have
// entered or left a particular scope.
type RelationScopeChange struct {
	Entered []string
	Left    []string
}

// MachineUnitsWatcher observes the assignment and removal of units
// to and from a machine.
type MachineUnitsWatcher struct {
	commonWatcher
	machine    *Machine
	changeChan chan *MachineUnitsChange
	knownUnits map[string]*Unit
}

// MachineUnitsChange contains information about units that have been
// assigned to or removed from the machine.
type MachineUnitsChange struct {
	Added   []*Unit
	Removed []*Unit
}

// EnvironConfigWatcher observes changes to the
// environment configuration.
type EnvironConfigWatcher struct {
	commonWatcher
	changeChan chan *config.Config
}

// newMachineWatcher creates and starts a watcher to watch information
// about the machine.
func newMachineWatcher(m *Machine) *MachineWatcher {
	w := &MachineWatcher{
		changeChan:    make(chan *Machine),
		commonWatcher: commonWatcher{st: m.st},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop(m))
	}()
	return w
}

// Changes returns a channel that will receive the new
// *Machine when a change is detected. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial state
// as returned by Machine.Info.
func (w *MachineWatcher) Changes() <-chan *Machine {
	return w.changeChan
}

func (w *MachineWatcher) loop(m *Machine) (err error) {
	ch := make(chan watcher.Change)
	id := m.Id()
	st := m.st
	st.watcher.Watch(st.machines.Name, id, m.doc.TxnRevno, ch)
	defer st.watcher.Unwatch(st.machines.Name, id, ch)
	for {
		select {
		case <-st.watcher.Dead():
			return watcher.MustErr(st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-ch:
		}
		if m, err = st.Machine(id); err != nil {
			return err
		}
		for {
			select {
			case <-st.watcher.Dead():
				return watcher.MustErr(st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case <-ch:
				if err := m.Refresh(); err != nil {
					return err
				}
				continue
			case w.changeChan <- m:
			}
			break
		}
	}
	return nil
}

// WatchMachines returns a watcher for observing machines being
// added or removed.
func (s *State) WatchMachines() *MachinesWatcher {
	return newMachinesWatcher(s)
}

// newMachinesWatcher creates and starts a watcher to watch information
// about machines being added or deleted.
func newMachinesWatcher(st *State) *MachinesWatcher {
	w := &MachinesWatcher{
		changeChan:    make(chan *MachinesChange),
		knownMachines: make(map[int]*Machine),
		commonWatcher: commonWatcher{st: st},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive changes when machines are
// added or deleted. The Added field in the first event on the channel
// holds the initial state as returned by State.AllMachines.
func (w *MachinesWatcher) Changes() <-chan *MachinesChange {
	return w.changeChan
}

func (w *MachinesWatcher) mergeChange(changes *MachinesChange, ch watcher.Change) (err error) {
	id := ch.Id.(int)
	if m, ok := w.knownMachines[id]; ch.Revno == -1 && ok {
		m.doc.Life = Dead
		changes.Removed = append(changes.Removed, m)
		delete(w.knownMachines, id)
		return nil
	}
	doc := &machineDoc{}
	err = w.st.machines.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	m := newMachine(w.st, doc)
	if _, ok := w.knownMachines[id]; !ok {
		changes.Added = append(changes.Added, m)
	}
	w.knownMachines[id] = m
	return nil
}

func (changes *MachinesChange) isEmpty() bool {
	return len(changes.Added)+len(changes.Removed) == 0
}

func (w *MachinesWatcher) getInitialEvent() (initial *MachinesChange, err error) {
	changes := &MachinesChange{}
	docs := []machineDoc{}
	err = w.st.machines.Find(nil).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		m := newMachine(w.st, &doc)
		w.knownMachines[doc.Id] = m
		changes.Added = append(changes.Added, m)
	}
	return changes, nil
}

func (w *MachinesWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.machines.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.machines.Name, ch)
	changes, err := w.getInitialEvent()
	if err != nil {
		return err
	}
	for {
		for changes != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case c := <-ch:
				err := w.mergeChange(changes, c)
				if err != nil {
					return err
				}
			case w.changeChan <- changes:
				changes = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-ch:
			changes = &MachinesChange{}
			err := w.mergeChange(changes, c)
			if err != nil {
				return err
			}
			if changes.isEmpty() {
				changes = nil
			}
		}
	}
	return nil
}

// WatchServices returns a watcher for observing services being
// added or removed.
func (s *State) WatchServices() *ServicesWatcher {
	return newServicesWatcher(s)
}

// newServicesWatcher creates and starts a watcher to watch information
// about services being added or deleted.
func newServicesWatcher(st *State) *ServicesWatcher {
	w := &ServicesWatcher{
		changeChan:    make(chan *ServicesChange),
		knownServices: make(map[string]*Service),
		commonWatcher: commonWatcher{st: st},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive changes when services are
// added or deleted. The Added field in the first event on the channel
// holds the initial state as returned by State.AllServices.
func (w *ServicesWatcher) Changes() <-chan *ServicesChange {
	return w.changeChan
}

func (w *ServicesWatcher) mergeChange(changes *ServicesChange, ch watcher.Change) (err error) {
	name := ch.Id.(string)
	if svc, ok := w.knownServices[name]; ch.Revno == -1 && ok {
		svc.doc.Life = Dead
		changes.Removed = append(changes.Removed, svc)
		delete(w.knownServices, name)
		return nil
	}
	doc := &serviceDoc{}
	err = w.st.services.FindId(name).One(doc)
	if err == mgo.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	svc := newService(w.st, doc)
	if _, ok := w.knownServices[name]; !ok {
		changes.Added = append(changes.Added, svc)
	}
	w.knownServices[name] = svc
	return nil
}

func (changes *ServicesChange) isEmpty() bool {
	return len(changes.Added)+len(changes.Removed) == 0
}

func (w *ServicesWatcher) getInitialEvent() (initial *ServicesChange, err error) {
	changes := &ServicesChange{}
	docs := []serviceDoc{}
	err = w.st.services.Find(nil).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		svc := newService(w.st, &doc)
		w.knownServices[doc.Name] = svc
		changes.Added = append(changes.Added, svc)
	}
	return changes, nil
}

func (w *ServicesWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.services.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.services.Name, ch)
	changes, err := w.getInitialEvent()
	if err != nil {
		return err
	}
	for {
		for changes != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case c := <-ch:
				err := w.mergeChange(changes, c)
				if err != nil {
					return err
				}
			case w.changeChan <- changes:
				changes = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-ch:
			changes = &ServicesChange{}
			err := w.mergeChange(changes, c)
			if err != nil {
				return err
			}
			if changes.isEmpty() {
				changes = nil
			}
		}
	}
	return nil
}

// WatchUnits returns a watcher for observing units being
// added or removed.
func (s *Service) WatchUnits() *ServiceUnitsWatcher {
	return newServiceUnitsWatcher(s)
}

// newServiceUnitsWatcher creates and starts a watcher to watch information
// about units being added or deleted.
func newServiceUnitsWatcher(svc *Service) *ServiceUnitsWatcher {
	w := &ServiceUnitsWatcher{
		changeChan:    make(chan *ServiceUnitsChange),
		knownUnits:    make(map[string]*Unit),
		service:       svc,
		prefix:        svc.doc.Name + "/",
		commonWatcher: commonWatcher{st: svc.st},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive changes when units are
// added or deleted. The Added field in the first event on the channel
// holds the initial state as returned by State.AllUnits.
func (w *ServiceUnitsWatcher) Changes() <-chan *ServiceUnitsChange {
	return w.changeChan
}

func (w *ServiceUnitsWatcher) mergeChange(changes *ServiceUnitsChange, ch watcher.Change) (err error) {
	name := ch.Id.(string)
	if !strings.HasPrefix(name, w.prefix) {
		return nil
	}
	if unit, ok := w.knownUnits[name]; ch.Revno == -1 && ok {
		unit.doc.Life = Dead
		changes.Removed = append(changes.Removed, unit)
		delete(w.knownUnits, name)
		return nil
	}
	doc := &unitDoc{}
	err = w.st.units.FindId(name).One(doc)
	if err == mgo.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	unit := newUnit(w.st, doc)
	if _, ok := w.knownUnits[name]; !ok {
		changes.Added = append(changes.Added, unit)
	}
	w.knownUnits[name] = unit
	return nil
}

func (changes *ServiceUnitsChange) isEmpty() bool {
	return len(changes.Added)+len(changes.Removed) == 0
}

func (w *ServiceUnitsWatcher) getInitialEvent() (initial *ServiceUnitsChange, err error) {
	changes := &ServiceUnitsChange{}
	docs := []unitDoc{}
	err = w.st.units.Find(D{{"service", w.service.Name()}}).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		unit := newUnit(w.st, &doc)
		w.knownUnits[doc.Name] = unit
		changes.Added = append(changes.Added, unit)
	}
	return changes, nil
}

func (w *ServiceUnitsWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.units.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.units.Name, ch)
	changes, err := w.getInitialEvent()
	if err != nil {
		return err
	}
	for {
		for changes != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case c := <-ch:
				err := w.mergeChange(changes, c)
				if err != nil {
					return err
				}
			case w.changeChan <- changes:
				changes = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-ch:
			changes = &ServiceUnitsChange{}
			err := w.mergeChange(changes, c)
			if err != nil {
				return err
			}
			if changes.isEmpty() {
				changes = nil
			}
		}
	}
	return nil
}

// WatchRelations returns a watcher for observing relations being
// added or removed from the service.
func (s *Service) WatchRelations() *ServiceRelationsWatcher {
	return newServiceRelationsWatcher(s)
}

// newServiceRelationsWatcher creates and starts a watcher to watch
// information about relations being added or deleted from service m.
func newServiceRelationsWatcher(s *Service) *ServiceRelationsWatcher {
	w := &ServiceRelationsWatcher{
		changeChan:     make(chan *RelationsChange),
		knownRelations: make(map[string]*Relation),
		service:        s,
		commonWatcher:  commonWatcher{st: s.st},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive changes when relations are
// added or deleted. The Added field in the first event on the channel
// holds the initial state as returned by State.AllRelations.
func (w *ServiceRelationsWatcher) Changes() <-chan *RelationsChange {
	return w.changeChan
}

func (w *ServiceRelationsWatcher) mergeChange(changes *RelationsChange, ch watcher.Change) (err error) {
	key := ch.Id.(string)
	if !strings.HasPrefix(key, w.service.doc.Name+":") && !strings.Contains(key, " "+w.service.doc.Name+":") {
		return nil
	}
	if relation, ok := w.knownRelations[key]; ch.Revno == -1 && ok {
		relation.doc.Life = Dead
		changes.Removed = append(changes.Removed, relation)
		delete(w.knownRelations, key)
		return nil
	}
	// Relations don't change, which means this only ever runs
	// when a relation is added. The logic is correct even if they
	// do change, though.
	doc := &relationDoc{}
	err = w.st.relations.Find(D{{"_id", key}, {"endpoints.servicename", w.service.doc.Name}}).One(doc)
	if err == mgo.ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	relation := newRelation(w.st, doc)
	if _, ok := w.knownRelations[key]; !ok {
		changes.Added = append(changes.Added, relation)
	}
	w.knownRelations[key] = relation
	return nil
}

func (changes *RelationsChange) isEmpty() bool {
	return len(changes.Added)+len(changes.Removed) == 0
}

func (w *ServiceRelationsWatcher) getInitialEvent() (initial *RelationsChange, err error) {
	changes := &RelationsChange{}
	relations, err := w.service.Relations()
	if err != nil {
		return nil, err
	}
	for _, relation := range relations {
		w.knownRelations[relation.doc.Key] = relation
		changes.Added = append(changes.Added, relation)
	}
	return changes, nil
}

func (w *ServiceRelationsWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.relations.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.relations.Name, ch)
	changes, err := w.getInitialEvent()
	if err != nil {
		return err
	}
	for {
		for changes != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case c := <-ch:
				err := w.mergeChange(changes, c)
				if err != nil {
					return err
				}
			case w.changeChan <- changes:
				changes = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-ch:
			changes = &RelationsChange{}
			err := w.mergeChange(changes, c)
			if err != nil {
				return err
			}
			if changes.isEmpty() {
				changes = nil
			}
		}
	}
	return nil
}

// WatchPrincipalUnits returns a watcher for observing units being
// added to or removed from the machine.
func (m *Machine) WatchPrincipalUnits() *MachineUnitsWatcher {
	return newMachineUnitsWatcher(m)
}

// newMachineUnitsWatcher creates and starts a watcher to watch information
// about units being added to or deleted from the machine.
func newMachineUnitsWatcher(m *Machine) *MachineUnitsWatcher {
	w := &MachineUnitsWatcher{
		changeChan:    make(chan *MachineUnitsChange),
		machine:       m,
		knownUnits:    make(map[string]*Unit),
		commonWatcher: commonWatcher{st: m.st},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive changes when units are
// added or deleted. The Added field in the first event on the channel
// holds the initial state as returned by Machine.Units.
func (w *MachineUnitsWatcher) Changes() <-chan *MachineUnitsChange {
	return w.changeChan
}

func (w *MachineUnitsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *MachineUnitsWatcher) mergeChange(changes *MachineUnitsChange, ch watcher.Change) (err error) {
	if ch.Revno == -1 {
		return fmt.Errorf("machine has been removed")
	}
	err = w.machine.Refresh()
	if err != nil {
		return err
	}
	units := make(map[string]*Unit)
	for _, name := range w.machine.doc.Principals {
		var unit *Unit
		doc := &unitDoc{}
		if _, ok := w.knownUnits[name]; !ok {
			err = w.st.units.FindId(name).One(doc)
			if err == mgo.ErrNotFound {
				continue
			}
			if err != nil {
				return err
			}
			unit = newUnit(w.st, doc)
			changes.Added = append(changes.Added, unit)
			w.knownUnits[name] = unit
		}
		units[name] = unit
	}
	for name, unit := range w.knownUnits {
		if _, ok := units[name]; !ok {
			changes.Removed = append(changes.Removed, unit)
			delete(w.knownUnits, name)
		}
	}
	return nil
}

func (changes *MachineUnitsChange) isEmpty() bool {
	return len(changes.Added)+len(changes.Removed) == 0
}

func (w *MachineUnitsWatcher) getInitialEvent() (initial *MachineUnitsChange, err error) {
	changes := &MachineUnitsChange{}
	docs := []unitDoc{}
	err = w.st.units.Find(D{{"_id", D{{"$in", w.machine.doc.Principals}}}}).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		unit := newUnit(w.st, &doc)
		w.knownUnits[doc.Name] = unit
		changes.Added = append(changes.Added, unit)
	}
	return changes, nil
}

func (w *MachineUnitsWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.st.watcher.Watch(w.st.machines.Name, w.machine.doc.Id, w.machine.doc.TxnRevno, ch)
	defer w.st.watcher.Unwatch(w.st.machines.Name, w.machine.doc.Id, ch)
	changes, err := w.getInitialEvent()
	if err != nil {
		return err
	}
	for {
		for changes != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case c := <-ch:
				err := w.mergeChange(changes, c)
				if err != nil {
					return err
				}
			case w.changeChan <- changes:
				changes = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-ch:
			changes = &MachineUnitsChange{}
			err := w.mergeChange(changes, c)
			if err != nil {
				return err
			}
			if changes.isEmpty() {
				changes = nil
			}
		}
	}
	return nil
}

func newRelationScopeWatcher(st *State, scope, ignore string) *RelationScopeWatcher {
	w := &RelationScopeWatcher{
		commonWatcher: commonWatcher{st: st},
		prefix:        scope + "#",
		ignore:        ignore,
		changeChan:    make(chan *RelationScopeChange),
		knownUnits:    make(map[string]bool),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive changes when units enter and
// leave a relation scope. The Entered field in the first event on the channel
// holds the initial state.
func (w *RelationScopeWatcher) Changes() <-chan *RelationScopeChange {
	return w.changeChan
}

func (changes *RelationScopeChange) isEmpty() bool {
	return len(changes.Entered)+len(changes.Left) == 0
}

func (w *RelationScopeWatcher) mergeChange(changes *RelationScopeChange, ch watcher.Change) (err error) {
	doc := &relationScopeDoc{ch.Id.(string)}
	if !strings.HasPrefix(doc.Key, w.prefix) {
		return nil
	}
	name := doc.unitName()
	if name == w.ignore {
		return nil
	}
	if ch.Revno == -1 {
		if w.knownUnits[name] {
			changes.Left = append(changes.Left, name)
			delete(w.knownUnits, name)
		}
		return nil
	}
	if !w.knownUnits[name] {
		changes.Entered = append(changes.Entered, name)
		w.knownUnits[name] = true
	}
	return nil
}

func (w *RelationScopeWatcher) getInitialEvent() (initial *RelationScopeChange, err error) {
	changes := &RelationScopeChange{}
	docs := []relationScopeDoc{}
	sel := D{{"_id", D{{"$regex", "^" + w.prefix}}}}
	err = w.st.relationScopes.Find(sel).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		if name := doc.unitName(); name != w.ignore {
			changes.Entered = append(changes.Entered, name)
			w.knownUnits[name] = true
		}
	}
	return changes, nil
}

func (w *RelationScopeWatcher) loop() error {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.relationScopes.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.relationScopes.Name, ch)
	changes, err := w.getInitialEvent()
	if err != nil {
		return err
	}
	for {
		for changes != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case c := <-ch:
				if err := w.mergeChange(changes, c); err != nil {
					return err
				}
			case w.changeChan <- changes:
				changes = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-ch:
			changes = &RelationScopeChange{}
			if err := w.mergeChange(changes, c); err != nil {
				return err
			}
			if changes.isEmpty() {
				changes = nil
			}
		}
	}
	return nil
}

// WatchEnvironConfig returns a watcher for observing changes
// to the environment configuration.
func (s *State) WatchEnvironConfig() *EnvironConfigWatcher {
	return newEnvironConfigWatcher(s)
}

func newEnvironConfigWatcher(s *State) *EnvironConfigWatcher {
	w := &EnvironConfigWatcher{
		changeChan:    make(chan *config.Config),
		commonWatcher: commonWatcher{st: s},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive the new environment
// configuration when a change is detected. Note that multiple changes may
// be observed as a single event in the channel.
func (w *EnvironConfigWatcher) Changes() <-chan *config.Config {
	return w.changeChan
}

func (w *EnvironConfigWatcher) loop() (err error) {
	settingsWatcher := w.st.watchConfig("e")
	defer settingsWatcher.Stop()
	changes := settingsWatcher.Changes()
	configNode := <-changes
	cfg, err := config.New(configNode.Map())
	if err != nil {
		return err
	}
	for {
		for cfg != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case configNode := <-changes:
				cfg, err = config.New(configNode.Map())
				if err != nil {
					return err
				}
			case w.changeChan <- cfg:
				cfg = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case configNode := <-changes:
			cfg, err = config.New(configNode.Map())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type settingsWatcher struct {
	commonWatcher
	changeChan chan *ConfigNode
}

// watchConfig creates a watcher for observing changes to settings.
func (s *State) watchConfig(key string) *settingsWatcher {
	return newSettingsWatcher(s, key)
}

func newSettingsWatcher(s *State, key string) *settingsWatcher {
	w := &settingsWatcher{
		changeChan:    make(chan *ConfigNode),
		commonWatcher: commonWatcher{st: s},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop(key))
	}()
	return w
}

// Changes returns a channel that will receive the new settings.
// Multiple changes may be observed as a single event in the channel.
func (w *settingsWatcher) Changes() <-chan *ConfigNode {
	return w.changeChan
}

func (w *settingsWatcher) loop(key string) (err error) {
	ch := make(chan watcher.Change)
	configNode, err := readConfigNode(w.st, key)
	if err != nil {
		return err
	}
	w.st.watcher.Watch(w.st.settings.Name, key, configNode.txnRevno, ch)
	defer w.st.watcher.Unwatch(w.st.settings.Name, key, ch)
	for {
		for configNode != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case <-ch:
				configNode, err = readConfigNode(w.st, key)
				if err != nil {
					return err
				}
			case w.changeChan <- configNode:
				configNode = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-ch:
			configNode, err = readConfigNode(w.st, key)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// UnitWatcher observes changes to a unit.
type UnitWatcher struct {
	commonWatcher
	changeChan chan *Unit
}

// Watch return a watcher for observing changes to a unit.
func (u *Unit) Watch() *UnitWatcher {
	return newUnitWatcher(u)
}

func newUnitWatcher(u *Unit) *UnitWatcher {
	w := &UnitWatcher{
		changeChan:    make(chan *Unit),
		commonWatcher: commonWatcher{st: u.st},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop(u))
	}()
	return w
}

// Changes returns a channel that will receive the new version of a unit.
// Multiple changes may be observed as a single event in the channel.
func (w *UnitWatcher) Changes() <-chan *Unit {
	return w.changeChan
}

func (w *UnitWatcher) loop(unit *Unit) (err error) {
	ch := make(chan watcher.Change)
	name := unit.doc.Name
	w.st.watcher.Watch(w.st.units.Name, name, unit.doc.TxnRevno, ch)
	defer w.st.watcher.Unwatch(w.st.units.Name, name, ch)
	for {
		for unit != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case <-ch:
				unit, err = w.st.Unit(name)
				if err != nil {
					return err
				}
			case w.changeChan <- unit:
				unit = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-ch:
			unit, err = w.st.Unit(name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// ServiceWatcher observes changes to a service.
type ServiceWatcher struct {
	commonWatcher
	changeChan chan *Service
}

// Watch return a watcher for observing changes to a service.
func (s *Service) Watch() *ServiceWatcher {
	return newServiceWatcher(s)
}

func newServiceWatcher(s *Service) *ServiceWatcher {
	w := &ServiceWatcher{
		changeChan:    make(chan *Service),
		commonWatcher: commonWatcher{st: s.st},
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.changeChan)
		w.tomb.Kill(w.loop(s))
	}()
	return w
}

// Changes returns a channel that will receive the new version of a service.
// Multiple changes may be observed as a single event in the channel.
func (w *ServiceWatcher) Changes() <-chan *Service {
	return w.changeChan
}

func (w *ServiceWatcher) loop(service *Service) (err error) {
	ch := make(chan watcher.Change)
	name := service.doc.Name
	w.st.watcher.Watch(w.st.services.Name, name, service.doc.TxnRevno, ch)
	defer w.st.watcher.Unwatch(w.st.services.Name, name, ch)
	for {
		for service != nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case <-ch:
				service, err = w.st.Service(name)
				if err != nil {
					return err
				}
			case w.changeChan <- service:
				service = nil
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-ch:
			service, err = w.st.Service(name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type ConfigWatcher struct {
	*settingsWatcher
}

func (s *Service) WatchConfig() *ConfigWatcher {
	return &ConfigWatcher{newSettingsWatcher(s.st, "s#"+s.Name())}
}
