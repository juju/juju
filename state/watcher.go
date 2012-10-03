package state

import (
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
	"strings"
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

// MachinesWatcher notifies about lifecycle changes for all machines
// in the environment.
// 
// The first event emitted will contain the ids of all machines found
// irrespective of their life state. From then on a new event is emitted
// whenever one or more machines are added or change their lifecycle.
//
// After a machine is found to be Dead, no further event will include it.
type MachinesWatcher struct {
	commonWatcher
	out  chan []int
	life map[int]Life
}

var lifeFields = D{{"_id", 1}, {"life", 1}}

// WatchMachines returns a new MachinesWatcher.
func (s *State) WatchMachines() *MachinesWatcher {
	return newMachinesWatcher(s)
}

// WatchMachines returns a new MachinesWatcher.
func newMachinesWatcher(st *State) *MachinesWatcher {
	w := &MachinesWatcher{
		commonWatcher: commonWatcher{st: st},
		out:           make(chan []int),
		life:          make(map[int]Life),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for the MachinesWatcher.
func (w *MachinesWatcher) Changes() <-chan []int {
	return w.out
}

func (w *MachinesWatcher) initial() (ids []int, err error) {
	iter := w.st.machines.Find(nil).Select(lifeFields).Iter()
	var doc machineDoc
	for iter.Next(&doc) {
		ids = append(ids, doc.Id)
		w.life[doc.Id] = doc.Life
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (w *MachinesWatcher) merge(ids []int, ch watcher.Change) ([]int, error) {
	id := ch.Id.(int)
	for _, pending := range ids {
		if id == pending {
			return ids, nil
		}
	}
	if ch.Revno == -1 {
		if life, ok := w.life[id]; ok && life != Dead {
			ids = append(ids, id)
		}
		delete(w.life, id)
		return ids, nil
	}
	doc := machineDoc{Id: id, Life: Dead}
	err := w.st.machines.FindId(id).Select(lifeFields).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, err
	}
	if life, ok := w.life[id]; !ok || doc.Life != life {
		ids = append(ids, id)
		if err != mgo.ErrNotFound {
			w.life[id] = doc.Life
		}
	}
	return ids, nil
}

func (w *MachinesWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.machines.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.machines.Name, ch)
	ids, err := w.initial()
	if err != nil {
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
			if ids, err = w.merge(ids, c); err != nil {
				return err
			}
			if len(ids) > 0 {
				out = w.out
			}
		case out <- ids:
			ids = nil
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

// ServiceUnitsWatcher notifies about the lifecycle changes of the units
// belonging to the service. The first event returned by the watcher is the
// set of names of all units that are part of the service, irrespective
// of their life state. Subsequent events return batches of newly added
// units and units which have changed their lifecycle, up to when they are
// found dead.
type ServiceUnitsWatcher2 struct {
	commonWatcher
	service *Service
	in      chan watcher.Change
	out     chan []string
	known   map[string]Life
}

// Changes returns the event channel for w.
func (w *ServiceUnitsWatcher2) Changes() <-chan []string {
	return w.out
}

// WatchUnits returns a new ServiceUnitsWatcher for s.
func (s *Service) WatchUnits2() *ServiceUnitsWatcher2 {
	return newServiceUnitsWatcher2(s)
}

func newServiceUnitsWatcher2(svc *Service) *ServiceUnitsWatcher2 {
	w := &ServiceUnitsWatcher2{
		commonWatcher: commonWatcher{st: svc.st},
		service:       svc,
		known:         make(map[string]Life),
		in:            make(chan watcher.Change),
		out:           make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (w *ServiceUnitsWatcher2) initial() (changes []string, err error) {
	return w.updateService(changes)
}

func (w *ServiceUnitsWatcher2) updateService(pending []string) (changes []string, err error) {
	doc := serviceDoc{}
	err = w.st.services.FindId(w.service.doc.Name).One(&doc)
	if err != nil {
		return nil, err
	}
	for _, unit := range doc.Units {
		if _, ok := w.known[unit]; !ok {
			pending, err = w.merge(pending, unit)
			if err != nil {
				return nil, err
			}
		}
	}
	return pending, nil
}

func (w *ServiceUnitsWatcher2) merge(pending []string, name string) (changes []string, err error) {
	doc := unitDoc{}
	err = w.st.units.FindId(name).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, err
	}
	if err == mgo.ErrNotFound {
		w.st.watcher.Unwatch(w.st.units.Name, name, w.in)
		if w.known[name] != Dead {
			return append(pending, name), nil
		}
	}
	if w.known[name] == doc.Life {
		return pending, nil
	}
	w.known[name] = doc.Life
	for _, v := range pending {
		if name == v {
			return pending, nil
		}
	}
	return append(pending, name), nil
}

func (w *ServiceUnitsWatcher2) loop() (err error) {
	serviceCh := make(chan watcher.Change)
	w.st.watcher.Watch(w.st.services.Name, w.service.doc.Name, w.service.doc.TxnRevno, serviceCh)
	defer w.st.watcher.Unwatch(w.st.services.Name, w.service.doc.Name, serviceCh)
	defer func() {
		for _, name := range w.known {
			w.st.watcher.Unwatch(w.st.units.Name, name, w.in)
		}
	}()
	changes, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-serviceCh:
			changes, err = w.updateService(changes)
			if err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.out
			}
		case c := <-w.in:
			name := c.Id.(string)
			changes, err = w.merge(changes, name)
			if err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			out = nil
			changes = nil
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
