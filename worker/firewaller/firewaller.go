package firewaller

import (
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// Firewaller watches the state for ports opened or closed
// and reflects those changes onto the backing environment.
type Firewaller struct {
	st                  *state.State
	tomb                tomb.Tomb
	machinesWatcher     *state.MachinesWatcher
	machines            map[int]*machineTracker
	machineUnitsChanges chan *machineUnitsChange
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(st *state.State) (*Firewaller, error) {
	fw := &Firewaller{
		st:                  st,
		machinesWatcher:     st.WatchMachines(),
		machines:            make(map[int]*machineTracker),
		machineUnitsChanges: make(chan *machineUnitsChange),
	}
	go fw.loop()
	return fw, nil
}

func (fw *Firewaller) loop() {
	defer fw.finish()
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				return
			}
			for _, removedMachine := range change.Removed {
				m, ok := fw.machines[removedMachine.Id()]
				if !ok {
					panic("trying to remove machine that wasn't added")
				}
				delete(fw.machines, removedMachine.Id())
				if err := m.stop(); err != nil {
					log.Printf("machine tracker %d returned error when stopping: %v", removedMachine.Id(), err)
				}
				log.Debugf("firewaller: stopped tracking machine %d", removedMachine.Id())
			}
			for _, addedMachine := range change.Added {
				m := newMachineTracker(addedMachine, fw)
				fw.machines[addedMachine.Id()] = m
				log.Debugf("firewaller: started tracking machine %d", m.id)
			}
		case <-fw.machineUnitsChanges:
			// TODO(mue) fill with life.
		}
	}
}

// finishes cleans up when the firewaller is stopping.
func (fw *Firewaller) finish() {
	watcher.Stop(fw.machinesWatcher, &fw.tomb)
	for _, m := range fw.machines {
		fw.tomb.Kill(m.stop())
	}
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
	tomb       tomb.Tomb
	firewaller *Firewaller
	id         int
	watcher    *state.MachineUnitsWatcher
	ports      map[state.Port]*unitTracker
}

// newMachineTracker creates a new machine tracker keeping track of
// unit changes of the passed machine.
func newMachineTracker(mst *state.Machine, fw *Firewaller) *machineTracker {
	mt := &machineTracker{
		firewaller: fw,
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
		case <-mt.tomb.Dying():
			return
		case change, ok := <-mt.watcher.Changes():
			// Send change or nil.
			select {
			case mt.firewaller.machineUnitsChanges <- &machineUnitsChange{mt, change}:
			case <-mt.tomb.Dying():
				return
			}
			// The watcher terminated prematurely, so end the loop.
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

type unitTracker struct {
	service *serviceTracker
	id      string
	ports   []state.Port
}

type serviceTracker struct {
	exposed bool
}
