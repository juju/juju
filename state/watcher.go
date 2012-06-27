package state

import (
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// watcherStopper allows us to call Stop on a watcher without
// caring which watcher type it actually is.
type watcherStopper interface {
	Stop() error
}

// stopWatcher stops a watcher and propagates
// the error to the given tomb if nessary.
func stopWatcher(w watcherStopper, t *tomb.Tomb) {
	if err := w.Stop(); err != nil {
		t.Kill(err)
	}
}

// watcherErr allows us to check the error that killed a watcher
// without caring which watcher type it actually is.
type watcherErr interface {
	Err() error
}

// mustErr panics if the watcher does not report an error that has caused
// it to die in response. This is intended to ensure that closed subwatcher
// change channels indicate an actual error that has caused the subwatcher
// to die unexpectedly; if it hasn't died, or if it was killed with a nil
// error (which indicates a clean stop), there's a logic error somewhere.
func mustErr(w watcherErr) error {
	err := w.Err()
	if err == nil {
		panic("subwatcher was stopped cleanly")
	} else if err == tomb.ErrStillAlive {
		panic("subwatcher closed change channel")
	}
	return err
}

// ConfigWatcher observes changes to any configuration node.
type ConfigWatcher struct {
	st         *State
	path       string
	tomb       tomb.Tomb
	watcher    *watcher.ContentWatcher
	changeChan chan *ConfigNode
}

// newConfigWatcher creates and starts a new config watcher for
// the given path.
func newConfigWatcher(st *State, path string) *ConfigWatcher {
	w := &ConfigWatcher{
		st:         st,
		path:       path,
		changeChan: make(chan *ConfigNode),
		watcher:    watcher.NewContentWatcher(st.zk, path),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive the new
// *ConfigNode when a change is detected. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial state
// as returned by Service.Config.
func (w *ConfigWatcher) Changes() <-chan *ConfigNode {
	return w.changeChan
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *ConfigWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *ConfigWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)
	defer stopWatcher(w.watcher, &w.tomb)
	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				w.tomb.Kill(mustErr(w.watcher))
				return
			}
			// A non-existent node is treated as an empty node.
			configNode, err := parseConfigNode(w.st.zk, w.path, change.Content)
			if err != nil {
				w.tomb.Kill(err)
				return
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.changeChan <- configNode:
			}
		}
	}
}

// FlagWatcher observes whether a given flag is on or off.
type FlagWatcher struct {
	st         *State
	path       string
	tomb       tomb.Tomb
	watcher    *watcher.ContentWatcher
	changeChan chan bool
}

// newFlagWatcher creates and starts a new flag watcher for
// the given path.
func newFlagWatcher(st *State, path string) *FlagWatcher {
	w := &FlagWatcher{
		st:         st,
		path:       path,
		changeChan: make(chan bool),
		watcher:    watcher.NewContentWatcher(st.zk, path),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive true when a
// flag is set and false if it is cleared. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial state.
func (w *FlagWatcher) Changes() <-chan bool {
	return w.changeChan
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *FlagWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *FlagWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)
	defer stopWatcher(w.watcher, &w.tomb)

	emitted := false
	exists := false

	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				w.tomb.Kill(mustErr(w.watcher))
				return
			}
			if emitted && change.Exists == exists {
				// Nothing to do.
				continue
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.changeChan <- change.Exists:
				emitted = true
				exists = change.Exists
			}
		}
	}
}

// NeedsUpgradeWatcher observes changes to a unit's upgrade flag.
type NeedsUpgradeWatcher struct {
	st         *State
	tomb       tomb.Tomb
	watcher    *watcher.ContentWatcher
	changeChan chan NeedsUpgrade
}

