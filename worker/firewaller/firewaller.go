package firewaller

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// Firewaller watches the state for ports opened or closed
// and reflects those changes onto the backing environment.
type Firewaller struct {
	environ               environs.Environ
	st                    *state.State
	tomb                  tomb.Tomb
	machinesWatcher       *state.MachinesWatcher
	machines              map[int]*machineTracker
	machineUnitsChanges   chan *machineUnitsChange
	units                 map[string]*unitTracker
	unitPortsChanges      chan *unitPortsChange
	services              map[string]*serviceTracker
	serviceExposedChanges chan *serviceExposedChange
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(environ environs.Environ, st *state.State) (*Firewaller, error) {
	fw := &Firewaller{
		environ:               environ,
		st:                    st,
		machinesWatcher:       st.WatchMachines(),
		machines:              make(map[int]*machineTracker),
		machineUnitsChanges:   make(chan *machineUnitsChange),
		units:                 make(map[string]*unitTracker),
		unitPortsChanges:      make(chan *unitPortsChange),
		services:              make(map[string]*serviceTracker),
		serviceExposedChanges: make(chan *serviceExposedChange),
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
		case change, ok := <-fw.machineUnitsChanges:
			if !ok {
				panic("aggregation of machine units changes failed")
			}
			if change.change == nil {
				log.Printf("tracker of machine %d terminated prematurely", change.machine.id)
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
				ut := newUnitTracker(addedUnit, change.machine.id, fw)
				fw.units[addedUnit.Name()] = ut
				if fw.services[addedUnit.ServiceName()] == nil {
					service, err := fw.st.Service(addedUnit.ServiceName())
					if err != nil {
						// TODO(mue) Check if panic is too hard.
						panic(fmt.Sprintf("service state %q can't be retrieved: %v", addedUnit.ServiceName(), err))
					}
					st := newServiceTracker(service, fw)
					ut.service = st
					fw.services[addedUnit.ServiceName()] = st
				}
				log.Debugf("firewaller: started tracking unit %s", ut.name)
			}
		case change := <-fw.unitPortsChanges:
			mt, ok := fw.machines[change.unitTracker.machineId]
			if !ok {
				panic("machine for unit ports change isn't tracked")
			}
			for _, port := range change.change {
				if mt.ports[port] == nil && change.unitTracker.service.exposed {
					if err := fw.openPortOnAllInstances(mt.id, port); err != nil {
						log.Printf("can't open port %v on machine %d: %v", port, mt.id, err)
					}
					log.Debugf("firewaller: opened port %v on machine %d", port, mt.id)
				}
				mt.ports[port] = change.unitTracker
			}
			for _, port := range change.unitTracker.ports {
				if mt.ports[port] == change.unitTracker {
					if change.unitTracker.service.exposed {
						if err := fw.closePortOnAllInstances(mt.id, port); err != nil {
							log.Printf("can't close port %v on machine %d: %v", port, mt.id, err)
						}
						log.Debugf("firewaller: closed port %v on machine %d", port, mt.id)
					}
				}
				delete(mt.ports, port)
			}
			change.unitTracker.ports = change.change
		case change := <-fw.serviceExposedChanges:
			if change.terminated == true {
				log.Printf("tracker of service %q terminated prematurely", change.st.name)
				delete(fw.services, change.st.name)
				continue
			}
			if change.st.exposed == change.exposed {
				break
			}
			change.st.exposed = change.exposed
			for _, mt := range fw.machines {
				for _, ut := range mt.ports {
					switch {
					case ut.service != change.st:
						continue
					case ut.service.exposed:
						for _, port := range ut.ports {
							if err := fw.openPortOnAllInstances(mt.id, port); err != nil {
								log.Printf("can't open port %v on machine %d: %v", port, mt.id, err)
							}
							log.Debugf("firewaller: opened port %v on machine %d", port, mt.id)
						}
					default:
						for _, port := range ut.ports {
							if err := fw.closePortOnAllInstances(mt.id, port); err != nil {
								log.Printf("can't close port %v on machine %d: %v", port, mt.id, err)
							}
							log.Debugf("firewaller: closed port %v on machine %d", port, mt.id)
						}
					}
				}
			}
		}
	}
}

// openPortOnAllInstances opens the passed port on all instances in the environment.
func (fw *Firewaller) openPortOnAllInstances(machineId int, port state.Port) error {
	instances, err := fw.environ.AllInstances()
	if err != nil {
		return err
	}
	for _, instance := range instances {
		err = instance.OpenPorts(machineId, []state.Port{port})
		if err != nil {
			// TODO(mue) Add a retry logic later.
			return err
		}
	}
	return nil
}

// closePortOnAllInstances closes the passed port on all instances in the environment. 
func (fw *Firewaller) closePortOnAllInstances(machineId int, port state.Port) error {
	instances, err := fw.environ.AllInstances()
	if err != nil {
		return err
	}
	for _, instance := range instances {
		err = instance.ClosePorts(machineId, []state.Port{port})
		if err != nil {
			// TODO(mue) Add a retry logic later.
			return err
		}
	}
	return nil
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
	unitTracker *unitTracker
	change      []state.Port
}

// unitTracker keeps track of the port changes of a unit.
type unitTracker struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	machineId  int
	name       string
	watcher    *state.PortsWatcher
	service    *serviceTracker
	ports      []state.Port
}

// newUnitTracker creates a new machine tracker keeping track of
// unit changes of the passed machine.
func newUnitTracker(ust *state.Unit, machineId int, fw *Firewaller) *unitTracker {
	ut := &unitTracker{
		firewaller: fw,
		machineId:  machineId,
		name:       ust.Name(),
		watcher:    ust.WatchPorts(),
		ports:      make([]state.Port, 0),
	}
	go ut.loop()
	return ut
}

// loop is the backend watching for unit ports changes.
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

// serviceExposedChange contains the changed exposed flag for one specific service. 
type serviceExposedChange struct {
	st         *serviceTracker
	exposed    bool
	terminated bool
}

// serviceTracker keeps track of the changes of a service.
type serviceTracker struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	name       string
	watcher    *state.FlagWatcher
	exposed    bool
}

// newUnitTracker creates a new service tracker keeping track of
// exposed flag changes of the passed service.
func newServiceTracker(sst *state.Service, fw *Firewaller) *serviceTracker {
	st := &serviceTracker{
		firewaller: fw,
		name:       sst.Name(),
		watcher:    sst.WatchExposed(),
	}
	isExposed, err := sst.IsExposed()
	if err != nil {
		panic(fmt.Sprintf("can't retrieve exposed state of service %q: %v", sst.Name(), err))
	}
	st.exposed = isExposed
	go st.loop()
	return st
}

// loop is the backend watching for service exposed flag changes.
func (st *serviceTracker) loop() {
	defer st.tomb.Done()
	defer st.watcher.Stop()
	for {
		select {
		case <-st.tomb.Dying():
			return
		case change, ok := <-st.watcher.Changes():
			// Send change together with flag for premature termination.
			select {
			case st.firewaller.serviceExposedChanges <- &serviceExposedChange{st, change, !ok}:
			case <-st.tomb.Dying():
				return
			}
			// The watcher terminated prematurely, so end the loop.
			if !ok {
				st.firewaller.tomb.Kill(watcher.MustErr(st.watcher))
				return
			}
		}
	}
}

// stop stops the service tracker.
func (st *serviceTracker) stop() error {
	st.tomb.Kill(nil)
	return st.tomb.Wait()
}
