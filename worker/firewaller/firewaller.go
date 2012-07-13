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
	machineUnitsChanges chan *machineUnitsChange
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
		machineUnitsChanges: make(chan *machineUnitsChange),
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
					// TODO(mue) Error handling in
					// case of not managed machine?
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
				fw.machines[addedMachine.Id()] = m
				log.Debugf("Added machine %v", m.id)
			}
		case <-fw.machineUnitsChanges:
			// TODO(mue) fill with life.
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

type machine struct {
	id      int
	watcher *state.MachineUnitsWatcher
	ports   map[state.Port]*unit
}

type machineUnitsChange struct {
	machine *machine
	change  *state.MachineUnitsChange
}

func (m *machine) loop(fw *Firewaller) {
	defer m.watcher.Stop()
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case change, ok := <-m.watcher.Changes():
			select {
			case fw.machineUnitsChanges <- &machineUnitsChange{m, change}:
			case <-fw.tomb.Dying():
				return
			}
			if !ok {
				return
			}
		}
	}
}

type service struct {
	exposed bool
}

type unit struct {
	svc   *service
	id    string
	ports []state.Port
}
