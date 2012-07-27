package state

import (
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
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

func newContentWatcher(st *State, path string) contentWatcher {
	return contentWatcher{st: st, path: path}
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
		contentWatcher: newContentWatcher(st, path),
		changeChan:     make(chan *ConfigNode),
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
		contentWatcher: newContentWatcher(st, path),
		changeChan:     make(chan bool),
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
		contentWatcher: newContentWatcher(st, path),
		changeChan:     make(chan NeedsUpgrade),
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
		contentWatcher: newContentWatcher(st, path),
		changeChan:     make(chan ResolvedMode),
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
		contentWatcher: newContentWatcher(st, path),
		changeChan:     make(chan []Port),
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
	watcher          *watcher.ContentWatcher
	knownMachineKeys []string
}

// MachinesChange contains information about
// machines that have been added or deleted.
type MachinesChange struct {
	Added   []*Machine
	Removed []*Machine
}

// newMachinesWatcher creates and starts a new watcher for changes to
// the set of machines known to the topology.
func newMachinesWatcher(st *State) *MachinesWatcher {
	w := &MachinesWatcher{
		contentWatcher: newContentWatcher(st, zkTopologyPath),
		changeChan:     make(chan *MachinesChange),
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
	removed := diff(w.knownMachineKeys, currentMachineKeys)
	w.knownMachineKeys = currentMachineKeys
	if w.updated && len(added) == 0 && len(removed) == 0 {
		return nil
	}
	mc := &MachinesChange{}
	for _, m := range added {
		mc.Added = append(mc.Added, newMachine(w.st, m))
	}
	for _, m := range removed {
		mc.Removed = append(mc.Removed, newMachine(w.st, m))
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
	Added   []*Unit
	Removed []*Unit
}

// newMachineUnitsWatcher creates and starts a new machine units watcher.
func newMachineUnitsWatcher(m *Machine) *MachineUnitsWatcher {
	w := &MachineUnitsWatcher{
		contentWatcher: newContentWatcher(m.st, zkTopologyPath),
		machine:        m,
		changeChan:     make(chan *MachineUnitsChange),
		knownUnits:     make(map[string]*Unit),
	}
	go w.loop(w)
	return w
}

// Changes returns a channel that will receive changes when
// units are assigned or unassigned from a machine.
// The Added field in the first event on the channel holds the initial
// state as returned by machine.Units.
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
	removed := diff(w.knownUnitKeys, currentUnitKeys)
	w.knownUnitKeys = currentUnitKeys
	if w.updated && len(added) == 0 && len(removed) == 0 {
		return nil
	}
	uc := new(MachineUnitsChange)
	for _, ukey := range removed {
		unit := w.knownUnits[ukey]
		if unit == nil {
			panic("unknown unit removed: " + ukey)
		}
		delete(w.knownUnits, ukey)
		uc.Removed = append(uc.Removed, unit)
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

// MachineInfo holds information about the settings of a machine.
type MachineInfo struct {
	// AgentVersion holds the current version of the machine agent,
	// as returned by Machine.AgentVersion. It is zero if the
	// version has not been set.
	AgentVersion version.Version

	// ProposedAgentVersion holds the proposed version of the machine agent,
	// as returned by Machine.ProposedAgentVersion. It is zero if the
	// proposed version has not been set.
	ProposedAgentVersion version.Version
}

// MachineInfoWatcher observes changes to the settings of a machine.
type MachineInfoWatcher struct {
	contentWatcher
	m          *Machine
	changeChan chan *MachineInfo
}

// newMachineInfoWatcher creates and starts a watcher to watch information
// about the machine.
func newMachineInfoWatcher(m *Machine) *MachineInfoWatcher {
	w := &MachineInfoWatcher{
		m:              m,
		contentWatcher: newContentWatcher(m.st, m.zkPath()),
		changeChan:     make(chan *MachineInfo),
	}
	go w.loop(w)
	return w
}

// Changes returns a channel that will receive the new
// *MachineInfo when a change is detected. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial state
// as returned by Machine.Info.
func (w *MachineInfoWatcher) Changes() <-chan *MachineInfo {
	return w.changeChan
}

// versionFromConfig gets a version number from the given attribute
// of the ConfigNode. It returns an error only if the attribute exists
// and is malformed.
func versionFromConfig(c *ConfigNode, attr string) (version.Version, error) {
	val, ok := c.Get(attr)
	if !ok {
		return version.Version{}, nil
	}
	s, ok := val.(string)
	if !ok {
		return version.Version{}, fmt.Errorf("invalid version type of value %#v: %T", val, val)
	}
	v, err := version.Parse(s)
	if err != nil {
		return version.Version{}, fmt.Errorf("cannot parse version %q: %v", s, err)
	}
	return v, nil
}

func (w *MachineInfoWatcher) update(change watcher.ContentChange) error {
	// A non-existent node is treated as an empty node.
	configNode, err := parseConfigNode(w.st.zk, w.path, change.Content)
	if err != nil {
		return err
	}
	var info MachineInfo
	info.AgentVersion, err = versionFromConfig(configNode, "version")
	if err != nil {
		return err
	}

	info.ProposedAgentVersion, err = versionFromConfig(configNode, "proposed-version")
	if err != nil {
		return err
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- &info:
	}
	return nil
}

func (w *MachineInfoWatcher) done() {
	close(w.changeChan)
}

// ServicesWatcher observes the addition and removal of services.
type ServicesWatcher struct {
	contentWatcher
	knownServices    map[string]*Service
	knownServiceKeys []string
	changeChan       chan *ServicesChange
}

// ServicesChange holds services that were added or removed
// from the environment.
type ServicesChange struct {
	Added   []*Service
	Removed []*Service
}

// newServicesWatcher returns a new ServicesWatcher.
func newServicesWatcher(st *State) *ServicesWatcher {
	w := &ServicesWatcher{
		contentWatcher: newContentWatcher(st, zkTopologyPath),
		knownServices:  make(map[string]*Service),
		changeChan:     make(chan *ServicesChange),
	}
	go w.loop(w)
	return w
}

// Changes returns a channel that will receive a notification when services
// are added to or removed from the state. The Added field in
// the first event on the channel holds the initial state as would be 
// returned by Service.AllServices.
func (w *ServicesWatcher) Changes() <-chan *ServicesChange {
	return w.changeChan
}

func (w *ServicesWatcher) update(change watcher.ContentChange) error {
	topology, err := parseTopology(change.Content)
	if err != nil {
		return err
	}
	currentServiceKeys := topology.ServiceKeys()
	added := diff(currentServiceKeys, w.knownServiceKeys)
	removed := diff(w.knownServiceKeys, currentServiceKeys)
	w.knownServiceKeys = currentServiceKeys
	if w.updated && len(added) == 0 && len(removed) == 0 {
		return nil
	}
	servicesChange := &ServicesChange{}
	for _, serviceKey := range removed {
		service := w.knownServices[serviceKey]
		delete(w.knownServices, serviceKey)
		servicesChange.Removed = append(servicesChange.Removed, service)
	}
	for _, serviceKey := range added {
		serviceName, err := topology.ServiceName(serviceKey)
		if err != nil {
			log.Printf("cannot read service %q: %v", serviceKey, err)
			continue
		}
		service, err := w.st.Service(serviceName)
		if err != nil {
			log.Printf("cannot read service %q: %v", serviceName, err)
			continue
		}
		w.knownServices[serviceKey] = service
		servicesChange.Added = append(servicesChange.Added, service)
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- servicesChange:
	}
	return nil
}

func (w *ServicesWatcher) done() {
	close(w.changeChan)
}

// ServiceUnitsWatcher observes the addition and removal
// of units to and from a service.
type ServiceUnitsWatcher struct {
	contentWatcher
	serviceKey    string
	knownUnits    map[string]*Unit
	knownUnitKeys []string
	changeChan    chan *ServiceUnitsChange
}

// ServiceUnitsChange contains information about
// units that have been added to or removed from
// services.
type ServiceUnitsChange struct {
	Added   []*Unit
	Removed []*Unit
}

// newServiceUnitsWatcher creates and starts a new watcher
// for service unit changes.
func newServiceUnitsWatcher(service *Service) *ServiceUnitsWatcher {
	w := &ServiceUnitsWatcher{
		contentWatcher: newContentWatcher(service.st, zkTopologyPath),
		serviceKey:     service.key,
		knownUnits:     make(map[string]*Unit),
		changeChan:     make(chan *ServiceUnitsChange),
	}
	go w.loop(w)
	return w
}

// Changes returns a channel that will receive changes when units
// are added to or removed from the service. The Added field in
// the first event on the channel holds the initial state as returned
// by Service.AllUnits.
func (w *ServiceUnitsWatcher) Changes() <-chan *ServiceUnitsChange {
	return w.changeChan
}

func (w *ServiceUnitsWatcher) update(change watcher.ContentChange) error {
	topology, err := parseTopology(change.Content)
	if err != nil {
		return err
	}
	currentUnitKeys, err := topology.UnitKeys(w.serviceKey)
	if err != nil {
		return err
	}
	added := diff(currentUnitKeys, w.knownUnitKeys)
	removed := diff(w.knownUnitKeys, currentUnitKeys)
	w.knownUnitKeys = currentUnitKeys
	if w.updated && len(added) == 0 && len(removed) == 0 {
		return nil
	}
	serviceUnitsChange := &ServiceUnitsChange{}
	for _, unitKey := range removed {
		unit := w.knownUnits[unitKey]
		delete(w.knownUnits, unitKey)
		serviceUnitsChange.Removed = append(serviceUnitsChange.Removed, unit)
	}
	for _, unitKey := range added {
		unit, err := w.st.unitFromKey(topology, unitKey)
		if err != nil {
			log.Printf("cannot read unit %q: %v", unitKey, err)
			continue
		}
		w.knownUnits[unitKey] = unit
		serviceUnitsChange.Added = append(serviceUnitsChange.Added, unit)
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- serviceUnitsChange:
	}
	return nil
}

func (w *ServiceUnitsWatcher) done() {
	close(w.changeChan)
}

// ServiceRelationsWatcher notifies of changes to a service's relations.
type ServiceRelationsWatcher struct {
	contentWatcher
	changeChan chan RelationsChange
	service    *Service
	current    map[string]*Relation
}

type RelationsChange struct {
	Added, Removed []*Relation
}

// newServiceRelationsWatcher creates and starts a new service relations watcher.
func newServiceRelationsWatcher(s *Service) *ServiceRelationsWatcher {
	w := &ServiceRelationsWatcher{
		contentWatcher: newContentWatcher(s.st, zkTopologyPath),
		changeChan:     make(chan RelationsChange),
		service:        s,
		current:        make(map[string]*Relation),
	}
	go w.loop(w)
	return w
}

// Changes returns a channel that will receive changes when
// the service enters and leaves relations.
// The Added field in the first event on the channel holds the initial
// state, corresponding to that returned by service.Relations.
func (w *ServiceRelationsWatcher) Changes() <-chan RelationsChange {
	return w.changeChan
}

func (w *ServiceRelationsWatcher) update(change watcher.ContentChange) error {
	t, err := parseTopology(change.Content)
	if err != nil {
		return err
	}
	relations, err := w.service.relationsFromTopology(t)
	if err != nil {
		return err
	}
	latest := map[string]*Relation{}
	for _, rel := range relations {
		latest[rel.key] = rel
	}
	ch := RelationsChange{}
	for key, rel := range latest {
		if w.current[key] == nil {
			ch.Added = append(ch.Added, rel)
		}
	}
	for key, rel := range w.current {
		if latest[key] == nil {
			ch.Removed = append(ch.Removed, rel)
		}
	}
	if w.updated && len(ch.Added) == 0 && len(ch.Removed) == 0 {
		return nil
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.changeChan <- ch:
		w.current = latest
	}
	return nil
}

func (w *ServiceRelationsWatcher) done() {
	close(w.changeChan)
}

// RelationUnitsWatcher watches the presence and settings of units
// playing a particular role in a particular scope of a relation,
// on behalf of another relation unit (which can potentially be in
// that scope/role, and will if so be exluded from reported events).
type RelationUnitsWatcher struct {
	st        *State
	tomb      tomb.Tomb
	role      RelationRole
	scope     unitScopePath
	ignore    string
	updates   chan unitSettingsChange
	unitTombs map[string]*tomb.Tomb
	names     map[string]string
	changes   chan RelationUnitsChange
}

// RelationUnitsChange holds settings information for newly-added and -changed
// units, and the names of those newly departed from the relation.
type RelationUnitsChange struct {
	Changed  map[string]UnitSettings
	Departed []string
}

// UnitSettings holds information about a service unit's settings within a
// relation.
type UnitSettings struct {
	Version  int
	Settings map[string]interface{}
}

// unitSettingsChange is used internally by RelationUnitsWatcher to communicate
// information about a particular unit's settings within a relation.
type unitSettingsChange struct {
	name     string
	settings UnitSettings
}

// newRelationUnitsWatcher returns a RelationUnitsWatcher which notifies of
// all presence and settings changes to units playing role within scope,
// excluding the given unit.
func newRelationUnitsWatcher(scope unitScopePath, role RelationRole, u *Unit) *RelationUnitsWatcher {
	w := &RelationUnitsWatcher{
		st:        u.st,
		role:      role,
		scope:     scope,
		ignore:    u.key,
		names:     make(map[string]string),
		unitTombs: make(map[string]*tomb.Tomb),
		updates:   make(chan unitSettingsChange),
		changes:   make(chan RelationUnitsChange),
	}
	go w.loop()
	return w
}

func (w *RelationUnitsWatcher) loop() {
	defer w.finish()
	roleWatcher := presence.NewChildrenWatcher(w.st.zk, w.scope.presencePath(w.role, ""))
	defer watcher.Stop(roleWatcher, &w.tomb)
	emittedValue := false
	for {
		var change RelationUnitsChange
		select {
		case <-w.tomb.Dying():
			return
		case ch, ok := <-roleWatcher.Changes():
			if !ok {
				w.tomb.Kill(watcher.MustErr(roleWatcher))
				return
			}
			if pchange, err := w.updateWatches(ch); err != nil {
				w.tomb.Kill(err)
				return
			} else {
				change = *pchange
			}
			if emittedValue && len(change.Changed) == 0 && len(change.Departed) == 0 {
				continue
			}
		case ch, ok := <-w.updates:
			if !ok {
				panic("updates channel closed")
			}
			change = RelationUnitsChange{
				Changed: map[string]UnitSettings{ch.name: ch.settings},
			}
		}
		select {
		case <-w.tomb.Dying():
			return
		case w.changes <- change:
			emittedValue = true
		}
	}
}

func (w *RelationUnitsWatcher) finish() {
	for _, t := range w.unitTombs {
		t.Kill(nil)
		w.tomb.Kill(t.Wait())
	}
	close(w.updates)
	close(w.changes)
	w.tomb.Done()
}

// Stop stops the watcher and returns any errors encountered while watching.
func (w *RelationUnitsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// Dying returns a channel that is closed when the
// watcher has stopped or is about to stop.
func (w *RelationUnitsWatcher) Dying() <-chan struct{} {
	return w.tomb.Dying()
}

// Err returns any error encountered while stopping the watcher, or
// tome.ErrStillAlive if the watcher is still running.
func (w *RelationUnitsWatcher) Err() error {
	return w.tomb.Err()
}

// Changes returns a channel that will receive the changes to
// the relation when detected.
// The first event on the channel holds the initial state of the
// relation in its Changed field.
func (w *RelationUnitsWatcher) Changes() <-chan RelationUnitsChange {
	return w.changes
}

// updateWatches starts or stops watches on the settings of the relation
// units declared present or absent by ch, and returns a RelationUnitsChange
// event expressing those changes.
func (w *RelationUnitsWatcher) updateWatches(ch watcher.ChildrenChange) (*RelationUnitsChange, error) {
	change := &RelationUnitsChange{}
	for _, key := range ch.Removed {
		if key == w.ignore {
			continue
		}
		// When we stop a unit settings watcher, we have to wait for its tomb,
		// lest its latest change (potentially waiting to be sent on the updates
		// channel) be received and sent on as a RelationUnitsChange event *after*
		// we notify of its departure in the event we are currently preparing.
		t := w.unitTombs[key]
		delete(w.unitTombs, key)
		t.Kill(nil)
		if err := t.Wait(); err != nil {
			return nil, err
		}
		name := w.names[key]
		delete(w.names, key)
		change.Departed = append(change.Departed, name)
	}
	var topo *topology
	for _, key := range ch.Added {
		if key == w.ignore {
			continue
		}
		if topo == nil {
			// Read topology no more than once.
			var err error
			if topo, err = readTopology(w.st.zk); err != nil {
				return nil, err
			}
		}
		name, err := topo.UnitName(key)
		if err != nil {
			return nil, err
		}
		// Start watching unit settings, and consume initial event to get
		// initial settings for the event we're preparing; subsequent
		// changes will be received on the unitLoop goroutine and sent to
		// this one via w.updates.
		w.names[key] = name
		uw := watcher.NewContentWatcher(w.st.zk, w.scope.settingsPath(key))
		select {
		case <-w.tomb.Dying():
			return nil, tomb.ErrDying
		case cch, ok := <-uw.Changes():
			if !ok {
				return nil, watcher.MustErr(uw)
			}
			us := UnitSettings{Version: cch.Version}
			if err = goyaml.Unmarshal([]byte(cch.Content), &us.Settings); err != nil {
				return nil, err
			}
			if change.Changed == nil {
				change.Changed = map[string]UnitSettings{}
			}
			change.Changed[name] = us
			t := &tomb.Tomb{}
			w.unitTombs[key] = t
			go w.unitLoop(name, uw, t)
		}
	}
	return change, nil
}

// unitLoop sends a unitSettingsChange event on w.updates for each ContentChange
// event received from uw.
func (w *RelationUnitsWatcher) unitLoop(name string, uw *watcher.ContentWatcher, t *tomb.Tomb) {
	defer t.Done()
	defer uw.Stop()
	for {
		select {
		case <-t.Dying():
			return
		case ch, ok := <-uw.Changes():
			if !ok {
				w.tomb.Kill(watcher.MustErr(uw))
				return
			}
			us := UnitSettings{Version: ch.Version}
			if err := goyaml.Unmarshal([]byte(ch.Content), &us.Settings); err != nil {
				w.tomb.Kill(err)
				return
			}
			select {
			case <-t.Dying():
				return
			case w.updates <- unitSettingsChange{name, us}:
			}
		}
	}
}
