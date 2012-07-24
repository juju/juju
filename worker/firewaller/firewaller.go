package firewaller

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// Firewaller watches the state for ports opened or closed
// and reflects those changes onto the backing environment.
type Firewaller struct {
	environ             environs.Environ
	st                  *state.State
	tomb                tomb.Tomb
	machinesWatcher     *state.MachinesWatcher
	machineDatas        map[int]*machineData
	machineUnitsChanges chan *machineUnitsChange
	unitDatas           map[string]*unitData
	unitPortsChanges    chan *unitPortsChange
	serviceDatas        map[string]*serviceData
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(environ environs.Environ, st *state.State) (*Firewaller, error) {
	fw := &Firewaller{
		environ:             environ,
		st:                  st,
		machinesWatcher:     st.WatchMachines(),
		machineDatas:        make(map[int]*machineData),
		machineUnitsChanges: make(chan *machineUnitsChange),
		unitDatas:           make(map[string]*unitData),
		unitPortsChanges:    make(chan *unitPortsChange),
		serviceDatas:        make(map[string]*serviceData),
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
				machined, ok := fw.machineDatas[machine.Id()]
				if !ok {
					panic("trying to remove machine that wasn't added")
				}
				delete(fw.machineDatas, machine.Id())
				if err := machined.stopWatch(); err != nil {
					log.Printf("machine data %d returned error when stopping: %v", machine.Id(), err)
				}
				log.Debugf("firewaller: stopped watching machine %d", machine.Id())
			}
			for _, machine := range change.Added {
				fw.machineDatas[machine.Id()] = newMachineData(machine, fw)
				log.Debugf("firewaller: started watching machine %d", machine.Id())
			}
		case change := <-fw.machineUnitsChanges:
			for _, unit := range change.Removed {
				unitd, ok := fw.unitDatas[unit.Name()]
				if !ok {
					panic("trying to remove unit that wasn't added")
				}
				delete(fw.unitDatas, unit.Name())
				// TODO(mue) Close ports.
				if err := unitd.stopWatch(); err != nil {
					log.Printf("unit data %s returned error when stopping: %v", unit.Name(), err)
				}
				log.Debugf("firewaller: stopped watching unit %s", unit.Name())
			}
			for _, unit := range change.Added {
				unitd := newUnitData(unit, fw)
				fw.unitDatas[unit.Name()] = unitd
				if fw.serviceDatas[unit.ServiceName()] == nil {
					service, err := fw.st.Service(unit.ServiceName())
					if err != nil {
						fw.tomb.Killf("service state %q can't be retrieved: %v", unit.ServiceName(), err)
						return
					}
					fw.serviceDatas[unit.ServiceName()] = newServiceData(service, fw)

				}
				log.Debugf("firewaller: started watching unit %s", unit.Name())
			}
		case change := <-fw.unitPortsChanges:
			machineId, err := change.unitd.unit.AssignedMachineId()
			if err != nil {
				fw.tomb.Killf("cannot retrieve assigned machine id of unit %q: %v", change.unitd.unit, err)
				return
			}
			machined, ok := fw.machineDatas[machineId]
			if !ok {
				panic("machine for unit ports change isn't watched")
			}
			if err = fw.openClosePorts(machined); err != nil {
				fw.tomb.Killf("cannot open and close ports on machine %d: %v", machineId, err)
				return
			}
		}
	}
}

// openClosePorts checks for needed opening and closing of ports and performs it. 
func (fw *Firewaller) openClosePorts(machined *machineData) (err error) {
	toOpen, toClose, err := machined.checkPorts()
	if err != nil {
		return err
	}
	for _, port := range toOpen {
		if err = fw.openPort(machined, port); err != nil {
			return err
		}
	}
	for _, port := range toClose {
		if err = fw.closePort(machined, port); err != nil {
			return err
		}
	}
	return nil
}

// openPort opens the passed port on the instances for the passed machine.
func (fw *Firewaller) openPort(machined *machineData, port state.Port) (err error) {
	defer log.Debugf("firewaller: opened port %v on machine %d", port, machined.machine.Id())
	instanceId, err := machined.machine.InstanceId()
	if err != nil {
		return err
	}
	instances, err := fw.environ.Instances([]string{instanceId})
	if err != nil {
		return err
	}
	err = instances[0].OpenPorts(machined.machine.Id(), []state.Port{port})
	if err != nil {
		// TODO(mue) Add a retry logic later.
		return err
	}
	return nil
}

