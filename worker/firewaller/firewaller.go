package firewaller

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// Firewaller watches the state for ports open or closed
// and reflects those changes onto the backing environment.
type Firewaller struct {
	st       *state.State
	environ  environs.Environ
	tomb     tomb.Tomb
	machines map[int]*machineTracker
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(environ environs.Environ) (*Firewaller, error) {
	info, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	st, err := state.Open(info)
	if err != nil {
		return nil, err
	}
	fw := &Firewaller{
		st:       st,
		environ:  environ,
		machines: make(map[int]*machineTracker),
	}
	go fw.loop()
	return fw, nil
}

func (fw *Firewaller) loop() {
	defer fw.finish()
	// Set up channels and watchers.
	machineUnitsChanges := make(chan *machineUnitsChange)
	defer close(machineUnitsChanges)
	machinesWatcher := fw.st.WatchMachines()
	defer watcher.Stop(machinesWatcher, &fw.tomb)
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case change, ok := <-machinesWatcher.Changes():
			if !ok {
				return
			}
			for _, removedMachine := range change.Removed {
				m, ok := fw.machines[removedMachine.Id()]
				if !ok {
					panic("trying to remove machine that wasn't added")
				}
				if err := m.stop(); err != nil {
					log.Printf("can't stop machine tracker: %v", err)
					continue
				}
				delete(fw.machines, removedMachine.Id())
			}
			for _, addedMachine := range change.Added {
				m := newMachineTracker(addedMachine, fw, machineUnitsChanges)
				fw.machines[addedMachine.Id()] = m
				log.Debugf("Added machine %v", m.id)
			}
		case <-machineUnitsChanges:
			// TODO(mue) fill with life.
		}
	}
}

// finishes cleans up when the firewaller is stopping.
func (fw *Firewaller) finish() {
	for _, m := range fw.machines {
		fw.tomb.Kill(m.stop())
	}
	fw.st.Close()
	fw.tomb.Done()
}

// Wait waits for the Firewaller to exit.
func (fw *Firewaller) Wait() error {
	return fw.tomb.Wait()
}

// Stop stops the Firewaller and returns any error encountered while stopping.
func (fw *Firewaller) Stop() error {
	fw.tomb.Kill(nil)
	return fw.tomb.Wait()
}

// machineUnitsChange contains the changed units for one specific machine. 
type machineUnitsChange struct {
	machine *machineTracker
	change  *state.MachineUnitsChange
}

// machineTracker keeps track of the unit changes of a machine.
type machineTracker struct {
	firewaller *Firewaller
	changes    chan<- *machineUnitsChange
	tomb       tomb.Tomb
	id         int
	watcher    *state.MachineUnitsWatcher
	ports      map[state.Port]*unitTracker
}

// newMachineTracker creates a new machine tracker keeping track of
// unit changes of the passed machine.
func newMachineTracker(mst *state.Machine, fw *Firewaller, changes chan<- *machineUnitsChange) *machineTracker {
	mt := &machineTracker{
		firewaller: fw,
		changes:    changes,
		id:         mst.Id(),
		watcher:    mst.WatchUnits(),
		ports:      make(map[state.Port]*unitTracker),
	}
	go mt.loop()
	return mt
}

// loop is the backend watching for machine units changes.
func (mt *machineTracker) loop() {
	defer mt.tomb.Done()
	defer mt.watcher.Stop()
	for {
		select {
		case <-mt.firewaller.tomb.Dying():
			return
		case <-mt.tomb.Dying():
			return
		case change, ok := <-mt.watcher.Changes():
			// Send change or nil.
			select {
			case mt.changes <- &machineUnitsChange{mt, change}:
			case <-mt.firewaller.tomb.Dying():
				return
			case <-mt.tomb.Dying():
				return
			}
			// Had been an error, so end the loop.
			if !ok {
				mt.firewaller.tomb.Kill(watcher.MustErr(mt.watcher))
				return
			}
		}
	}
}

// stop stops the machine tracker.
func (mt *machineTracker) stop() error {
	mt.tomb.Kill(nil)
	return mt.tomb.Wait()
}

type serviceTracker struct {
	exposed bool
}

type unitTracker struct {
	service *serviceTracker
	id      string
	ports   []state.Port
}
