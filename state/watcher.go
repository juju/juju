package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
	"strings"
	"sync"
)

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

// MachinePrincipalUnitsWatcher observes the assignment and removal of units
// to and from a machine.
type MachinePrincipalUnitsWatcher struct {
	commonWatcher
	machine    *Machine
	changeChan chan *MachinePrincipalUnitsChange
	knownUnits map[string]*Unit
}

// MachinePrincipalUnitsChange contains information about units that have been
// assigned to or removed from the machine.
type MachinePrincipalUnitsChange struct {
	Added   []*Unit
	Removed []*Unit
}

// MachineWatcher observes changes to the properties of a machine.
type MachineWatcher struct {
	commonWatcher
	out chan int
}

// newMachineWatcher creates and starts a watcher to watch information
// about the machine.
func newMachineWatcher(m *Machine) *MachineWatcher {
	w := &MachineWatcher{
		commonWatcher: commonWatcher{st: m.st},
		out:           make(chan int),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(m))
	}()
	return w
}

// Changes returns a channel that will receive a machine id
// when a change is detected. Note that multiple changes may
// be observed as a single event in the channel.
// As conventional for watchers, an initial event is sent when
// the watcher starts up, whether changes are detected or not.
func (w *MachineWatcher) Changes() <-chan int {
	return w.out
}

