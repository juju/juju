package firewaller

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// Firewaller watches the state for ports opened or closed
// and reflects those changes onto the backing environment.
type Firewaller struct {
	tomb            tomb.Tomb
	st              *state.State
	environ         environs.Environ
	environWatcher  *state.ConfigWatcher
	machinesWatcher *state.MachinesWatcher
	machineds       map[int]*machineData
	unitsChange     chan *unitsChange
	unitds          map[string]*unitData
	portsChange     chan *portsChange
	serviceds       map[string]*serviceData
	exposedChange   chan *exposedChange
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(st *state.State) (*Firewaller, error) {
	fw := &Firewaller{
		st:              st,
		environWatcher:  st.WatchEnvironConfig(),
		machinesWatcher: st.WatchMachines(),
		machineds:       make(map[int]*machineData),
		unitsChange:     make(chan *unitsChange),
		unitds:          make(map[string]*unitData),
		portsChange:     make(chan *portsChange),
		serviceds:       make(map[string]*serviceData),
		exposedChange:   make(chan *exposedChange),
	}
	go fw.loop()
	return fw, nil
}

func (fw *Firewaller) loop() {
	defer fw.finish()

Loop:
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case config, ok := <-fw.environWatcher.Changes():
			if !ok {
				return
			}
			var err error
			fw.environ, err = environs.NewFromAttrs(config.Map())
			if err != nil {
				log.Printf("firewaller loaded invalid environment configuration: %v", err)
				continue
			}
			log.Printf("firewaller loaded new environment configuration")
			break Loop
		}
	}

	for {
		select {
		case <-fw.tomb.Dying():
			return
		case change, ok := <-fw.environWatcher.Changes():
			if !ok {
				return
			}
			config, err := config.New(change.Map())
			if err != nil {
				log.Printf("firewaller loaded invalid environment configuration: %v", err)
				continue
			}
			fw.environ.SetConfig(config)
			log.Printf("firewaller loaded new environment configuration")
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				return
			}
			for _, machine := range change.Removed {
				machined, ok := fw.machineds[machine.Id()]
				if !ok {
					panic("trying to remove machine that was not added")
				}
				delete(fw.machineds, machine.Id())
				if err := machined.stopWatch(); err != nil {
					log.Printf("machine data %d returned error when stopping: %v", machine.Id(), err)
				}
				log.Debugf("firewaller: stopped watching machine %d", machine.Id())
			}
			for _, machine := range change.Added {
				fw.machineds[machine.Id()] = newMachineData(machine, fw)
				log.Debugf("firewaller: started watching machine %d", machine.Id())
			}
		case change := <-fw.unitsChange:
			changed := []*unitData{}
			for _, unit := range change.Removed {
				unitd, ok := fw.unitds[unit.Name()]
				if !ok {
					panic("trying to remove unit that was not added")
				}
				delete(fw.unitds, unit.Name())
				delete(unitd.machined.unitds, unit.Name())
				delete(unitd.serviced.unitds, unit.Name())
				changed = append(changed, unitd)
				if err := unitd.stopWatch(); err != nil {
					log.Printf("unit watcher %q returned error when stopping: %v", unit.Name(), err)
				}
				log.Debugf("firewaller: stopped watching unit %s", unit.Name())
			}
			for _, unit := range change.Added {
				unitd := newUnitData(unit, fw)
				fw.unitds[unit.Name()] = unitd
				machineId, err := unit.AssignedMachineId()
				if err != nil {
					fw.tomb.Kill(err)
				}
				if fw.machineds[machineId] == nil {
					panic("machine of added unit is not watched")
				}
				unitd.machined = fw.machineds[machineId]
				unitd.machined.unitds[unit.Name()] = unitd
				if fw.serviceds[unit.ServiceName()] == nil {
					service, err := fw.st.Service(unit.ServiceName())
					if err != nil {
						fw.tomb.Kill(err)
						return
					}
					fw.serviceds[unit.ServiceName()] = newServiceData(service, fw)
				}
				unitd.serviced = fw.serviceds[unit.ServiceName()]
				unitd.serviced.unitds[unit.ServiceName()] = unitd
				fw.serviceds[unit.ServiceName()].unitds[unit.Name()] = unitd
				changed = append(changed, unitd)
				log.Debugf("firewaller: started watching unit %s", unit.Name())
			}
			if err := fw.flushUnits(changed); err != nil {
				fw.tomb.Killf("cannot change firewall ports: %v", err)
				return
			}
		case change := <-fw.portsChange:
			change.unitd.ports = change.ports
			if err := fw.flushUnits([]*unitData{change.unitd}); err != nil {
				fw.tomb.Killf("cannot change firewall ports: %v", err)
				return
			}
		case change := <-fw.exposedChange:
			change.serviced.exposed = change.exposed
			unitds := []*unitData{}
			for _, unitd := range change.serviced.unitds {
				unitds = append(unitds, unitd)
			}
			if err := fw.flushUnits(unitds); err != nil {
				fw.tomb.Killf("cannot change firewall ports: %v", err)
				return
			}
		}
	}
}

