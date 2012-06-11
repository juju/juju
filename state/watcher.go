package state

import (
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/state/presence"
	"launchpad.net/juju-core/juju/state/watcher"
	"launchpad.net/tomb"
)

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
	if err := w.watcher.Stop(); err != nil {
		w.tomb.Wait()
		return err
	}
	return w.tomb.Wait()
}

// loop is the backend for watching the configuration node.
func (w *ConfigWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)

	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				return
			}
			// A non-existent node is treated as an empty node.
			configNode, err := parseConfigNode(w.st.zk, w.path, change.Content)
			if err != nil {
				w.tomb.Kill(err)
				return
			}
			select {
			case <-w.watcher.Dying():
				return
			case <-w.tomb.Dying():
				return
			case w.changeChan <- configNode:
			}
		}
	}
}

// NeedsUpgradeWatcher observes changes to a unit's upgrade flag.
type NeedsUpgradeWatcher struct {
	st         *State
	path       string
	tomb       tomb.Tomb
	watcher    *watcher.ContentWatcher
	changeChan chan NeedsUpgrade
}

// newNeedsUpgradeWatcher creates and starts a new resolved flag node 
// watcher for the given path.
func newNeedsUpgradeWatcher(st *State, path string) *NeedsUpgradeWatcher {
	w := &NeedsUpgradeWatcher{
		st:         st,
		path:       path,
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
	if err := w.watcher.Stop(); err != nil {
		w.tomb.Wait()
		return err
	}
	return w.tomb.Wait()
}

// loop is the backend for watching the resolved flag node.
func (w *NeedsUpgradeWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)

	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
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
			case <-w.watcher.Dying():
				return
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
	path       string
	tomb       tomb.Tomb
	watcher    *watcher.ContentWatcher
	changeChan chan ResolvedMode
}

// newResolvedWatcher returns a new ResolvedWatcher watching path.
func newResolvedWatcher(st *State, path string) *ResolvedWatcher {
	w := &ResolvedWatcher{
		st:         st,
		path:       path,
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
	if err := w.watcher.Stop(); err != nil {
		w.tomb.Wait()
		return err
	}
	return w.tomb.Wait()
}

// loop is the backend for watching the resolved flag node.
func (w *ResolvedWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)

	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
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
			case <-w.watcher.Dying():
				return
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
	path       string
	tomb       tomb.Tomb
	watcher    *watcher.ContentWatcher
	changeChan chan []Port
}

// newPortsWatcher creates and starts a new ports node 
// watcher for the given path.
func newPortsWatcher(st *State, path string) *PortsWatcher {
	w := &PortsWatcher{
		st:         st,
		path:       path,
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
	if err := w.watcher.Stop(); err != nil {
		w.tomb.Wait()
		return err
	}
	return w.tomb.Wait()
}

// loop is the backend for watching the ports node.
func (w *PortsWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)

	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
				return
			}
			var ports openPortsNode
			if err := goyaml.Unmarshal([]byte(change.Content), &ports); err != nil {
				w.tomb.Kill(err)
				return
			}
			select {
			case <-w.watcher.Dying():
				return
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
	path             string
	tomb             tomb.Tomb
	changeChan       chan *MachinesChange
	watcher          *watcher.ContentWatcher
	knownMachineKeys []string
}