func (w *MachineWatcher) loop(m *Machine) (err error) {
	ch := make(chan watcher.Change)
	id := m.Id()
	st := m.st
	st.watcher.Watch(st.machines.Name, id, m.doc.TxnRevno, ch)
	defer st.watcher.Unwatch(st.machines.Name, id, ch)
	out := w.out
	for {
		select {
		case <-st.watcher.Dead():
			return watcher.MustErr(st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-ch:
			out = w.out
		case out <- id:
			out = nil
		}
	}
	return nil
}

// MachinesWatcher notifies about machines being added or removed
// from the environment.
type MachinesWatcher struct {
	commonWatcher
	out   chan *MachinesChange
	alive map[int]bool
}

// MachinesChange holds the ids of machines that are observed to
// be alive or dead.
type MachinesChange struct {
	Alive []int
	Dead  []int
}

func (c *MachinesChange) empty() bool {
	return len(c.Alive)+len(c.Dead) == 0
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
		commonWatcher: commonWatcher{st: st},
		out:           make(chan *MachinesChange),
		alive:         make(map[int]bool),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive changes when machines are
// added or deleted. The Alive field in the first event on the channel
// holds the initial state as returned by State.AllMachines.
func (w *MachinesWatcher) Changes() <-chan *MachinesChange {
	return w.out
}

func (w *MachinesWatcher) initial(changes *MachinesChange) (err error) {
	iter := w.st.machines.Find(notDead).Select(D{{"_id", 1}}).Iter()
	var doc struct {
		Id int `bson:"_id"`
	}
	for iter.Next(&doc) {
		changes.Alive = append(changes.Alive, doc.Id)
		w.alive[doc.Id] = true
	}
	if err := iter.Err(); err != nil {
		return err
	}
	return nil
}

func (w *MachinesWatcher) merge(changes *MachinesChange, ch watcher.Change) error {
	id := ch.Id.(int)
	if ch.Revno == -1 && w.alive[id] {
		panic("machine removed before being dead")
	}
	qdoc := D{{"_id", id}, {"life", D{{"$ne", Dead}}}}
	c, err := w.st.machines.Find(qdoc).Count()
	if err != nil {
		return err
	}
	if c > 0 {
		if !w.alive[id] {
			w.alive[id] = true
			changes.Alive = append(changes.Alive, id)
		}
	} else {
		if w.alive[id] {
			delete(w.alive, id)
			changes.Dead = append(changes.Dead, id)
		}
	}
	return nil
}

func (w *MachinesWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.machines.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.machines.Name, ch)
	changes := &MachinesChange{}
	if err = w.initial(changes); err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-ch:
			if err := w.merge(changes, c); err != nil {
				return err
			}
			if !changes.empty() {
				out = w.out
			}
		case out <- changes:
			changes = &MachinesChange{}
			out = nil
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
func (m *Machine) WatchPrincipalUnits() *MachinePrincipalUnitsWatcher {
	return newMachinePrincipalUnitsWatcher(m)
}

// newMachinePrincipalUnitsWatcher creates and starts a watcher to watch information
// about units being added to or deleted from the machine.
func newMachinePrincipalUnitsWatcher(m *Machine) *MachinePrincipalUnitsWatcher {
	w := &MachinePrincipalUnitsWatcher{
		changeChan:    make(chan *MachinePrincipalUnitsChange),
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
func (w *MachinePrincipalUnitsWatcher) Changes() <-chan *MachinePrincipalUnitsChange {
	return w.changeChan
}

func (w *MachinePrincipalUnitsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *MachinePrincipalUnitsWatcher) mergeChange(changes *MachinePrincipalUnitsChange, ch watcher.Change) (err error) {
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

func (changes *MachinePrincipalUnitsChange) isEmpty() bool {
	return len(changes.Added)+len(changes.Removed) == 0
}

func (w *MachinePrincipalUnitsWatcher) getInitialEvent() (initial *MachinePrincipalUnitsChange, err error) {
	changes := &MachinePrincipalUnitsChange{}
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

func (w *MachinePrincipalUnitsWatcher) loop() (err error) {
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
			changes = &MachinePrincipalUnitsChange{}
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

// RelationUnitsWatcher sends notifications of units entering and leaving the
// scope of a RelationUnit, and changes to the settings of those units known
// to have entered.
type RelationUnitsWatcher struct {
	commonWatcher
	sw       *RelationScopeWatcher
	watching map[string]bool
	updates  chan watcher.Change
	out      chan RelationUnitsChange
}

// RelationUnitsChange holds notifications of units entering and leaving the
// scope of a RelationUnit, and changes to the settings of those units known
// to have entered.
//
// When a counterpart first enters scope, it is/ noted in the Joined field,
// and its settings are noted in the Changed field. Subsequently, settings
// changes will be noted in the Changed field alone, until the couterpart
// leaves the scope; at that point, it will be noted in the Departed field,
// and no further events will be sent for that counterpart unit.
type RelationUnitsChange struct {
	Joined   []string
	Changed  map[string]UnitSettings
	Departed []string
}

// Watch returns a watcher that notifies of changes to conterpart units in
// the relation.
func (ru *RelationUnit) Watch() *RelationUnitsWatcher {
	return newRelationUnitsWatcher(ru)
}

func newRelationUnitsWatcher(ru *RelationUnit) *RelationUnitsWatcher {
	w := &RelationUnitsWatcher{
		commonWatcher: commonWatcher{st: ru.st},
		sw:            ru.WatchScope(),
		watching:      map[string]bool{},
		updates:       make(chan watcher.Change),
		out:           make(chan RelationUnitsChange),
	}
	go func() {
		defer w.finish()
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive the changes to
// counterpart units in a relation. The first event on the
// channel holds the initial state of the relation in its
// Joined and Changed fields.
func (w *RelationUnitsWatcher) Changes() <-chan RelationUnitsChange {
	return w.out
}

func (changes *RelationUnitsChange) empty() bool {
	return len(changes.Joined)+len(changes.Changed)+len(changes.Departed) == 0
}

// mergeSettings reads the relation settings node for the unit with the
// supplied id, and sets a value in the Changed field keyed on the unit's
// name. It returns the mgo/txn revision number of the settings node.
func (w *RelationUnitsWatcher) mergeSettings(changes *RelationUnitsChange, key string) (int64, error) {
	node, err := readConfigNode(w.st, key)
	if err != nil {
		return -1, err
	}
	name := (&relationScopeDoc{key}).unitName()
	settings := UnitSettings{node.txnRevno, node.Map()}
	if changes.Changed == nil {
		changes.Changed = map[string]UnitSettings{name: settings}
	} else {
		changes.Changed[name] = settings
	}
	return node.txnRevno, nil
}

// mergeScope starts and stops settings watches on the units entering and
// leaving the scope in the supplied RelationScopeChange event, and applies
// the expressed changes to the supplied RelationUnitsChange event.
func (w *RelationUnitsWatcher) mergeScope(changes *RelationUnitsChange, c *RelationScopeChange) error {
	for _, name := range c.Entered {
		key := w.sw.prefix + name
		revno, err := w.mergeSettings(changes, key)
		if err != nil {
			return err
		}
		changes.Joined = append(changes.Joined, name)
		changes.Departed = remove(changes.Departed, name)
		w.st.watcher.Watch(w.st.settings.Name, key, revno, w.updates)
		w.watching[key] = true
	}
	for _, name := range c.Left {
		key := w.sw.prefix + name
		changes.Departed = append(changes.Departed, name)
		if changes.Changed != nil {
			delete(changes.Changed, name)
		}
		changes.Joined = remove(changes.Joined, name)
		w.st.watcher.Unwatch(w.st.settings.Name, key, w.updates)
		delete(w.watching, key)
	}
	return nil
}

// remove removes s from strs and returns the modified slice.
func remove(strs []string, s string) []string {
	for i, v := range strs {
		if s == v {
			strs[i] = strs[len(strs)-1]
			return strs[:len(strs)-1]
		}
	}
	return strs
}

func (w *RelationUnitsWatcher) finish() {
	watcher.Stop(w.sw, &w.tomb)
	for key := range w.watching {
		w.st.watcher.Unwatch(w.st.settings.Name, key, w.updates)
	}
	close(w.updates)
	close(w.out)
	w.tomb.Done()
}

func (w *RelationUnitsWatcher) loop() (err error) {
	sentInitial := false
	changes := RelationUnitsChange{}
	out := w.out
	out = nil
	for {
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c, ok := <-w.sw.Changes():
			if !ok {
				return watcher.MustErr(w.sw)
			}
			if err = w.mergeScope(&changes, c); err != nil {
				return err
			}
			if !sentInitial || !changes.empty() {
				out = w.out
			} else {
				out = nil
			}
		case c := <-w.updates:
			if _, err = w.mergeSettings(&changes, c.Id.(string)); err != nil {
				return err
			}
			out = w.out
		case out <- changes:
			sentInitial = true
			changes = RelationUnitsChange{}
			out = nil
		}
	}
	panic("unreachable")
}

// EnvironConfigWatcher observes changes to the
// environment configuration.
type EnvironConfigWatcher struct {
	commonWatcher
	out chan *config.Config
}

// WatchEnvironConfig returns a watcher for observing changes
// to the environment configuration.
func (s *State) WatchEnvironConfig() *EnvironConfigWatcher {
	return newEnvironConfigWatcher(s)
}

func newEnvironConfigWatcher(s *State) *EnvironConfigWatcher {
	w := &EnvironConfigWatcher{
		commonWatcher: commonWatcher{st: s},
		out:           make(chan *config.Config),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive the new environment
// configuration when a change is detected. Note that multiple changes may
// be observed as a single event in the channel.
func (w *EnvironConfigWatcher) Changes() <-chan *config.Config {
	return w.out
}

func (w *EnvironConfigWatcher) loop() (err error) {
	sw := w.st.watchSettings("e")
	defer sw.Stop()
	out := w.out
	out = nil
	cfg := &config.Config{}
	for {
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case configNode, ok := <-sw.Changes():
			if !ok {
				return watcher.MustErr(sw)
			}
			cfg, err = config.New(configNode.Map())
			if err == nil {
				out = w.out
			} else {
				out = nil
			}
		case out <- cfg:
			out = nil
		}
	}
	return nil
}

type settingsWatcher struct {
	commonWatcher
	out chan *ConfigNode
}

// watchSettings creates a watcher for observing changes to settings.
func (s *State) watchSettings(key string) *settingsWatcher {
	return newSettingsWatcher(s, key)
}

func newSettingsWatcher(s *State, key string) *settingsWatcher {
	w := &settingsWatcher{
		commonWatcher: commonWatcher{st: s},
		out:           make(chan *ConfigNode),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(key))
	}()
	return w
}

// Changes returns a channel that will receive the new settings.
// Multiple changes may be observed as a single event in the channel.
func (w *settingsWatcher) Changes() <-chan *ConfigNode {
	return w.out
}

func (w *settingsWatcher) loop(key string) (err error) {
	ch := make(chan watcher.Change)
	configNode, err := readConfigNode(w.st, key)
	if err != nil {
		return err
	}
	w.st.watcher.Watch(w.st.settings.Name, key, configNode.txnRevno, ch)
	defer w.st.watcher.Unwatch(w.st.settings.Name, key, ch)
	out := w.out
	nul := make(chan *ConfigNode)
	for {
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
			out = w.out
		case out <- configNode:
			out = nul
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
	name := unit.doc.Name
	if unit, err = w.st.Unit(name); err != nil {
		return err
	}
	ch := make(chan watcher.Change)
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
				if unit, err = w.st.Unit(name); err != nil {
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
			if unit, err = w.st.Unit(name); err != nil {
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
	name := service.doc.Name
	if service, err = w.st.Service(name); err != nil {
		return err
	}
	ch := make(chan watcher.Change)
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
				if service, err = w.st.Service(name); err != nil {
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
			if service, err = w.st.Service(name); err != nil {
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

// MachineUnitsWatcher observes the assignment and removal of units
// to and from a machine.
type MachineUnitsWatcher struct {
	commonWatcher
	wg         sync.WaitGroup
	machine    *Machine
	out        chan []string
	known      map[string]bool
	principals map[string]bool
	mu         sync.Mutex
}

// WatchUnits returns a watcher for observing units being assigned to
// or removed from a machine.
func (m *Machine) WatchUnits() *MachineUnitsWatcher {
	return newMachineUnitsWatcher(m)
}

func newMachineUnitsWatcher(m *Machine) *MachineUnitsWatcher {
	w := &MachineUnitsWatcher{
		commonWatcher: commonWatcher{st: m.st},
		out:           make(chan []string),
		known:         make(map[string]bool),
		principals:    make(map[string]bool),
		machine:       m,
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *MachineUnitsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// Changes returns a channel that will receive changes when units are added
// or removed from the machine. The first event on the channel holds the
// initial state as returned by Machine.Units.
func (w *MachineUnitsWatcher) Changes() <-chan []string {
	return w.out
}

func (w *MachineUnitsWatcher) initial() (changes []string, err error) {
	var pudocs []struct {
		Name string `bson:"_id"`
	}
	err = w.st.units.Find(append(notDead, D{{"machineid", w.machine.doc.Id}}...)).Select(D{{"_id", 1}}).All(&pudocs)
	if err != nil {
		return nil, err
	}
	for _, pudoc := range pudocs {
		changes = append(changes, pudoc.Name)
		w.known[pudoc.Name] = true
		sudocs := pudocs
		err = w.st.units.Find(append(notDead, D{{"principal", pudoc.Name}}...)).Select(D{{"_id", 1}}).All(&sudocs)
		if err != nil {
			return nil, err
		}
		for _, sudoc := range sudocs {
			changes = append(changes, sudoc.Name)
			w.known[sudoc.Name] = true
		}
	}
	return changes, nil
}

func (w *MachineUnitsWatcher) watchSubordinates(principal string, changes chan []string) {
	defer w.wg.Done()
	ch := make(chan watcher.Change)
	w.st.watcher.Watch(w.st.units.Name, principal, 0, ch)
	defer w.st.watcher.Unwatch(w.st.units.Name, principal, ch)
	fmt.Printf("\n  !!! ->sW: principal: %v\n\n", principal)
	for {
		var c watcher.Change
		var ok bool
		select {
		case <-w.st.watcher.Dead():
			return
		case <-w.tomb.Dying():
			return
		case c, ok = <-ch:
			fmt.Printf("  !! event: %#v\n", c)
			if !ok || c.Revno == -1 {
				return
			}
			var u struct {
				Subordinates []string
			}
			err := w.st.units.FindId(principal).Select(D{{"subordinates", 1}}).One(&u)
			if err != nil {
				w.tomb.Kill(err)
				return
			}
			fmt.Printf("  !! subordinates: %#v\n", u.Subordinates)
			subordinates := []string{}
			for _, unit := range u.Subordinates {
				w.mu.Lock()
				if !w.known[unit] {
					subordinates = append(subordinates, unit)
				}
				w.mu.Unlock()
			}
			if len(subordinates) > 0 {
				changes <- subordinates
			}
		}
	}
}

func (w *MachineUnitsWatcher) merge(changes []string, c watcher.Change) []string {
	fmt.Printf("\n  ! M1\n")
	name := c.Id.(string)
	w.mu.Lock()
	defer w.mu.Unlock()
	if c.Revno == -1 {
		delete(w.known, name)
		return changes
	}
	fmt.Printf("\n  ! M2\n")
	if !w.known[name] {
		fmt.Printf("\n  ! M2 returns %#v\n", changes)
		return changes
	}
	fmt.Printf("\n  ! M3\n")
	for i, unit := range changes {
		if unit == name {
			return append(changes[:i], append(changes[i+1:], name)...)
		}
	}
	return append(changes, name)
}

func (w *MachineUnitsWatcher) loop() (err error) {
	defer w.wg.Wait()
	ch := make(chan watcher.Change)
	w.st.watcher.Watch(w.st.machines.Name, w.machine.doc.Id, w.machine.doc.TxnRevno, ch)
	defer w.st.watcher.Unwatch(w.st.machines.Name, w.machine.doc.Id, ch)
	all := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.units.Name, all)
	defer w.st.watcher.UnwatchCollection(w.st.units.Name, all)
	changes, err := w.initial()
	if err != nil {
		return err
	}
	newunits := make(chan []string)
	for _, unit := range w.machine.doc.Principals {
		w.principals[unit] = true
		w.wg.Add(1)
		go w.watchSubordinates(unit, newunits)
	}
	out := w.out
	for {
		fmt.Printf("\n !!!! loop changes: %#v\n", changes)
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case units := <-newunits:
			fmt.Printf("\n  !!! nU: %#v\n", units)
			for _, u := range units {
				w.mu.Lock()
				w.known[u] = true
				w.mu.Unlock()
			}
			changes = append(changes, units...)
			out = w.out
			fmt.Printf("\n  !!! nU changes: %#v\n", changes)
		case <-ch:
			err = w.machine.Refresh()
			if err != nil {
				w.tomb.Kill(err)
				return err
			}
			newprincipals := map[string]bool{}
			for _, unit := range w.machine.doc.Principals {
				newprincipals[unit] = true
				if !w.principals[unit] {
					changes = append(changes, unit)
					w.wg.Add(1)
					go w.watchSubordinates(unit, newunits)
				}
			}
			w.principals = newprincipals
		case c := <-all:
			fmt.Printf("\n  !!! from All: %#v\n", changes)
			changes = w.merge(changes, c)
			fmt.Printf("\n  !!! from All+: %#v\n", changes)
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			fmt.Printf("\n  !!! delivered: %#v\n", changes)
			out = nil
			changes = nil
		}
	}
	panic("unreachable")
}
