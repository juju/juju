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
	machinesWatcher *state.LifecycleWatcher
	machineds       map[string]*machineData
	unitsChange     chan *unitsChange
	unitds          map[string]*unitData
	portsChange     chan *portsChange
	serviceds       map[string]*serviceData
	exposedChange   chan *exposedChange
	globalMode      bool
	globalPortRef   map[state.Port]int
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(st *state.State) *Firewaller {
	fw := &Firewaller{
		st:              st,
		environWatcher:  st.WatchEnvironConfig(),
		machinesWatcher: st.WatchMachines(),
		machineds:       make(map[string]*machineData),
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
		fw.globalPortRef = make(map[state.Port]int)
	}
	if err = fw.init(); err != nil {
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
				log.Printf("worker/firewaller: loaded invalid environment configuration: %v", err)
			}
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				return watcher.MustErr(fw.machinesWatcher)
			}
			for _, id := range change {
				fw.machineLifeChanged(id)
			}
		case change := <-fw.unitsChange:
			if err := fw.unitsChanged(change); err != nil {
				return err
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

// init starts the watchers for machines, units and services based
// on the current state. In global mode it additionally checks if
// ports are opened or closed illegally.
func (fw *Firewaller) init() error {
	change, ok := <-fw.machinesWatcher.Changes()
	if !ok {
		return watcher.MustErr(fw.machinesWatcher)
	}
	for _, id := range change {
		err := fw.initMachine(id)
		if state.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
	}

	if fw.globalMode {
		// Compare needed with initial environment ports.
		initialPorts, err := fw.environ.Ports()
		if err != nil {
			return err
		}
		for _, ud := range fw.unitds {
			for _, port := range ud.ports {
				fw.globalPortRef[port]++
			}
		}
		wantedPorts := []state.Port{}
		for port := range fw.globalPortRef {
			wantedPorts = append(wantedPorts, port)
		}
		// Check which ports to open or to close.
		toOpen := diff(wantedPorts, initialPorts)
		toClose := diff(initialPorts, wantedPorts)
		if len(toOpen) > 0 {
			if err := fw.environ.OpenPorts(toOpen); err != nil {
				return err
			}
			state.SortPorts(toOpen)
			log.Printf("worker/firewaller: initially opened ports %v in environment", toOpen)
		}
		if len(toClose) > 0 {
			if err := fw.environ.ClosePorts(toClose); err != nil {
				return err
			}
			state.SortPorts(toClose)
			log.Printf("worker/firewaller: initially closed ports %v in environment", toClose)
		}
	} else {
		// Compare needed with initial instance ports.
		for _, md := range fw.machineds {
			m, err := md.machine()
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
			instances, err := fw.environ.Instances([]state.InstanceId{instanceId})
			if err != nil {
				return err
			}
			initialPorts, err := instances[0].Ports(md.id)
			if err != nil {
				return err
			}
			// Check which ports to open or to close.
			toOpen := diff(md.ports, initialPorts)
			toClose := diff(initialPorts, md.ports)
			if len(toOpen) > 0 {
				if err := instances[0].OpenPorts(md.id, toOpen); err != nil {
					// TODO(mue) Add local retry logic.
					return err
				}
				state.SortPorts(toOpen)
				log.Printf("worker/firewaller: opened ports %v on machine %s", toOpen, md.id)
			}
			if len(toClose) > 0 {
				if err := instances[0].ClosePorts(md.id, toClose); err != nil {
					// TODO(mue) Add local retry logic.
					return err
				}
				state.SortPorts(toClose)
				log.Printf("worker/firewaller: closed ports %v on machine %s", toClose, md.id)
			}
		}
	}
	return nil
}

// initMachine initializes the watching of the machine with
// the given id and its assigned units based on the initial
// watcher events.
func (fw *Firewaller) initMachine(id string) error {
	md, err := createMachineData(id, fw)
	if err != nil {
		return err
	}
	fw.machineds[id] = md
	change := <-md.unitw.Changes()
	for _, unitName := range change {
		unit, err := fw.st.Unit(unitName)
		if err != nil {
			return err
		}
		service, err := unit.Service()
		if err != nil {
			return err
		}
		serviceName := service.Name()
		if service.IsExposed() {
			if fw.serviceds[serviceName] == nil {
				fw.serviceds[serviceName] = newServiceData(service, fw)
			}
			ud := createUnitData(unit, fw)
			fw.unitds[unitName] = ud
			ud.machined = md
			ud.machined.unitds[unitName] = ud
			ud.serviced = fw.serviceds[serviceName]
			ud.serviced.unitds[unitName] = ud

			ports := unit.OpenedPorts()
			missing := diff(ports, md.ports)
			ud.ports = ports
			md.ports = append(md.ports, missing...)

			go ud.watchLoop()
			log.Debugf("worker/firewaller: started watching unit %s", unitName)
		}
	}

	go md.watchLoop()
	log.Debugf("worker/firewaller: started watching machine %s", id)
	return nil
}

// unitsChanged responds to changes to the assigned units.
func (fw *Firewaller) unitsChanged(change *unitsChange) error {
	changed := []*unitData{}
	for _, name := range change.units {
		unit, err := fw.st.Unit(name)
		if err != nil && !state.IsNotFound(err) {
			return err
		}
		var machineId string
		if unit != nil {
			machineId, err = unit.AssignedMachineId()
			if state.IsNotFound(err) {
				continue
			} else if err != nil {
				if _, ok := err.(*state.NotAssignedError); !ok {
					return err
				}
			}
		}
		if unitd, known := fw.unitds[name]; known {
			knownMachineId := fw.unitds[name].machined.id
			if unit == nil || unit.Life() == state.Dead || machineId != knownMachineId {
				fw.forgetUnit(unitd)
				changed = append(changed, unitd)
				log.Debugf("worker/firewaller: stopped watching unit %s", name)
			}
		} else if unit != nil && unit.Life() != state.Dead && fw.machineds[machineId] != nil {
			unitd := newUnitData(unit, fw)
			fw.unitds[name] = unitd
			unitd.machined = fw.machineds[machineId]
			unitd.machined.unitds[name] = unitd
			serviceName := unit.ServiceName()
			if fw.serviceds[serviceName] == nil {
				service, err := fw.st.Service(serviceName)
				if err != nil {
					return err
				}
				fw.serviceds[serviceName] = newServiceData(service, fw)
			}
			unitd.serviced = fw.serviceds[serviceName]
			unitd.serviced.unitds[name] = unitd
			changed = append(changed, unitd)
			log.Debugf("worker/firewaller: started watching unit %s", name)
		}
	}
	if err := fw.flushUnits(changed); err != nil {
		return fmt.Errorf("cannot change firewall ports: %v", err)
	}
	return nil
}

// flushUnits opens and closes ports for the passed unit data.
func (fw *Firewaller) flushUnits(unitds []*unitData) error {
	machineds := map[string]*machineData{}
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

// flushGlobalPorts opens and closes global ports in the environment.
// It keeps a reference count for ports so that only 0-to-1 and 1-to-0 events
// modify the environment.
func (fw *Firewaller) flushGlobalPorts(rawOpen, rawClose []state.Port) error {
	// Filter which ports are really to open or close.
	var toOpen, toClose []state.Port
	for _, port := range rawOpen {
		if fw.globalPortRef[port] == 0 {
			toOpen = append(toOpen, port)
		}
		fw.globalPortRef[port]++
	}
	for _, port := range rawClose {
		fw.globalPortRef[port]--
		if fw.globalPortRef[port] == 0 {
			toClose = append(toClose, port)
			delete(fw.globalPortRef, port)
		}
	}
	// Open and close the ports.
	if len(toOpen) > 0 {
		if err := fw.environ.OpenPorts(toOpen); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toOpen)
		log.Printf("worker/firewaller: opened ports %v in environment", toOpen)
	}
	if len(toClose) > 0 {
		if err := fw.environ.ClosePorts(toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toClose)
		log.Printf("worker/firewaller: closed ports %v in environment", toClose)
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
	instances, err := fw.environ.Instances([]state.InstanceId{instanceId})
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
		log.Printf("worker/firewaller: opened ports %v on machine %s", toOpen, machined.id)
	}
	if len(toClose) > 0 {
		if err := instances[0].ClosePorts(machined.id, toClose); err != nil {
			// TODO(mue) Add local retry logic.
			return err
		}
		state.SortPorts(toClose)
		log.Printf("worker/firewaller: closed ports %v on machine %s", toClose, machined.id)
	}
	return nil
}

// machineLifeChanged starts watching new machines when the firewaller
// is starting, or when new machines come to life, and stops watching
// machines that are dying.
func (fw *Firewaller) machineLifeChanged(id string) error {
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
		md, err := newMachineData(id, fw)
		if err != nil {
			return err
		}
		fw.machineds[id] = md
		log.Debugf("worker/firewaller: started watching machine %s", id)
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
	log.Debugf("worker/firewaller: stopped watching machine %s", machined.id)
	return nil
}

// forgetUnit cleans the unit data after the unit is removed.
func (fw *Firewaller) forgetUnit(unitd *unitData) {
	name := unitd.unit.Name()
	serviced := unitd.serviced
	machined := unitd.machined
	if err := unitd.Stop(); err != nil {
		log.Printf("worker/firewaller: unit watcher %q returned error when stopping: %v", name, err)
	}
	// Clean up after stopping.
	delete(fw.unitds, name)
	delete(machined.unitds, name)
	delete(serviced.unitds, name)
	if len(serviced.unitds) == 0 {
		// Stop service data after all units are removed.
		if err := serviced.Stop(); err != nil {
			log.Printf("worker/firewaller: service watcher %q returned error when stopping: %v", serviced.service, err)
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
	units    []string
}

// machineData holds machine details and watches units added or removed.
type machineData struct {
	tomb   tomb.Tomb
	fw     *Firewaller
	id     string
	unitds map[string]*unitData
	ports  []state.Port
	unitw  *state.MachineUnitsWatcher
}

// createMachineData returns an initialized data value for a machine.
func createMachineData(id string, fw *Firewaller) (*machineData, error) {
	md := &machineData{
		fw:     fw,
		id:     id,
		unitds: make(map[string]*unitData),
		ports:  make([]state.Port, 0),
	}
	m, err := md.machine()
	if err != nil {
		if !state.IsNotFound(err) {
			md.fw.tomb.Killf("worker/firewaller: cannot watch machine units: %v", err)
		}
		return nil, err
	}
	md.unitw = m.WatchUnits()
	return md, nil
}

// newMachineData returns a new data value for tracking details of the
// machine, and starts watching the machine for units added or removed.
func newMachineData(id string, fw *Firewaller) (*machineData, error) {
	md, err := createMachineData(id, fw)
	if err != nil {
		return nil, err
	}
	go md.watchLoop()
	return md, nil
}

func (md *machineData) machine() (*state.Machine, error) {
	return md.fw.st.Machine(md.id)
}

// watchLoop watches the machine for units added or removed.
func (md *machineData) watchLoop() {
	defer md.tomb.Done()
	defer md.unitw.Stop()
	for {
		select {
		case <-md.tomb.Dying():
			return
		case change, ok := <-md.unitw.Changes():
			if !ok {
				_, err := md.machine()
				if !state.IsNotFound(err) {
					md.fw.tomb.Kill(watcher.MustErr(md.unitw))
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
	serviced *serviceData
	machined *machineData
	ports    []state.Port
}

// createUnitData returns an initialized data value for a unit.
func createUnitData(unit *state.Unit, fw *Firewaller) *unitData {
	return &unitData{
		fw:    fw,
		unit:  unit,
		ports: make([]state.Port, 0),
	}
}

// newUnitData returns a new data value for tracking details of the unit,
// and starts watching the unit for port changes.
func newUnitData(unit *state.Unit, fw *Firewaller) *unitData {
	ud := createUnitData(unit, fw)
	go ud.watchLoop()
	return ud
}

// watchLoop watches the unit for port changes.
func (ud *unitData) watchLoop() {
	defer ud.tomb.Done()
	w := ud.unit.Watch()
	defer watcher.Stop(w, &ud.tomb)
	// ports for change management opposit to unit data ports 
	// managed by the firewaller.
	var ports []state.Port
	for {
		select {
		case <-ud.tomb.Dying():
			return
		case _, ok := <-w.Changes():
			if !ok {
				ud.fw.tomb.Kill(watcher.MustErr(w))
				return
			}
			if err := ud.unit.Refresh(); err != nil {
				if !state.IsNotFound(err) {
					ud.fw.tomb.Kill(err)
				}
				return
			}
			change := ud.unit.OpenedPorts()
			if samePorts(change, ports) {
				continue
			}
			ports = append(ports[:0], change...)
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
	exposed bool
	unitds  map[string]*unitData
}

// newServiceData returns a new data value for tracking details of the
// service, and starts watching the service for exposure changes.
func newServiceData(service *state.Service, fw *Firewaller) *serviceData {
	sd := &serviceData{
		fw:      fw,
		service: service,
		unitds:  make(map[string]*unitData),
	}
	sd.exposed = service.IsExposed()
	go sd.watchLoop(sd.exposed)
	return sd
}

// watchLoop watches the service's exposed flag for changes.
func (sd *serviceData) watchLoop(exposed bool) {
	defer sd.tomb.Done()
	w := sd.service.Watch()
	defer watcher.Stop(w, &sd.tomb)
	for {
		select {
		case <-sd.tomb.Dying():
			return
		case _, ok := <-w.Changes():
			if !ok {
				sd.fw.tomb.Kill(watcher.MustErr(w))
				return
			}
			if err := sd.service.Refresh(); err != nil {
				if !state.IsNotFound(err) {
					sd.fw.tomb.Kill(err)
				}
				return
			}
			change := sd.service.IsExposed()
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