// newMachinesWatcher creates and starts a new machine watcher.
func newMachinesWatcher(st *State) *MachinesWatcher {
	// start with an empty topology
	topology, _ := parseTopology("")
	w := &MachinesWatcher{
		st:               st,
		path:             zkTopologyPath,
		changeChan:       make(chan *MachinesChange),
		watcher:          watcher.NewContentWatcher(st.zk, zkTopologyPath),
		knownMachineKeys: topology.MachineKeys(),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive the actual
// watcher.ChildrenChanges. Note that multiple changes may
// be observed as a single event in the channel.
// The Added field in the first event on the channel holds the initial
// state as returned by State.AllMachines.
func (w *MachinesWatcher) Changes() <-chan *MachinesChange {
	return w.changeChan
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *MachinesWatcher) Stop() error {
	w.tomb.Kill(nil)
	if err := w.watcher.Stop(); err != nil {
		w.tomb.Wait()
		return err
	}
	return w.tomb.Wait()
}

// loop is the backend for watching the ports node.
func (w *MachinesWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)
	for {
		select {
		case <-w.tomb.Dying():
			return
		case change, ok := <-w.watcher.Changes():
			if !ok {
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
			if len(added) == 0 && len(deleted) == 0 {
				// nothing changed in zkMachinePath
				continue
			}
			// Why are we dealing with strings, not *Machines at this point ?
			// Because *Machine does not define equality, yet.
			mc := new(MachinesChange)
			for _, m := range added {
				mc.Added = append(mc.Added, &Machine{w.st, m})
			}
			for _, m := range deleted {
				mc.Deleted = append(mc.Deleted, &Machine{w.st, m})
			}
			select {
			case <-w.watcher.Dying():
				return
			case <-w.tomb.Dying():
				return
			case w.changeChan <- mc:
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

// unitRelationChange describes the state of a unit relation.
type unitRelationChange struct {
	Present  bool
	Settings string
}

// unitRelationWatcher produces notifications regarding changes to a
// particular unit relation.
type unitRelationWatcher struct {
	zk              *zookeeper.Conn
	tomb            tomb.Tomb
	presencePath    string
	settingsPath    string
	settingsWatcher *watcher.ContentWatcher
	changes         chan unitRelationChange
	started         bool
}

func newUnitRelationWatcher(zk *zookeeper.Conn, basePath, key string, role RelationRole) *unitRelationWatcher {
	w := &unitRelationWatcher{
		zk:           zk,
		presencePath: basePath + "/" + string(role) + "/" + key,
		settingsPath: basePath + "/settings/" + key,
		changes:      make(chan unitRelationChange),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive notifications
// about changes in a unit relation. Note that multiple changes
// may be observed as a single event in the channel.
// The first event on the channel holds the presence and, if
// applicable, the current version of the unit relation's settings.
func (w *unitRelationWatcher) Changes() <-chan unitRelationChange {
	return w.changes
}

// Stop stops all watches and returns any error encountered
// while watching. This method should always be called
// before discarding the watcher.
func (w *unitRelationWatcher) Stop() error {
	w.tomb.Kill(nil)
	if w.settingsWatcher != nil {
		if err := w.settingsWatcher.Stop(); err != nil {
			w.tomb.Wait()
			return err
		}
	}
	return w.tomb.Wait()
}

// loop is the backend that watches a presence node and (sometimes) a settings node.
func (w *unitRelationWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changes)
	aliveW, err := w.updatePresence(false)
	if err != nil {
		w.tomb.Kill(err)
		return
	}
	for {
		select {
		case <-w.tomb.Dying():
			return
		case alive, ok := <-aliveW:
			if !ok {
				return
			}
			aliveW, err = w.updatePresence(alive)
		case change, ok := <-w.settingsChanges():
			if !ok {
				return
			}
			err = w.updateSettings(change)
		}
		if err != nil {
			w.tomb.Kill(err)
			return
		}
	}
}

// settingsChanges returns a channel on which to receive settings node
// content changes, or nil if the settings node is not being watched.
func (w *unitRelationWatcher) settingsChanges() <-chan watcher.ContentChange {
	if w.settingsWatcher == nil {
		return nil
	}
	return w.settingsWatcher.Changes()
}

// updatePresence may send an event indicating that the unit relation has
// departed; or that it is present, and has a particular settings version.
func (w *unitRelationWatcher) updatePresence(alive bool) (aliveW <-chan bool, err error) {
	latestAlive, aliveW, err := presence.AliveW(w.zk, w.presencePath)
	if err != nil {
		return
	}
	if w.started && alive != latestAlive {
		// Presence has changed an odd number of times since the watch
		// last fired, and is therefore in the same state we last saw.
		return
	}
	if latestAlive {
		// Start settings content watcher; process initial event.
		w.settingsWatcher = watcher.NewContentWatcher(w.zk, w.settingsPath)
		select {
		case <-w.tomb.Dying():
			return nil, tomb.ErrDying
		case change, ok := <-w.settingsWatcher.Changes():
			if !ok {
				return nil, tomb.ErrDying
			}
			if err := w.updateSettings(change); err != nil {
				return nil, err
			}
		}
	} else {
		// The presence node is absent; send a notification.
		if w.started {
			sw := w.settingsWatcher
			w.settingsWatcher = nil
			if err := sw.Stop(); err != nil {
				return nil, err
			}
		}
		select {
		case <-w.tomb.Dying():
			return nil, tomb.ErrDying
		case w.changes <- unitRelationChange{}:
		}
	}
	w.started = true
	return
}

// updateSettings sends a change event indicating that the unit relation is
// present, and that its settings node exists and has the given version.
func (w *unitRelationWatcher) updateSettings(change watcher.ContentChange) error {
	if !change.Exists {
		// By activating its presence node, a unit relation is guaranteeing that
		// its settings node exists and contains valid data; if it's somehow not
		// done this then we are dealing with an unknown quantity and can't
		// sanely continue.
		return fmt.Errorf("unit relation settings not present")
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changes <- unitRelationChange{true, change.Content}:
	}
	return nil
}
