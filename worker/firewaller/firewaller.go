package firewaller

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
)

// Firewaller watches the state for ports opened or closed
// and reflects those changes onto the backing environment.
type Firewaller struct {
	tomb            tomb.Tomb
	st              *state.State
	environ         environs.Environ
	environWatcher  *state.EnvironConfigWatcher
	machinesWatcher *state.MachinesWatcher
	machineds       map[int]*machineData
	unitsChange     chan *unitsChange
	unitds          map[string]*unitData
	portsChange     chan *portsChange
	serviceds       map[string]*serviceData
	exposedChange   chan *exposedChange
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(st *state.State) *Firewaller {
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
	go func() {
		defer fw.tomb.Done()
		fw.tomb.Kill(fw.loop())
	}()
	return fw
}

func (fw *Firewaller) loop() error {
	defer fw.stopWatchers()

	var err error
	fw.environ, err = worker.WaitForEnviron(fw.environWatcher, fw.tomb.Dying())
	if err != nil {
		return err
	}
	for {
		select {
		case <-fw.tomb.Dying():
			return tomb.ErrDying
		case change, ok := <-fw.environWatcher.Changes():
			if !ok {
				return watcher.MustErr(fw.environWatcher)
			}
			if err := fw.environ.SetConfig(change); err != nil {
				log.Printf("firewaller loaded invalid environment configuration: %v", err)
			}
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				return watcher.MustErr(fw.machinesWatcher)
			}
			for _, machine := range change.Removed {
				machined, ok := fw.machineds[machine.Id()]
				if !ok {
					panic("trying to remove machine that was not added")
				}
				delete(fw.machineds, machine.Id())
				if err := machined.Stop(); err != nil {
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
				if err := unitd.Stop(); err != nil {
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
						return err
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
				return fmt.Errorf("cannot change firewall ports: %v", err)
			}
		case change := <-fw.portsChange:
			change.unitd.ports = change.ports
			if err := fw.flushUnits([]*unitData{change.unitd}); err != nil {
				return fmt.Errorf("cannot change firewall ports: %v", err)
			}
		case change := <-fw.exposedChange:
			change.serviced.exposed = change.exposed
			unitds := []*unitData{}
			for _, unitd := range change.serviced.unitds {
				unitds = append(unitds, unitd)
			}
			if err := fw.flushUnits(unitds); err != nil {
				return fmt.Errorf("cannot change firewall ports: %v", err)
			}
		}
	}
	panic("not reached")
}

// flushUnits opens and closes ports for the passed unit data.
func (fw *Firewaller) flushUnits(unitds []*unitData) error {
	machineds := map[int]*machineData{}
	for _, unitd := range unitds {
		machineds[unitd.machined.machine.Id()] = unitd.machined
	}
	for _, machined := range machineds {
		if err := fw.flushMachine(machined); err != nil {
			return err
		}
	}
	return nil
}

// flushMachine opens and closes ports for the passed machine.
func (fw *Firewaller) flushMachine(machined *machineData) error {
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

	// If there's nothing to do, do nothing.
	// This is important because when a machine is first created,
	// it will have no instance id but also no open ports -
	// InstanceId will fail but we don't care.
	if len(toOpen) == 0 && len(toClose) == 0 {
		return nil
	}
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

// stopWatchers stops all the firewaller's watchers.
func (fw *Firewaller) stopWatchers() {
	watcher.Stop(fw.environWatcher, &fw.tomb)
	watcher.Stop(fw.machinesWatcher, &fw.tomb)
	for _, unitd := range fw.unitds {
		watcher.Stop(unitd, &fw.tomb)
	}
	for _, serviced := range fw.serviceds {
		watcher.Stop(serviced, &fw.tomb)
	}
	for _, machined := range fw.machineds {
		watcher.Stop(machined, &fw.tomb)
	}
}

func (fw *Firewaller) String() string {
	return "firewaller"
}

// Dying returns a channel that signals a Firewaller exit.
func (fw *Firewaller) Dying() <-chan struct{} {
	return fw.tomb.Dying()
}

// Err returns the reason why the firewaller has stopped or tomb.ErrStillAlive
// when it is still alive.
func (fw *Firewaller) Err() (reason error) {
	return fw.tomb.Err()
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
		watcher:    machine.WatchPrincipalUnits(),
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
func (md *machineData) Stop() error {
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
	watcher    *state.UnitWatcher
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
		watcher:    unit.Watch(),
		ports:      make([]state.Port, 0),
	}
	go ud.watchLoop()
	return ud
}

// watchLoop watches the unit for port changes.
func (ud *unitData) watchLoop() {
	defer ud.tomb.Done()
	defer ud.watcher.Stop()
	var ports []state.Port
	for {
		select {
		case <-ud.tomb.Dying():
			return
		case unit, ok := <-ud.watcher.Changes():
			if !ok {
				ud.firewaller.tomb.Kill(watcher.MustErr(ud.watcher))
				return
			}
			change := unit.OpenedPorts()
			state.SortPorts(change)
			if !portsChanged(change, ports) {
				continue
			}
			ports = append([]state.Port(nil), change...)
			select {
			case ud.firewaller.portsChange <- &portsChange{ud, change}:
			case <-ud.tomb.Dying():
				return
			}
		}
	}
}

// portsChanged returns whether old and new contain the same set ports.
// Both old and new must be sorted.
func portsChanged(old, new []state.Port) bool {
	if len(old) != len(new) {
		return true
	}
	for i, p := range old {
		if new[i] != p {
			return true
		}
	}
	return false
}

// Stop stops the unit watching.
func (ud *unitData) Stop() error {
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
	watcher    *state.ServiceWatcher
	exposed    bool
	unitds     map[string]*unitData
}

// newServiceData returns a new data value for tracking details of the
// service, and starts watching the service for exposure changes.
func newServiceData(service *state.Service, fw *Firewaller) *serviceData {
	sd := &serviceData{
		firewaller: fw,
		service:    service,
		watcher:    service.Watch(),
		unitds:     make(map[string]*unitData),
	}
	var err error
	sd.exposed, err = service.IsExposed()
	if err != nil {
		sd.firewaller.tomb.Kill(err)
		return sd
	}
	go sd.watchLoop(sd.exposed)
	return sd
}

// watchLoop watches the service's exposed flag for changes.
func (sd *serviceData) watchLoop(exposed bool) {
	defer sd.tomb.Done()
	defer sd.watcher.Stop()
	for {
		select {
		case <-sd.tomb.Dying():
			return
		case service, ok := <-sd.watcher.Changes():
			if !ok {
				sd.firewaller.tomb.Kill(watcher.MustErr(sd.watcher))
				return
			}
			change := service.IsExposed()
			if change == exposed {
				continue
			}
			exposed = change
			select {
			case sd.firewaller.exposedChange <- &exposedChange{sd, change}:
			case <-sd.tomb.Dying():
				return
			}
		}
	}
}

// Stop stops the service watching.
func (sd *serviceData) Stop() error {
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
