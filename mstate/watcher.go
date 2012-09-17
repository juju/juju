package mstate

import (
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/mstate/watcher"
	"launchpad.net/tomb"
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

func (w *MachinesWatcher) appendChange(changes *MachinesChange, ch watcher.Change) (err error) {
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
		if changes == nil {
			select {
			case <-w.st.watcher.Dead():
				return watcher.MustErr(w.st.watcher)
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case c := <-ch:
				changes = &MachinesChange{}
				err := w.appendChange(changes, c)
				if err != nil {
					return err
				}
				if changes.isEmpty() {
					changes = nil
				}
				continue
			}
		}
		select {
		case <-w.st.watcher.Dead():
			return watcher.MustErr(w.st.watcher)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-ch:
			err := w.appendChange(changes, c)
			if err != nil {
				return err
			}
		case w.changeChan <- changes:
			changes = nil
		}
	}
	return nil
}