// newNeedsUpgradeWatcher creates and starts a new resolved flag node
// watcher for the given path.
func newNeedsUpgradeWatcher(st *State, path string) *NeedsUpgradeWatcher {
	w := &NeedsUpgradeWatcher{
		st:         st,
		changeChan: make(chan NeedsUpgrade),
		watcher:    watcher.NewContentWatcher(st.zk, path),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive notifications
// about upgrades for the unit. Note that multiple changes
// may be observed as a single event in the channel.
// The first event on the channel holds the initial
// state as returned by Unit.NeedsUpgrade.
func (w *NeedsUpgradeWatcher) Changes() <-chan NeedsUpgrade {
	return w.changeChan
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *NeedsUpgradeWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *NeedsUpgradeWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)
	defer stopWatcher(w.watcher, &w.tomb)
	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				w.tomb.Kill(mustErr(w.watcher))
				return
			}
			var needsUpgrade NeedsUpgrade
			if change.Exists {
				needsUpgrade.Upgrade = true
				var setting needsUpgradeNode
				if err := goyaml.Unmarshal([]byte(change.Content), &setting); err != nil {
					w.tomb.Kill(err)
					return
				}
				needsUpgrade.Force = setting.Force
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.changeChan <- needsUpgrade:
			}
		}
	}
}

// ResolvedWatcher observes changes to a unit's resolved
// mode. See SetResolved for details.
type ResolvedWatcher struct {
	st         *State
	tomb       tomb.Tomb
	watcher    *watcher.ContentWatcher
	changeChan chan ResolvedMode
}

// newResolvedWatcher returns a new ResolvedWatcher watching path.
func newResolvedWatcher(st *State, path string) *ResolvedWatcher {
	w := &ResolvedWatcher{
		st:         st,
		changeChan: make(chan ResolvedMode),
		watcher:    watcher.NewContentWatcher(st.zk, path),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive the new
// resolved mode when a change is detected. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial
// state as returned by Unit.Resolved.
func (w *ResolvedWatcher) Changes() <-chan ResolvedMode {
	return w.changeChan
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *ResolvedWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *ResolvedWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)
	defer stopWatcher(w.watcher, &w.tomb)
	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				w.tomb.Kill(mustErr(w.watcher))
				return
			}
			mode := ResolvedNone
			if change.Exists {
				var err error
				mode, err = parseResolvedMode(change.Content)
				if err != nil {
					w.tomb.Kill(err)
					return
				}
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.changeChan <- mode:
			}
		}
	}
}

// PortsWatcher observes changes to a unit's open ports.
// See OpenPort for details.
type PortsWatcher struct {
	st         *State
	tomb       tomb.Tomb
	watcher    *watcher.ContentWatcher
	changeChan chan []Port
}

// newPortsWatcher creates and starts a new ports node
// watcher for the given path.
func newPortsWatcher(st *State, path string) *PortsWatcher {
	w := &PortsWatcher{
		st:         st,
		changeChan: make(chan []Port),
		watcher:    watcher.NewContentWatcher(st.zk, path),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive the actual
// open ports when a change is detected. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial
// state as returned by Unit.OpenPorts.
func (w *PortsWatcher) Changes() <-chan []Port {
	return w.changeChan
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *PortsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *PortsWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)
	defer stopWatcher(w.watcher, &w.tomb)
	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				w.tomb.Kill(mustErr(w.watcher))
				return
			}
			var ports openPortsNode
			if err := goyaml.Unmarshal([]byte(change.Content), &ports); err != nil {
				w.tomb.Kill(err)
				return
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.changeChan <- ports.Open:
			}
		}
	}
}

// MachinesWatcher notifies about machines being added or removed
// from the environment.
type MachinesWatcher struct {
	st               *State
	tomb             tomb.Tomb
	changeChan       chan *MachinesChange
	watcher          *watcher.ContentWatcher
	knownMachineKeys []string
}

// MachinesChange contains information about
// machines that have been added or removed.
type MachinesChange struct {
	Added   []*Machine
	Removed []*Machine
}

