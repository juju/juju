package mstate

import (
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/mstate/watcher"
	"launchpad.net/tomb"
	"sort"
	"strings"
)

// commonWatcher is part of all client watchers.
type commonWatcher struct {
	st   *State
	tomb tomb.Tomb
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
// joined or departed a particular relation scope.
type RelationScopeChange struct {
	Added   []string
	Removed []string
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

func (w *MachineWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
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

// Stop stops the watcher and returns any errors encountered while watching.
func (w *MachinesWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
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

// Stop stops the watcher and returns any errors encountered while watching.
func (w *ServicesWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
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

// Stop stops the watcher and returns any errors encountered while watching.
func (w *ServiceUnitsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
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

// Changes returns a channel that will receive changes when units join and
// depart a relation scope. The Added field in the first event on the channel
// holds the initial state.
func (w *RelationScopeWatcher) Changes() <-chan *RelationScopeChange {
	return w.changeChan
}

// Stop stops the watcher and returns any errors encountered while watching.
func (w *RelationScopeWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (changes *RelationScopeChange) isEmpty() bool {
	return len(changes.Added)+len(changes.Removed) == 0
}

func (w *RelationScopeWatcher) mergeChange(changes *RelationScopeChange, ch watcher.Change) (err error) {
	doc := &relationRefDoc{ch.Id.(string)}
	if !strings.HasPrefix(doc.Key, w.prefix) {
		return nil
	}
	name := doc.UnitName()
	if name == w.ignore {
		return nil
	}
	if ch.Revno == -1 {
		if w.knownUnits[name] {
			changes.Removed = append(changes.Removed, name)
			delete(w.knownUnits, name)
		}
		return nil
	}
	if !w.knownUnits[name] {
		changes.Added = append(changes.Added, name)
		w.knownUnits[name] = true
	}
	return nil
}

func (w *RelationScopeWatcher) getInitialEvent() (initial *RelationScopeChange, err error) {
	changes := &RelationScopeChange{}
	docs := []relationRefDoc{}
	sel := D{{"_id", D{{"$regex", "^" + w.prefix}}}}
	err = w.st.relationRefs.Find(sel).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		if name := doc.UnitName(); name != w.ignore {
			changes.Added = append(changes.Added, name)
			w.knownUnits[name] = true
		}
	}
	return changes, nil
}

func (w *RelationScopeWatcher) loop() error {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.relationRefs.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.relationRefs.Name, ch)
	changes, err := w.getInitialEvent()
	if err != nil {
		return err
	}
	for {
		for changes != nil {
			sort.Strings(changes.Added)
			sort.Strings(changes.Removed)
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case c := <-ch:
				log.Printf("tick %#v", c)
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
			log.Printf("tock %#v", c)
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
