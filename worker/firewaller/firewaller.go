package firewaller

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/tomb"
)

// Firewaller manages the opening and closing of ports.
type Firewaller struct {
	st                  *state.State
	info                *state.Info
	environ             environs.Environ
	tomb                tomb.Tomb
	environWatcher      *state.ConfigWatcher
	machinesWatcher     *state.MachinesWatcher
	machines            map[int]*machine
	units               map[string]*unit
	services            map[string]*service
	machineUnitsChanges chan *machineUnitsChange
	unitPortsChanges    chan *unitPortsChange
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(info *state.Info) (*Firewaller, error) {
	st, err := state.Open(info)
	if err != nil {
		return nil, err
	}
	fw := &Firewaller{
		st:                  st,
		info:                info,
		machinesWatcher:     st.WatchMachines(),
		machines:            make(map[int]*machine),
		units:               make(map[string]*unit),
		services:            make(map[string]*service),
		machineUnitsChanges: make(chan *machineUnitsChange),
		unitPortsChanges:    make(chan *unitPortsChange),
	}
	go fw.loop()
	return fw, nil
}

func (fw *Firewaller) loop() {
	defer fw.tomb.Done()
	defer fw.st.Close()
	// Get environment first.
	fw.environWatcher = fw.st.WatchEnvironConfig()
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case config, ok := <-fw.environWatcher.Changes():
			if !ok {
				err := fw.environWatcher.Stop()
				if err != nil {
					fw.tomb.Kill(err)
				}
				return
			}
			var err error
			fw.environ, err = environs.NewEnviron(config.Map())
			if err != nil {
				log.Printf("firewaller: loaded invalid environment configuration: %v", err)
				continue
			}
			log.Printf("firewaller: loaded new environment configuration")
		case change, ok := <-fw.machinesWatcher.Changes():
			if !ok {
				err := fw.machinesWatcher.Stop()
				if err != nil {
					fw.tomb.Kill(err)
				}
				return
			}
			for _, removedMachine := range change.Removed {
				m, ok := fw.machines[removedMachine.Id()]
				if !ok {
					// TODO(mue) Panic in case of
					// not yet managed machine?
				}
				m.watcher.Stop()
				delete(fw.machines, removedMachine.Id())
			}
			for _, addedMachine := range change.Added {
				m := &machine{
					id:      addedMachine.Id(),
					watcher: addedMachine.WatchUnits(),
					ports:   make(map[state.Port]*unit),
				}
				go m.loop(fw)
				fw.machines[addedMachine.Id()] = m
				log.Debugf("Added machine %v", m.id)
			}
		case change, ok := <-fw.machineUnitsChanges:
			if !ok {
				fw.tomb.Killf("aggregation of machine units changes failed")
				return
			}
			if change.stateChange != nil {
				for _, removedUnit := range change.stateChange.Removed {
					u, ok := fw.units[removedUnit.Name()]
					if !ok {
						// TODO(mue) Panic in case of
						// a not yet managed unit?
					}
					u.watcher.Stop()
					delete(fw.units, removedUnit.Name())
				}
				for _, addedUnit := range change.stateChange.Added {
					u := &unit{
						name:    addedUnit.Name(),
						watcher: addedUnit.WatchPorts(),
						ports:   make([]state.Port, 0),
					}
					fw.units[addedUnit.Name()] = u
					go u.loop(fw)
					if fw.services[addedUnit.ServiceName()] == nil {
						// TODO(mue) Add service watcher.
					}
				}
			}
		case <-fw.unitPortsChanges:
			// TODO(mue) Handle changes of ports.
		}
	}
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

// machine keeps track of the unit changes of a machine.
type machine struct {
	id      int
	watcher *state.MachineUnitsWatcher
	ports   map[state.Port]*unit
}

type machineUnitsChange struct {
	machine     *machine
	stateChange *state.MachineUnitsChange
}

func (m *machine) loop(fw *Firewaller) {
	defer m.watcher.Stop()
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case stateChange, ok := <-m.watcher.Changes():
			select {
			case fw.machineUnitsChanges <- &machineUnitsChange{m, stateChange}:
			case <-fw.tomb.Dying():
				return
			}
			if !ok {
				return
			}
		}
	}
}

// unit keeps track of the port changes of a unit.
type unit struct {
	svc     *service
	name    string
	watcher *state.PortsWatcher
	ports   []state.Port
}

type unitPortsChange struct {
	unit        *unit
	stateChange []state.Port
}

func (u *unit) loop(fw *Firewaller) {
	defer u.watcher.Stop()
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case stateChange, ok := <-u.watcher.Changes():
			select {
			case fw.unitPortsChanges <- &unitPortsChange{u, stateChange}:
			case <-fw.tomb.Dying():
				return
			}
			if !ok {
				return
			}
		}
	}
}

// service keeps track of the changes of a service.
type service struct {
	name    string
	exposed bool
}