// newMachinesWatcher creates and starts a new watcher for changes to
// the set of machines known to the topology.
func newMachinesWatcher(st *State) *MachinesWatcher {
	w := &MachinesWatcher{
		st:         st,
		changeChan: make(chan *MachinesChange),
		watcher:    watcher.NewContentWatcher(st.zk, zkTopologyPath),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive changes when machines are
// added or deleted.  The Added field in the first event on the channel
// holds the initial state as returned by State.AllMachines.
func (w *MachinesWatcher) Changes() <-chan *MachinesChange {
	return w.changeChan
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *MachinesWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *MachinesWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)
	defer stopWatcher(w.watcher, &w.tomb)
	emittedValue := false
	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				w.tomb.Kill(mustErr(w.watcher))
				return
			}
			topology, err := parseTopology(change.Content)
			if err != nil {
				w.tomb.Kill(err)
				return
			}
			currentMachineKeys := topology.MachineKeys()
			added, deleted := diff(currentMachineKeys, w.knownMachineKeys), diff(w.knownMachineKeys, currentMachineKeys)
			w.knownMachineKeys = currentMachineKeys
			if emittedValue && len(added) == 0 && len(deleted) == 0 {
				// The change was not relevant to this watcher.
				continue
			}
			// Why are we dealing with strings, not *Machines at this point ?
			// Because *Machine does not define equality, yet.
			mc := new(MachinesChange)
			for _, m := range added {
				mc.Added = append(mc.Added, &Machine{w.st, m})
			}
			for _, m := range deleted {
				mc.Removed = append(mc.Removed, &Machine{w.st, m})
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.changeChan <- mc:
				emittedValue = true
			}
		}
	}
}

type MachineUnitsWatcher struct {
	st         *State
	machine    *Machine
	tomb       tomb.Tomb
	changeChan chan *MachineUnitsChange
	watcher    *watcher.ContentWatcher
}

type MachineUnitsChange struct {
	Added   []*Unit
	Removed []*Unit
}

// newMachinesWatcher creates and starts a new machine watcher.
func newMachineUnitsWatcher(m *Machine) *MachineUnitsWatcher {
	w := &MachineUnitsWatcher{
		st:         m.st,
		machine:    m,
		changeChan: make(chan *MachineUnitsChange),
		watcher:    watcher.NewContentWatcher(m.st.zk, zkTopologyPath),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive changes when
// units are assigned or unassigned from a machine.
// The Added field in the first event on the channel holds the initial
// state as returned by State.AllMachines.
func (w *MachineUnitsWatcher) Changes() <-chan *MachineUnitsChange {
	return w.changeChan
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *MachineUnitsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// loop is the backend for watching the ports node.
func (w *MachineUnitsWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)
	defer stopWatcher(w.watcher, &w.tomb)

	// knownUnits keeps track of the current units because
	// when a unit is deleted, we can't create a *Unit from
	// a key alone.
	knownUnits := make(map[string]*Unit)
	var knownUnitKeys []string
	emittedValue := false

	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				w.tomb.Kill(mustErr(w.watcher))
				return
			}
			topology, err := parseTopology(change.Content)
			if err != nil {
				w.tomb.Kill(err)
				return
			}
			currentUnitKeys := topology.UnitsForMachine(w.machine.key)
			added, deleted := diff(currentUnitKeys, knownUnitKeys), diff(knownUnitKeys, currentUnitKeys)
			knownUnitKeys = currentUnitKeys
			if emittedValue && len(added) == 0 && len(deleted) == 0 {
				// The change was not relevant to this watcher.
				continue
			}
			uc := new(MachineUnitsChange)
			for _, ukey := range deleted {
				unit := knownUnits[ukey]
				if unit == nil {
					panic("unknown unit removed: " + ukey)
				}
				delete(knownUnits, ukey)
				uc.Removed = append(uc.Removed, unit)
			}
			for _, ukey := range added {
				unit, err := w.st.unitFromKey(topology, ukey)
				if err != nil {
					log.Printf("inconsistent topology: %v", err)
					continue
				}
				knownUnits[ukey] = unit
				uc.Added = append(uc.Added, unit)
			}
			select {
			case <-w.tomb.Dying():
				return
			case w.changeChan <- uc:
				emittedValue = true
			}
		}
	}
}

// diff returns all the elements that exist in A but not B.
func diff(A, B []string) (missing []string) {
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
