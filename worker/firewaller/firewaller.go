package firewaller

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
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
	globalMode      bool
	globalPorts     map[state.Port]int
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
	if fw.environ.Config().FirewallMode() == config.FwGlobal {
		fw.globalMode = true
		fw.globalPorts = make(map[state.Port]int)
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
			for _, id := range change {
				fw.machineLifeChanged(id)
			}
		case change := <-fw.unitsChange:
			changed := []*unitData{}
			for _, unit := range change.Removed {
				unitd, ok := fw.unitds[unit.Name()]
				if !ok {
					panic("trying to remove unit that was not added")
				}
				fw.forgetUnit(unitd)
				changed = append(changed, unitd)
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
				unitd.serviced.unitds[unit.Name()] = unitd
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
		machineds[unitd.machined.id] = unitd.machined
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
	if fw.globalMode {
		return fw.flushGlobalPorts(toOpen, toClose)
	}
	return fw.flushInstancePorts(machined, toOpen, toClose)
}

// flushGlobalPorts opens and closes ports global in the environment.
func (fw *Firewaller) flushGlobalPorts(rawOpen, rawClose []state.Port) error {
	// Filter which ports are really to open or close.
	var toOpen, toClose []state.Port
	for _, port := range rawOpen {
		if fw.globalPorts[port] == 0 {
			// The port is not already open.
			toOpen = append(toOpen, port)
		}
		fw.globalPorts[port]++
	}
	for _, port := range rawClose {
		fw.globalPorts[port]--
		if fw.globalPorts[port] == 0 {
			// The last reference to the port is gone,
			// so close the port globally.
			toClose = append(toClose, port)
			delete(fw.globalPorts, port)
		}
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := fw.environ.OpenPorts(toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toOpen)
		log.Printf("firewaller: opened ports %v in environment", toOpen)
	}
	if len(toClose) > 0 {
		if err := fw.environ.ClosePorts(toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toClose)
		log.Printf("firewaller: closed ports %v in environment", toClose)
	}
	return nil
}

// flushGlobalPorts opens and closes ports global on the machine.
func (fw *Firewaller) flushInstancePorts(machined *machineData, toOpen, toClose []state.Port) error {
	// If there's nothing to do, do nothing.
	// This is important because when a machine is first created,
	// it will have no instance id but also no open ports -
	// InstanceId will fail but we don't care.
	if len(toOpen) == 0 && len(toClose) == 0 {
		return nil
	}
	m, err := machined.machine()
	if state.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	instanceId, err := m.InstanceId()
	if err != nil {
		return err
	}
	instances, err := fw.environ.Instances([]string{instanceId})
	if err != nil {
		return err
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := instances[0].OpenPorts(machined.id, toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toOpen)
		log.Printf("firewaller: opened ports %v on machine %d", toOpen, machined.id)
	}
	if len(toClose) > 0 {
		if err := instances[0].ClosePorts(machined.id, toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toClose)
		log.Printf("firewaller: closed ports %v on machine %d", toClose, machined.id)
	}
	return nil
}

// machineLifeChanged starts watching new machines when the firewaller
// is starting, or when new machines come to life, and stops watching
// machines that are dying.
func (fw *Firewaller) machineLifeChanged(id int) error {
	m, err := fw.st.Machine(id)
	found := !state.IsNotFound(err)
	if found && err != nil {
		return err
	}
	dead := !found || m.Life() == state.Dead
	machined, known := fw.machineds[id]
	if known && dead {
		return fw.forgetMachine(machined)
	}
	if !known && !dead {
		fw.machineds[id] = newMachineData(id, fw)
		log.Debugf("firewaller: started watching machine %d", id)
	}
	return nil
}

func (fw *Firewaller) forgetMachine(machined *machineData) error {
	for _, unitd := range machined.unitds {
		unitd.ports = nil
	}
	if err := fw.flushMachine(machined); err != nil {
		return err
	}
	for _, unitd := range machined.unitds {
		fw.forgetUnit(unitd)
	}
	delete(fw.machineds, machined.id)
	if err := machined.Stop(); err != nil {
		return err
	}
	log.Debugf("firewaller: stopped watching machine %d", machined.id)
	return nil
}

// forgetUnit cleans the unit data after the unit is removed.
func (fw *Firewaller) forgetUnit(unitd *unitData) {
	name := unitd.unit.Name()
	serviced := unitd.serviced
	machined := unitd.machined
	if err := unitd.Stop(); err != nil {
		log.Printf("unit watcher %q returned error when stopping: %v", name, err)
	}
	// Clean up after stopping.
	delete(fw.unitds, name)
	delete(machined.unitds, name)
	delete(serviced.unitds, name)
	if len(serviced.unitds) == 0 {
		// Stop service data after all units are removed.
		if err := serviced.Stop(); err != nil {
			log.Printf("service watcher %q returned error when stopping: %v", serviced.service, err)
		}
		delete(fw.serviceds, serviced.service.Name())
	}
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
	*state.MachinePrincipalUnitsChange
}

// machineData holds machine details and watches units added or removed.
type machineData struct {
	tomb   tomb.Tomb
	fw     *Firewaller
	id     int
	unitds map[string]*unitData
	ports  []state.Port
}

// newMachineData returns a new data value for tracking details of the
// machine, and starts watching the machine for units added or removed.
func newMachineData(id int, fw *Firewaller) *machineData {
	md := &machineData{
		fw:     fw,
		id:     id,
		unitds: make(map[string]*unitData),
		ports:  make([]state.Port, 0),
	}
	go md.watchLoop()
	return md
}

func (md *machineData) machine() (*state.Machine, error) {
	return md.fw.st.Machine(md.id)
}

// watchLoop watches the machine for units added or removed.
func (md *machineData) watchLoop() {
	defer md.tomb.Done()
	m, err := md.machine()
	if state.IsNotFound(err) {
		return
	}
	if err != nil {
		md.fw.tomb.Killf("firewaller: cannot watch machine units: %v", err)
		return
	}
	// BUG(niemeyer): The firewaller must watch all units, not just principals.
	w := m.WatchPrincipalUnits()
	defer w.Stop()
	for {
		select {
		case <-md.tomb.Dying():
			return
		case change, ok := <-w.Changes():
			if !ok {
				_, err := md.machine()
				if !state.IsNotFound(err) {
					md.fw.tomb.Kill(watcher.MustErr(w))
				}
				return
			}
			select {
			case md.fw.unitsChange <- &unitsChange{md, change}:
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
	tomb     tomb.Tomb
	fw       *Firewaller
	unit     *state.Unit
	watcher  *state.UnitWatcher
	serviced *serviceData
	machined *machineData
	ports    []state.Port
}

// newUnitData returns a new data value for tracking details of the unit,
// and starts watching the unit for port changes.
func newUnitData(unit *state.Unit, fw *Firewaller) *unitData {
	ud := &unitData{
		fw:      fw,
		unit:    unit,
		watcher: unit.Watch(),
		ports:   make([]state.Port, 0),
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
				// TODO(niemeyer): Unit watcher shouldn't return a unit.
				err := watcher.MustErr(ud.watcher)
				if !state.IsNotFound(err) {
					ud.fw.tomb.Kill(err)
				}
				return
			}
			change := unit.OpenedPorts()
			if samePorts(change, ports) {
				continue
			}
			ports = append([]state.Port(nil), change...)
			select {
			case ud.fw.portsChange <- &portsChange{ud, change}:
			case <-ud.tomb.Dying():
				return
			}
		}
	}
}

// samePorts returns whether old and new contain the same set of ports.
// Both old and new must be sorted.
func samePorts(old, new []state.Port) bool {
	if len(old) != len(new) {
		return false
	}
	for i, p := range old {
		if new[i] != p {
			return false
		}
	}
	return true
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
	tomb    tomb.Tomb
	fw      *Firewaller
	service *state.Service
	watcher *state.ServiceWatcher
	exposed bool
	unitds  map[string]*unitData
}

// newServiceData returns a new data value for tracking details of the
// service, and starts watching the service for exposure changes.
func newServiceData(service *state.Service, fw *Firewaller) *serviceData {
	sd := &serviceData{
		fw:      fw,
		service: service,
		watcher: service.Watch(),
		unitds:  make(map[string]*unitData),
	}
	sd.exposed = service.IsExposed()
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
				// TODO(niemeyer): Service watcher shouldn't return a service.
				err := watcher.MustErr(sd.watcher)
				if !state.IsNotFound(err) {
					sd.fw.tomb.Kill(err)
				}
				return
			}
			change := service.IsExposed()
			if change == exposed {
				continue
			}
			exposed = change
			select {
			case sd.fw.exposedChange <- &exposedChange{sd, change}:
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