// openPort closes the passed port on the instances for the passed machine.
func (fw *Firewaller) closePort(machined *machineData, port state.Port) (err error) {
	defer log.Debugf("firewaller: closed port %v on machine %d", port, machined.machine.Id())
	instanceId, err := machined.machine.InstanceId()
	if err != nil {
		return err
	}
	instances, err := fw.environ.Instances([]string{instanceId})
	if err != nil {
		return err
	}
	err = instances[0].ClosePorts(machined.machine.Id(), []state.Port{port})
	if err != nil {
		// TODO(mue) Add a retry logic later.
		return err
	}
	return nil
}

// finishes cleans up when the firewaller is stopping.
func (fw *Firewaller) finish() {
	watcher.Stop(fw.machinesWatcher, &fw.tomb)
	for _, unitd := range fw.unitDatas {
		fw.tomb.Kill(unitd.stopWatch())
	}
	for _, machined := range fw.machineDatas {
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

// machineUnitsChange contains the changed units for one specific machine. 
type machineUnitsChange struct {
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
	ports      []state.Port
}

// newMachineData starts the watching of the passed machine. 
func newMachineData(machine *state.Machine, fw *Firewaller) *machineData {
	md := &machineData{
		firewaller: fw,
		machine:    machine,
		watcher:    machine.WatchUnits(),
		ports:      []state.Port{},
	}
	go md.watchLoop()
	return md
}

// checkPorts retrieves the ports that have to be open and compares it
// to the current open ports.
func (md *machineData) checkPorts() (toOpen []state.Port, toClose []state.Port, err error) {
	want := []state.Port{}
	units, err := md.machine.Units()
	if err != nil {
		return nil, nil, err
	}
	for _, unit := range units {
		service, err := md.firewaller.st.Service(unit.ServiceName())
		if err != nil {
			return nil, nil, err
		}
		isExposed, err := service.IsExposed()
		if err != nil {
			return nil, nil, err
		}
		if isExposed {
			ports, err := unit.OpenPorts()
			if err != nil {
				return nil, nil, err
			}
			want = append(want, ports...)
		}
	}
	toOpen = diff(want, md.ports)
	toClose = diff(md.ports, want)
	md.ports = want
	return
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
			case md.firewaller.machineUnitsChanges <- &machineUnitsChange{md, change}:
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

// unitPortsChange contains the changed ports for one specific unit. 
type unitPortsChange struct {
	unitd *unitData
}

// unitData watches the port changes of a unit and passes them
// to the firewaller for handling.
type unitData struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	unit       *state.Unit
	watcher    *state.PortsWatcher
	service    *serviceData
}

// newMachineData starts the watching of the passed unit. 
func newUnitData(unit *state.Unit, fw *Firewaller) *unitData {
	ud := &unitData{
		firewaller: fw,
		unit:       unit,
		watcher:    unit.WatchPorts(),
	}
	go ud.watchLoop()
	return ud
}

// watchLoop is the backend watching for machine units changes.
func (ud *unitData) watchLoop() {
	defer ud.tomb.Done()
	defer ud.watcher.Stop()
	for {
		select {
		case <-ud.tomb.Dying():
			return
		case _, ok := <-ud.watcher.Changes():
			if !ok {
				ud.firewaller.tomb.Kill(watcher.MustErr(ud.watcher))
				return
			}
			select {
			case ud.firewaller.unitPortsChanges <- &unitPortsChange{ud}:
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
	tomb       tomb.Tomb
	firewaller *Firewaller
	service    *state.Service
}

// newServiceData starts the watching of the passed service. 
func newServiceData(service *state.Service, fw *Firewaller) *serviceData {
	sd := &serviceData{
		firewaller: fw,
		service:    service,
	}
	// TODO(mue) Start backend watch loop.
	return sd
}

// diff returns all the ports that exist in A but not B.
func diff(A, B []state.Port) (missing []state.Port) {
next:
	for _, a := range A {
		for _, b := range B {
			if a.Protocol == b.Protocol && a.Number == b.Number {
				continue next
			}
		}
		missing = append(missing, a)
	}
	return
}