// flushUnits opens and closes ports for the passed unit data.
func (fw *Firewaller) flushUnits(unitds []*unitData) error {
	machineds := map[int]*machineData{}
	for _, unitd := range unitds {
		machineds[unitd.machined.machine.Id()] = unitd.machined
	}
	for _, machined := range machineds {
		if err := fw.flushMashine(machined); err != nil {
			return err
		}
	}
	return nil
}

// flushMashine opens and closes ports for the passed machine.
func (fw *Firewaller) flushMashine(machined *machineData) error {
	// Gather ports to open and close.
	ports := map[state.Port]bool{}
	for _, unitd := range machined.unitds {
		if unitd.serviced.exposed {
			for _, port := range unitd.ports {
				ports[port] = true
			}
		}
	}
	want := []state.Port{}
	for port := range ports {
		want = append(want, port)
	}
	toOpen := diff(want, machined.ports)
	toClose := diff(machined.ports, want)
	machined.ports = want
	// Open and close the ports.
	instanceId, err := machined.machine.InstanceId()
	if err != nil {
		return err
	}
	instances, err := fw.environ.Instances([]string{instanceId})
	if err != nil {
		return err
	}
	if len(toOpen) > 0 {
		err = instances[0].OpenPorts(machined.machine.Id(), toOpen)
		if err != nil {
			// TODO(mue) Add a retry logic later.
			return err
		}
		state.SortPorts(toOpen)
		log.Printf("firewaller: opened ports %v on machine %d", toOpen, machined.machine.Id())
	}
	if len(toClose) > 0 {
		err = instances[0].ClosePorts(machined.machine.Id(), toClose)
		if err != nil {
			// TODO(mue) Add a retry logic later.
			return err
		}
		state.SortPorts(toClose)
		log.Printf("firewaller: closed ports %v on machine %d", toClose, machined.machine.Id())
	}
	return nil
}

// finishes cleans up when the firewaller is stopping.
func (fw *Firewaller) finish() {
	watcher.Stop(fw.environWatcher, &fw.tomb)
	watcher.Stop(fw.machinesWatcher, &fw.tomb)
	for _, unitd := range fw.unitds {
		fw.tomb.Kill(unitd.stopWatch())
	}
	for _, serviced := range fw.serviceds {
		fw.tomb.Kill(serviced.stopWatch())
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

// machineData holds machine details and watches units added or removed.
type machineData struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	machine    *state.Machine
	watcher    *state.MachineUnitsWatcher
	unitds     map[string]*unitData
	ports      []state.Port
}

// newMachineData returns a new data value for tracking details of the
// machine, and starts watching the machine for units added or removed.
func newMachineData(machine *state.Machine, fw *Firewaller) *machineData {
	md := &machineData{
		firewaller: fw,
		machine:    machine,
		watcher:    machine.WatchUnits(),
		unitds:     make(map[string]*unitData),
		ports:      make([]state.Port, 0),
	}
	go md.watchLoop()
	return md
}

// watchLoop watches the machine for units added or removed.
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

// unitData holds unit details and watches port changes.
type unitData struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	unit       *state.Unit
	watcher    *state.PortsWatcher
	serviced   *serviceData
	machined   *machineData
	ports      []state.Port
}

// newUnitData returns a new data value for tracking details of the unit,
// and starts watching the unit for port changes.
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

// watchLoop watches the unit for port changes.
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

// exposedChange contains the changed exposed flag for one specific service. 
type exposedChange struct {
	serviced *serviceData
	exposed  bool
}

// serviceData holds service details and watches exposure changes.
type serviceData struct {
	tomb       tomb.Tomb
	firewaller *Firewaller
	service    *state.Service
	watcher    *state.FlagWatcher
	exposed    bool
	unitds     map[string]*unitData
}

// newServiceData returns a new data value for tracking details of the
// service, and starts watching the service for exposure changes.
func newServiceData(service *state.Service, fw *Firewaller) *serviceData {
	sd := &serviceData{
		firewaller: fw,
		service:    service,
		watcher:    service.WatchExposed(),
		unitds:     make(map[string]*unitData),
	}
	var err error
	sd.exposed, err = service.IsExposed()
	if err != nil {
		sd.firewaller.tomb.Kill(err)
		return sd
	}
	go sd.watchLoop()
	return sd
}

// watchLoop watches the services' exposed flag for changes.
func (sd *serviceData) watchLoop() {
	defer sd.tomb.Done()
	defer sd.watcher.Stop()
	for {
		select {
		case <-sd.tomb.Dying():
			return
		case change, ok := <-sd.watcher.Changes():
			if !ok {
				sd.firewaller.tomb.Kill(watcher.MustErr(sd.watcher))
				return
			}
			select {
			case sd.firewaller.exposedChange <- &exposedChange{sd, change}:
			case <-sd.tomb.Dying():
				return
			}
		}
	}
}

// stopWatch stops the service watching.
func (sd *serviceData) stopWatch() error {
	sd.tomb.Kill(nil)
	return sd.tomb.Wait()
}

// diff returns all the ports that exist in A but not B.
func diff(A, B []state.Port) (missing []state.Port) {
next:
	for _, a := range A {
		for _, b := range B {
			if a == b {
				continue next
			}
		}
		missing = append(missing, a)
	}
	return
}
