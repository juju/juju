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
	units               map[string]*unitTracker
	unitPortsChanges    chan *unitPortsChange
	services            map[string]*serviceTracker
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(st *state.State) (*Firewaller, error) {
	fw := &Firewaller{
		st:                  st,
		machinesWatcher:     st.WatchMachines(),
		machines:            make(map[int]*machineTracker),
		machineUnitsChanges: make(chan *machineUnitsChange),
		units:               make(map[string]*unitTracker),
		unitPortsChanges:    make(chan *unitPortsChange),
		services:            make(map[string]*serviceTracker),
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
				mt, ok := fw.machines[removedMachine.Id()]
				if !ok {
					panic("trying to remove machine that wasn't added")
				}
				delete(fw.machines, removedMachine.Id())
				if err := mt.stop(); err != nil {
					log.Printf("machine tracker %d returned error when stopping: %v", removedMachine.Id(), err)
				}
				log.Debugf("firewaller: stopped tracking machine %d", removedMachine.Id())
			}
			for _, addedMachine := range change.Added {
				mt := newMachineTracker(addedMachine, fw)
				fw.machines[addedMachine.Id()] = mt
				log.Debugf("firewaller: started tracking machine %d", mt.id)
			}
		case change := <-fw.machineUnitsChanges:
			if change.change == nil {
				log.Printf("tracker of machine %d terminated prematurely: %v", change.machine.id, change.machine.stop())
				delete(fw.machines, change.machine.id)
				continue
			}
			for _, removedUnit := range change.change.Removed {
				ut, ok := fw.units[removedUnit.Name()]
				if !ok {
					panic("trying to remove unit that wasn't added")
				}
				delete(fw.units, removedUnit.Name())
				if err := ut.stop(); err != nil {
					log.Printf("unit tracker %s returned error when stopping: %v", removedUnit.Name(), err)
				}
				log.Debugf("firewaller: stopped tracking unit %s", removedUnit.Name())
			}
			for _, addedUnit := range change.change.Added {
				ut := newUnitTracker(addedUnit, fw)
				fw.units[addedUnit.Name()] = ut
				if fw.services[addedUnit.ServiceName()] == nil {
					// TODO(mue) Add service watcher.
				}
				log.Debugf("firewaller: started tracking unit %s", ut.name)
			}
		case <-fw.unitPortsChanges:
			// TODO(mue) Handle changes of ports.
		}
	}
}

// finishes cleans up when the firewaller is stopping.
func (fw *Firewaller) finish() {
	watcher.Stop(fw.machinesWatcher, &fw.tomb)
	for _, ut := range fw.units {
		fw.tomb.Kill(ut.stop())
	}
	for _, mt := range fw.machines {
		fw.tomb.Kill(mt.stop())
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

// newMachineTracker tracks unit changes to the given machine and sends them 
// to the central firewaller loop. 
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
			// Send change or nil in case of an error.
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

// unitPortsChange contains the changed ports for one specific unit. 
type unitPortsChange struct {
	unit   *unitTracker
	change []state.Port
}

// unitTracker keeps track of the port changes of a unit.
type unitTracker struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	name       string
	watcher    *state.PortsWatcher
	service    *serviceTracker
	ports      []state.Port
}

// newUnitTracker creates a new machine tracker keeping track of
// unit changes of the passed machine.
func newUnitTracker(ust *state.Unit, fw *Firewaller) *unitTracker {
	ut := &unitTracker{
		firewaller: fw,
		name:       ust.Name(),
		watcher:    ust.WatchPorts(),
		ports:      make([]state.Port, 0),
	}
	go ut.loop()
	return ut
}

func (ut *unitTracker) loop() {
	defer ut.tomb.Done()
	defer ut.watcher.Stop()
	for {
		select {
		case <-ut.tomb.Dying():
			return
		case change, ok := <-ut.watcher.Changes():
			// Send change or nil in case of an error.
			select {
			case ut.firewaller.unitPortsChanges <- &unitPortsChange{ut, change}:
			case <-ut.tomb.Dying():
				return
			}
			// The watcher terminated prematurely, so end the loop.
			if !ok {
				ut.firewaller.tomb.Kill(watcher.MustErr(ut.watcher))
				return
			}
		}
	}
}

// stop stops the unit tracker.
func (ut *unitTracker) stop() error {
	ut.tomb.Kill(nil)
	return ut.tomb.Wait()
}

// serviceTracker  keeps track of the changes of a service.
type serviceTracker struct {
	name    string
	exposed bool
}
