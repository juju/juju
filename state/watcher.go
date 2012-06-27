package state

import (
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// contentWatcher holds behaviour common to all ContentWatcher clients in
// the state package.
type contentWatcher struct {
	st      *State
	tomb    tomb.Tomb
	path    string
	updated bool
}

// contentHandler must be implemented by watchers that intend to make use
// of contentWatcher.
type contentHandler interface {
	update(watcher.ContentChange) error
	done()
}

// loop handles the common tasks of receiving changes from a watcher.ContentWatcher,
// and dispatching them to the contentHandler's update method.
func (w *contentWatcher) loop(handler contentHandler) {
	defer w.tomb.Done()
	defer handler.done()
	cw := watcher.NewContentWatcher(w.st.zk, w.path)
	defer watcher.Stop(cw, &w.tomb)
	for {
		select {
		case <-w.tomb.Dying():
			return
		case ch, ok := <-cw.Changes():
			if !ok {
				w.tomb.Kill(watcher.MustErr(cw))
				return
			}
			if err := handler.update(ch); err != nil {
				w.tomb.Kill(err)
				return
			}
			w.updated = true
		}
	}
}

// Stop stops the watcher and returns any errors encountered while watching.
func (w *contentWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// Err returns any error encountered while stopping the watcher, or
// tome.ErrStillAlive if the watcher is still running.
func (w *contentWatcher) Err() error {
	return w.tomb.Err()
}

// ConfigWatcher observes changes to any configuration node.
type ConfigWatcher struct {
	contentWatcher
	changeChan chan *ConfigNode
}

// newConfigWatcher creates and starts a new config watcher for
// the given path.
func newConfigWatcher(st *State, path string) *ConfigWatcher {
	w := &ConfigWatcher{
		contentWatcher: contentWatcher{
			st:   st,
			path: path,
		},
		changeChan: make(chan *ConfigNode),
	}
	go w.loop(w)
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

func (w *ConfigWatcher) update(change watcher.ContentChange) error {
	// A non-existent node is treated as an empty node.
	configNode, err := parseConfigNode(w.st.zk, w.path, change.Content)
	if err != nil {
		return err
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- configNode:
	}
	return nil
}

func (w *ConfigWatcher) done() {
	close(w.changeChan)
}

// FlagWatcher observes whether a given flag is on or off.
type FlagWatcher struct {
	contentWatcher
	changeChan chan bool
	exists     bool
}

// newFlagWatcher creates and starts a new flag watcher for
// the given path.
func newFlagWatcher(st *State, path string) *FlagWatcher {
	w := &FlagWatcher{
		contentWatcher: contentWatcher{
			st:   st,
			path: path,
		},
		changeChan: make(chan bool),
	}
	go w.loop(w)
	return w
}

// Changes returns a channel that will receive true when a
// flag is set and false if it is cleared. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial state.
func (w *FlagWatcher) Changes() <-chan bool {
	return w.changeChan
}

func (w *FlagWatcher) update(change watcher.ContentChange) error {
	if w.updated && change.Exists == w.exists {
		return nil
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- change.Exists:
		w.exists = change.Exists
	}
	return nil
}

func (w *FlagWatcher) done() {
	close(w.changeChan)
}

// NeedsUpgradeWatcher observes changes to a unit's upgrade flag.
type NeedsUpgradeWatcher struct {
	contentWatcher
	changeChan chan NeedsUpgrade
}

// newNeedsUpgradeWatcher creates and starts a new resolved flag node
// watcher for the given path.
func newNeedsUpgradeWatcher(st *State, path string) *NeedsUpgradeWatcher {
	w := &NeedsUpgradeWatcher{
		contentWatcher: contentWatcher{
			st:   st,
			path: path,
		},
		changeChan: make(chan NeedsUpgrade),
	}
	go w.loop(w)
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

func (w *NeedsUpgradeWatcher) update(change watcher.ContentChange) error {
	var needsUpgrade NeedsUpgrade
	if change.Exists {
		needsUpgrade.Upgrade = true
		var setting needsUpgradeNode
		if err := goyaml.Unmarshal([]byte(change.Content), &setting); err != nil {
			return err
		}
		needsUpgrade.Force = setting.Force
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- needsUpgrade:
	}
	return nil
}

func (w *NeedsUpgradeWatcher) done() {
	close(w.changeChan)
}

// ResolvedWatcher observes changes to a unit's resolved
// mode. See SetResolved for details.
type ResolvedWatcher struct {
	contentWatcher
	changeChan chan ResolvedMode
}

// newResolvedWatcher returns a new ResolvedWatcher watching path.
func newResolvedWatcher(st *State, path string) *ResolvedWatcher {
	w := &ResolvedWatcher{
		contentWatcher: contentWatcher{
			st:   st,
			path: path,
		},
		changeChan: make(chan ResolvedMode),
	}
	go w.loop(w)
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

func (w *ResolvedWatcher) update(change watcher.ContentChange) error {
	mode := ResolvedNone
	if change.Exists {
		var err error
		mode, err = parseResolvedMode(change.Content)
		if err != nil {
			return err
		}
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- mode:
	}
	return nil
}

func (w *ResolvedWatcher) done() {
	close(w.changeChan)
}

// PortsWatcher observes changes to a unit's open ports.
// See OpenPort for details.
type PortsWatcher struct {
	contentWatcher
	changeChan chan []Port
}

// newPortsWatcher creates and starts a new ports node
// watcher for the given path.
func newPortsWatcher(st *State, path string) *PortsWatcher {
	w := &PortsWatcher{
		contentWatcher: contentWatcher{
			st:   st,
			path: path,
		},
		changeChan: make(chan []Port),
	}
	go w.loop(w)
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

func (w *PortsWatcher) update(change watcher.ContentChange) error {
	var ports openPortsNode
	if err := goyaml.Unmarshal([]byte(change.Content), &ports); err != nil {
		return err
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- ports.Open:
	}
	return nil
}

func (w *PortsWatcher) done() {
	close(w.changeChan)
}

// MachinesWatcher notifies about machines being added or removed
// from the environment.
type MachinesWatcher struct {
	contentWatcher
	changeChan       chan *MachinesChange
	knownMachineKeys []string
}

// MachinesChange contains information about
// machines that have been added or deleted.
type MachinesChange struct {
	Added, Deleted []*Machine
}

// newMachinesWatcher creates and starts a new watcher for changes to
// the set of machines known to the topology.
func newMachinesWatcher(st *State) *MachinesWatcher {
	w := &MachinesWatcher{
		contentWatcher: contentWatcher{
			st:   st,
			path: zkTopologyPath,
		},
		changeChan: make(chan *MachinesChange),
	}
	go w.loop(w)
	return w
}

// Changes returns a channel that will receive changes when machines are
// added or deleted.  The Added field in the first event on the channel
// holds the initial state as returned by State.AllMachines.
func (w *MachinesWatcher) Changes() <-chan *MachinesChange {
	return w.changeChan
}

func (w *MachinesWatcher) update(change watcher.ContentChange) error {
	topology, err := parseTopology(change.Content)
	if err != nil {
		return err
	}
	currentMachineKeys := topology.MachineKeys()
	added := diff(currentMachineKeys, w.knownMachineKeys)
	deleted := diff(w.knownMachineKeys, currentMachineKeys)
	w.knownMachineKeys = currentMachineKeys
	if w.updated && len(added) == 0 && len(deleted) == 0 {
		return nil
	}
	mc := &MachinesChange{}
	for _, m := range added {
		mc.Added = append(mc.Added, &Machine{w.st, m})
	}
	for _, m := range deleted {
		mc.Deleted = append(mc.Deleted, &Machine{w.st, m})
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- mc:
	}
	return nil
}

func (w *MachinesWatcher) done() {
	close(w.changeChan)
}

type MachineUnitsWatcher struct {
	contentWatcher
	machine       *Machine
	changeChan    chan *MachineUnitsChange
	knownUnitKeys []string
	knownUnits    map[string]*Unit
}

type MachineUnitsChange struct {
	Added, Deleted []*Unit
}

// newMachinesWatcher creates and starts a new machine watcher.
func newMachineUnitsWatcher(m *Machine) *MachineUnitsWatcher {
	w := &MachineUnitsWatcher{
		contentWatcher: contentWatcher{
			st:   m.st,
			path: zkTopologyPath,
		},
		machine:    m,
		changeChan: make(chan *MachineUnitsChange),
		knownUnits: make(map[string]*Unit),
	}
	go w.loop(w)
	return w
}

// Changes returns a channel that will receive changes when
// units are assigned or unassigned from a machine.
// The Added field in the first event on the channel holds the initial
// state as returned by State.AllMachines.
func (w *MachineUnitsWatcher) Changes() <-chan *MachineUnitsChange {
	return w.changeChan
}

func (w *MachineUnitsWatcher) update(change watcher.ContentChange) error {
	topology, err := parseTopology(change.Content)
	if err != nil {
		return err
	}
	currentUnitKeys := topology.UnitsForMachine(w.machine.key)
	added := diff(currentUnitKeys, w.knownUnitKeys)
	deleted := diff(w.knownUnitKeys, currentUnitKeys)
	w.knownUnitKeys = currentUnitKeys
	if w.updated && len(added) == 0 && len(deleted) == 0 {
		return nil
	}
	uc := new(MachineUnitsChange)
	for _, ukey := range deleted {
		unit := w.knownUnits[ukey]
		if unit == nil {
			panic("unknown unit deleted: " + ukey)
		}
		delete(w.knownUnits, ukey)
		uc.Deleted = append(uc.Deleted, unit)
	}
	for _, ukey := range added {
		unit, err := w.st.unitFromKey(topology, ukey)
		if err != nil {
			log.Printf("inconsistent topology: %v", err)
			continue
		}
		w.knownUnits[ukey] = unit
		uc.Added = append(uc.Added, unit)
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- uc:
	}
	return nil
}

func (w *MachineUnitsWatcher) done() {
	close(w.changeChan)
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
