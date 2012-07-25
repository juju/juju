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
	st              *state.State
	tomb            tomb.Tomb
	machinesWatcher *state.MachinesWatcher
	machineds       map[int]*machineData
	unitsChange     chan *unitsChange
	unitds          map[string]*unitData
	portsChange     chan *portsChange
	serviceds       map[string]*serviceData
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(st *state.State) (*Firewaller, error) {
	fw := &Firewaller{
		st:              st,
		machinesWatcher: st.WatchMachines(),
		machineds:       make(map[int]*machineData),
		unitsChange:     make(chan *unitsChange),
		unitds:          make(map[string]*unitData),
		portsChange:     make(chan *portsChange),
		serviceds:       make(map[string]*serviceData),
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
			for _, machine := range change.Removed {
				machined, ok := fw.machineds[machine.Id()]
				if !ok {
					panic("trying to remove machine that wasn't added")
				}
				delete(fw.machineds, machine.Id())
				if err := machined.stopWatch(); err != nil {
					log.Printf("machine data %d returned error when stopping: %v", machine.Id(), err)
				}
				log.Debugf("firewaller: stopped watching machine %d", machine.Id())
			}
			for _, machine := range change.Added {
				machined := newMachineData(machine, fw)
				fw.machineds[machine.Id()] = machined
				log.Debugf("firewaller: started watching machine %d", machine.Id())
			}
		case change := <-fw.unitsChange:
			for _, unit := range change.Removed {
				unitd, ok := fw.unitds[unit.Name()]
				if !ok {
					panic("trying to remove unit that wasn't added")
				}
				delete(fw.unitds, unit.Name())
				// TODO(mue) Close ports.
				if err := unitd.stopWatch(); err != nil {
					log.Printf("unit watcher %q returned error when stopping: %v", unit.Name(), err)
				}
				log.Debugf("firewaller: stopped watching unit %s", unit.Name())
			}
			for _, unit := range change.Added {
				unitd := newUnitData(unit, fw)
				fw.unitds[unit.Name()] = unitd
				if fw.serviceds[unit.ServiceName()] == nil {
					// TODO(mue) Add service watcher.
				}
				log.Debugf("firewaller: started watching unit %s", unit.Name())
			}
		case <-fw.portsChange:
			// TODO(mue) Handle changes of ports.
		}
	}
}

// finishes cleans up when the firewaller is stopping.
func (fw *Firewaller) finish() {
	watcher.Stop(fw.machinesWatcher, &fw.tomb)
	for _, unitd := range fw.unitds {
		fw.tomb.Kill(unitd.stopWatch())
	}
	for _, machined := range fw.machineds {
		fw.tomb.Kill(machined.stopWatch())
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

// unitsChange contains the changed units for one specific machine. 
type unitsChange struct {
	machined *machineData
	*state.MachineUnitsChange
}

// machineData watches the unit changes of a machine and passes them
// to the firewaller for handling.
type machineData struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	machine    *state.Machine
	watcher    *state.MachineUnitsWatcher
}

// newMachineData starts the watching of the passed machine. 
func newMachineData(machine *state.Machine, fw *Firewaller) *machineData {
	md := &machineData{
		firewaller: fw,
		machine:    machine,
		watcher:    machine.WatchUnits(),
	}
	go md.watchLoop()
	return md
}

// watchLoop is the backend watching for machine units changes.
func (md *machineData) watchLoop() {
	defer md.tomb.Done()
	defer md.watcher.Stop()
	for {
		select {
		case <-md.tomb.Dying():
			return
		case change, ok := <-md.watcher.Changes():
			if !ok {
				md.firewaller.tomb.Kill(watcher.MustErr(md.watcher))
				return
			}
			select {
			case md.firewaller.unitsChange <- &unitsChange{md, change}:
			case <-md.tomb.Dying():
				return
			}
		}
	}
}

// stopWatch stops the machine watching.
func (md *machineData) stopWatch() error {
	md.tomb.Kill(nil)
	return md.tomb.Wait()
}

// portsChange contains the changed ports for one specific unit. 
type portsChange struct {
	unitd *unitData
	ports []state.Port
}

// unitData watches the port changes of a unit and passes them
// to the firewaller for handling.
type unitData struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	unit       *state.Unit
	watcher    *state.PortsWatcher
	service    *serviceData
	ports      []state.Port
}

// newMachineData starts the watching of the passed unit. 
func newUnitData(unit *state.Unit, fw *Firewaller) *unitData {
	ud := &unitData{
		firewaller: fw,
		unit:       unit,
		watcher:    unit.WatchPorts(),
		ports:      make([]state.Port, 0),
	}
	go ud.watchLoop()
	return ud
}

func (ud *unitData) watchLoop() {
	defer ud.tomb.Done()
	defer ud.watcher.Stop()
	for {
		select {
		case <-ud.tomb.Dying():
			return
		case change, ok := <-ud.watcher.Changes():
			if !ok {
				ud.firewaller.tomb.Kill(watcher.MustErr(ud.watcher))
				return
			}
			select {
			case ud.firewaller.portsChange <- &portsChange{ud, change}:
			case <-ud.tomb.Dying():
				return
			}
		}
	}
}

// stopWatch stops the unit watching.
func (ud *unitData) stopWatch() error {
	ud.tomb.Kill(nil)
	return ud.tomb.Wait()
}

// serviceData watches the exposed flag changes of a service and passes them
// to the firewaller for handling.
type serviceData struct {
	// TODO(mue) Fill with life.
	service *state.Service
	exposed bool
}
