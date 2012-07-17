package firewaller

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// Firewaller manages the opening and closing of ports.
type Firewaller struct {
	st       *state.State
	environ  environs.Environ
	tomb     tomb.Tomb
	machines map[int]*machine
}

// NewFirewaller returns a new Firewaller.
func NewFirewaller(environ environs.Environ) (*Firewaller, error) {
	info, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	st, err := state.Open(info)
	if err != nil {
		return nil, err
	}
	fw := &Firewaller{
		st:       st,
		environ:  environ,
		machines: make(map[int]*machine),
	}
	go fw.loop()
	return fw, nil
}

func (fw *Firewaller) loop() {
	defer fw.finish()
	// Set up channels and watchers.
	machinesWatcher := fw.st.WatchMachines()
	defer watcher.Stop(machinesWatcher, &fw.tomb)
	machineUnitsChanges := make(chan *machineUnitsChange)
	defer close(machineUnitsChanges)
	for {
		select {
		case <-fw.tomb.Dying():
			return
		case change, ok := <-machinesWatcher.Changes():
			if !ok {
				err := machinesWatcher.Stop()
				if err != nil {
					fw.tomb.Kill(watcher.MustErr(machinesWatcher))
				}
				return
			}
			for _, removedMachine := range change.Removed {
				m, ok := fw.machines[removedMachine.Id()]
				if !ok {
					panic("trying to remove unmanaged machine")
				}
				if err := m.stop(); err != nil {
					panic("can't stop machine tracker")
				}
				delete(fw.machines, removedMachine.Id())
			}
			for _, addedMachine := range change.Added {
				m := newMachine(addedMachine, fw, machineUnitsChanges)
				fw.machines[addedMachine.Id()] = m
				log.Debugf("Added machine %v", m.id)
			}
		case <-machineUnitsChanges:
			// TODO(mue) fill with life.
		}
	}
}

// finishes cleans up when the firewaller is stopping.
func (fw *Firewaller) finish() {
	for _, m := range fw.machines {
		fw.tomb.Kill(m.stop())
	}
	fw.st.Close()
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
	machine *machine
	change  *state.MachineUnitsChange
}

// machine keeps track of the unit changes of a machine.
type machine struct {
	firewaller *Firewaller
	changes    chan *machineUnitsChange
	tomb       tomb.Tomb
	id         int
	watcher    *state.MachineUnitsWatcher
	ports      map[state.Port]*unit
}

// newMachine creates a new machine to be watched for units changes.
func newMachine(mst *state.Machine, fw *Firewaller, changes chan *machineUnitsChange) *machine {
	m := &machine{
		firewaller: fw,
		changes:    changes,
		id:         mst.Id(),
		watcher:    mst.WatchUnits(),
		ports:      make(map[state.Port]*unit),
	}
	go m.loop()
	return m
}

// loop is the backend watching for machine units changes.
func (m *machine) loop() {
	defer m.tomb.Done()
	defer m.watcher.Stop()
	for {
		select {
		case <-m.firewaller.tomb.Dying():
			return
		case <-m.tomb.Dying():
			return
		case change, ok := <-m.watcher.Changes():
			select {
			case m.changes <- &machineUnitsChange{m, change}:
			case <-m.firewaller.tomb.Dying():
				return
			}
			if !ok {
				m.firewaller.tomb.Kill(watcher.MustErr(m.watcher))
				return
			}
		}
	}
}

// stop lets the machine tracker stop working.
func (m *machine) stop() error {
	m.tomb.Kill(nil)
	return m.tomb.Wait()
}

type service struct {
	exposed bool
}

type unit struct {
	svc   *service
	id    string
	ports []state.Port
}
