// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/tomb"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

var watchLogger = loggo.GetLogger("juju.state.watch")

// Watcher is implemented by all watchers; the actual
// changes channel is returned by a watcher-specific
// Changes method.
type Watcher interface {
	// Kill asks the watcher to stop without waiting for it do so.
	Kill()
	// Wait waits for the watcher to die and returns any
	// error encountered when it was running.
	Wait() error
	// Stop kills the watcher, then waits for it to die.
	Stop() error
	// Err returns any error encountered while the watcher
	// has been running.
	Err() error
}

// NotifyWatcher generates signals when something changes, but it does not
// return any content for those changes
type NotifyWatcher interface {
	Watcher
	Changes() <-chan struct{}
}

// StringsWatcher generates signals when something changes, returning
// the changes as a list of strings.
type StringsWatcher interface {
	Watcher
	Changes() <-chan []string
}

// RelationUnitsWatcher generates signals when units enter or leave
// the scope of a RelationUnit, and changes to the settings of those
// units known to have entered.
type RelationUnitsWatcher interface {
	Watcher
	Changes() <-chan multiwatcher.RelationUnitsChange
}

// commonWatcher is part of all client watchers.
type commonWatcher struct {
	st   *State
	tomb tomb.Tomb
}

// Stop stops the watcher, and returns any error encountered while running
// or shutting down.
func (w *commonWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher without waiting for it to shut down.
func (w *commonWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *commonWatcher) Wait() error {
	return w.tomb.Wait()
}

// Err returns any error encountered while running or shutting down, or
// tomb.ErrStillAlive if the watcher is still running.
func (w *commonWatcher) Err() error {
	return w.tomb.Err()
}

// collect combines the effects of the one change, and any further changes read
// from more in the next 10ms. The result map describes the existence, or not,
// of every id observed to have changed. If a value is read from the supplied
// stop chan, collect returns false immediately.
func collect(one watcher.Change, more <-chan watcher.Change, stop <-chan struct{}) (map[interface{}]bool, bool) {
	var count int
	result := map[interface{}]bool{}
	handle := func(ch watcher.Change) {
		count++
		result[ch.Id] = ch.Revno != -1
	}
	handle(one)
	timeout := time.After(10 * time.Millisecond)
	for done := false; !done; {
		select {
		case <-stop:
			return nil, false
		case another := <-more:
			handle(another)
		case <-timeout:
			done = true
		}
	}
	watchLogger.Tracef("read %d events for %d documents", count, len(result))
	return result, true
}

func hasString(changes []string, name string) bool {
	for _, v := range changes {
		if v == name {
			return true
		}
	}
	return false
}

var _ Watcher = (*lifecycleWatcher)(nil)

// lifecycleWatcher notifies about lifecycle changes for a set of entities of
// the same kind. The first event emitted will contain the ids of all non-Dead
// entities; subsequent events are emitted whenever one or more entities are
// added, or change their lifecycle state. After an entity is found to be
// Dead, no further event will include it.
type lifecycleWatcher struct {
	commonWatcher
	out chan []string

	// coll is a function returning the mgo.Collection holding all
	// interesting entities
	coll     func() (stateCollection, func())
	collName string

	// members is used to select the initial set of interesting entities.
	members bson.D
	// filter is used to exclude events not affecting interesting entities.
	filter func(interface{}) bool
	// life holds the most recent known life states of interesting entities.
	life map[string]Life
}

func collFactory(st *State, collName string) func() (stateCollection, func()) {
	return func() (stateCollection, func()) {
		return st.getCollection(collName)
	}
}

// WatchServices returns a StringsWatcher that notifies of changes to
// the lifecycles of the services in the environment.
func (st *State) WatchServices() StringsWatcher {
	return newLifecycleWatcher(st, servicesC, nil, nil)
}

// WatchUnits returns a StringsWatcher that notifies of changes to the
// lifecycles of units of s.
func (s *Service) WatchUnits() StringsWatcher {
	members := bson.D{{"service", s.doc.Name}}
	prefix := s.doc.Name + "/"
	filter := func(unitDocID interface{}) bool {
		unitName, err := s.st.strictLocalID(unitDocID.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(unitName, prefix)
	}
	return newLifecycleWatcher(s.st, unitsC, members, filter)
}

// WatchRelations returns a StringsWatcher that notifies of changes to the
// lifecycles of relations involving s.
func (s *Service) WatchRelations() StringsWatcher {
	prefix := s.doc.Name + ":"
	infix := " " + prefix
	filter := func(id interface{}) bool {
		k, err := s.st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		out := strings.HasPrefix(k, prefix) || strings.Contains(k, infix)
		return out
	}

	members := bson.D{{"endpoints.servicename", s.doc.Name}}
	return newLifecycleWatcher(s.st, relationsC, members, filter)
}

// WatchEnvironMachines returns a StringsWatcher that notifies of changes to
// the lifecycles of the machines (but not containers) in the environment.
func (st *State) WatchEnvironMachines() StringsWatcher {
	members := bson.D{{"$or", []bson.D{
		{{"containertype", ""}},
		{{"containertype", bson.D{{"$exists", false}}}},
	}}}
	filter := func(id interface{}) bool {
		return !strings.Contains(id.(string), "/")
	}
	return newLifecycleWatcher(st, machinesC, members, filter)
}

// WatchContainers returns a StringsWatcher that notifies of changes to the
// lifecycles of containers of the specified type on a machine.
func (m *Machine) WatchContainers(ctype instance.ContainerType) StringsWatcher {
	isChild := fmt.Sprintf("^%s/%s/%s$", m.doc.DocID, ctype, names.NumberSnippet)
	return m.containersWatcher(isChild)
}

// WatchAllContainers returns a StringsWatcher that notifies of changes to the
// lifecycles of all containers on a machine.
func (m *Machine) WatchAllContainers() StringsWatcher {
	isChild := fmt.Sprintf("^%s/%s/%s$", m.doc.DocID, names.ContainerTypeSnippet, names.NumberSnippet)
	return m.containersWatcher(isChild)
}

func (m *Machine) containersWatcher(isChildRegexp string) StringsWatcher {
	members := bson.D{{"_id", bson.D{{"$regex", isChildRegexp}}}}
	compiled := regexp.MustCompile(isChildRegexp)
	filter := func(key interface{}) bool {
		return compiled.MatchString(key.(string))
	}
	return newLifecycleWatcher(m.st, machinesC, members, filter)
}

func newLifecycleWatcher(st *State, collName string, members bson.D, filter func(key interface{}) bool) StringsWatcher {
	w := &lifecycleWatcher{
		commonWatcher: commonWatcher{st: st},
		coll:          collFactory(st, collName),
		collName:      collName,
		members:       members,
		filter:        filter,
		life:          make(map[string]Life),
		out:           make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

type lifeDoc struct {
	Id   string `bson:"_id"`
	Life Life
}

var lifeFields = bson.D{{"_id", 1}, {"life", 1}}

// Changes returns the event channel for the LifecycleWatcher.
func (w *lifecycleWatcher) Changes() <-chan []string {
	return w.out
}

func (w *lifecycleWatcher) initial() (set.Strings, error) {
	coll, closer := w.coll()
	defer closer()

	ids := make(set.Strings)
	var doc lifeDoc
	iter := coll.Find(w.members).Select(lifeFields).Iter()
	for iter.Next(&doc) {
		id := w.st.localID(doc.Id)
		ids.Add(id)
		if doc.Life != Dead {
			w.life[id] = doc.Life
		}
	}
	return ids, iter.Close()
}

func (w *lifecycleWatcher) merge(ids set.Strings, updates map[interface{}]bool) error {
	coll, closer := w.coll()
	defer closer()

	// Separate ids into those thought to exist and those known to be removed.
	var changed []string
	latest := make(map[string]Life)
	for docID, exists := range updates {
		switch docID := docID.(type) {
		case string:
			if exists {
				changed = append(changed, docID)
			} else {
				latest[w.st.localID(docID)] = Dead
			}
		default:
			return errors.Errorf("id is not of type string, got %T", docID)
		}
	}

	// Collect life states from ids thought to exist. Any that don't actually
	// exist are ignored (we'll hear about them in the next set of updates --
	// all that's actually happened in that situation is that the watcher
	// events have lagged a little behind reality).
	iter := coll.Find(bson.D{{"_id", bson.D{{"$in", changed}}}}).Select(lifeFields).Iter()
	var doc lifeDoc
	for iter.Next(&doc) {
		latest[w.st.localID(doc.Id)] = doc.Life
	}
	if err := iter.Close(); err != nil {
		return err
	}

	// Add to ids any whose life state is known to have changed.
	for id, newLife := range latest {
		gone := newLife == Dead
		oldLife, known := w.life[id]
		switch {
		case known && gone:
			delete(w.life, id)
		case !known && !gone:
			w.life[id] = newLife
		case known && newLife != oldLife:
			w.life[id] = newLife
		default:
			continue
		}
		ids.Add(id)
	}
	return nil
}

// ErrStateClosed is returned from watchers if their underlying
// state connection has been closed.
var ErrStateClosed = fmt.Errorf("state has been closed")

// stateWatcherDeadError processes the error received when the watcher
// inside a state connection dies. If the State has been closed, the
// watcher will have been stopped and error will be nil, so we ensure
// that higher level watchers return a non-nil error in that case, as
// watchers are not expected to die unexpectedly without an error.
func stateWatcherDeadError(err error) error {
	if err != nil {
		return err
	}
	return ErrStateClosed
}

func (w *lifecycleWatcher) loop() error {
	in := make(chan watcher.Change)
	w.st.watcher.WatchCollectionWithFilter(w.collName, in, w.filter)
	defer w.st.watcher.UnwatchCollection(w.collName, in)
	ids, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case ch := <-in:
			updates, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			if err := w.merge(ids, updates); err != nil {
				return err
			}
			if !ids.IsEmpty() {
				out = w.out
			}
		case out <- ids.Values():
			ids = make(set.Strings)
			out = nil
		}
	}
}

// minUnitsWatcher notifies about MinUnits changes of the services requiring
// a minimum number of units to be alive. The first event returned by the
// watcher is the set of service names requiring a minimum number of units.
// Subsequent events are generated when a service increases MinUnits, or when
// one or more units belonging to a service are destroyed.
type minUnitsWatcher struct {
	commonWatcher
	known map[string]int
	out   chan []string
}

var _ Watcher = (*minUnitsWatcher)(nil)

func newMinUnitsWatcher(st *State) StringsWatcher {
	w := &minUnitsWatcher{
		commonWatcher: commonWatcher{st: st},
		known:         make(map[string]int),
		out:           make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// WatchMinUnits returns a StringsWatcher for the minUnits collection
func (st *State) WatchMinUnits() StringsWatcher {
	return newMinUnitsWatcher(st)
}

func (w *minUnitsWatcher) initial() (set.Strings, error) {
	serviceNames := make(set.Strings)
	var doc minUnitsDoc
	newMinUnits, closer := w.st.getCollection(minUnitsC)
	defer closer()

	iter := newMinUnits.Find(nil).Iter()
	for iter.Next(&doc) {
		w.known[doc.ServiceName] = doc.Revno
		serviceNames.Add(doc.ServiceName)
	}
	return serviceNames, iter.Close()
}

func (w *minUnitsWatcher) merge(serviceNames set.Strings, change watcher.Change) error {
	serviceName := w.st.localID(change.Id.(string))
	if change.Revno == -1 {
		delete(w.known, serviceName)
		serviceNames.Remove(serviceName)
		return nil
	}
	doc := minUnitsDoc{}
	newMinUnits, closer := w.st.getCollection(minUnitsC)
	defer closer()
	if err := newMinUnits.FindId(change.Id).One(&doc); err != nil {
		return err
	}
	revno, known := w.known[serviceName]
	w.known[serviceName] = doc.Revno
	if !known || doc.Revno > revno {
		serviceNames.Add(serviceName)
	}
	return nil
}

func (w *minUnitsWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(minUnitsC, ch)
	defer w.st.watcher.UnwatchCollection(minUnitsC, ch)
	serviceNames, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case change := <-ch:
			if err = w.merge(serviceNames, change); err != nil {
				return err
			}
			if !serviceNames.IsEmpty() {
				out = w.out
			}
		case out <- serviceNames.Values():
			out = nil
			serviceNames = set.NewStrings()
		}
	}
}

func (w *minUnitsWatcher) Changes() <-chan []string {
	return w.out
}

// scopeInfo holds a RelationScopeWatcher's last-delivered state, and any
// known but undelivered changes thereto.
type scopeInfo struct {
	base map[string]bool
	diff map[string]bool
}

func (info *scopeInfo) add(name string) {
	if info.base[name] {
		delete(info.diff, name)
	} else {
		info.diff[name] = true
	}
}

func (info *scopeInfo) remove(name string) {
	if info.base[name] {
		info.diff[name] = false
	} else {
		delete(info.diff, name)
	}
}

func (info *scopeInfo) commit() {
	for name, change := range info.diff {
		if change {
			info.base[name] = true
		} else {
			delete(info.base, name)
		}
	}
	info.diff = map[string]bool{}
}

func (info *scopeInfo) hasChanges() bool {
	return len(info.diff) > 0
}

func (info *scopeInfo) changes() *RelationScopeChange {
	ch := &RelationScopeChange{}
	for name, change := range info.diff {
		if change {
			ch.Entered = append(ch.Entered, name)
		} else {
			ch.Left = append(ch.Left, name)
		}
	}
	return ch
}

var _ Watcher = (*RelationScopeWatcher)(nil)

// RelationScopeChange contains information about units that have
// entered or left a particular scope.
type RelationScopeChange struct {
	Entered []string
	Left    []string
}

// RelationScopeWatcher observes changes to the set of units
// in a particular relation scope.
type RelationScopeWatcher struct {
	commonWatcher
	prefix string
	ignore string
	out    chan *RelationScopeChange
}

func newRelationScopeWatcher(st *State, scope, ignore string) *RelationScopeWatcher {
	w := &RelationScopeWatcher{
		commonWatcher: commonWatcher{st: st},
		prefix:        scope + "#",
		ignore:        ignore,
		out:           make(chan *RelationScopeChange),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive changes when units enter and
// leave a relation scope. The Entered field in the first event on the channel
// holds the initial state.
func (w *RelationScopeWatcher) Changes() <-chan *RelationScopeChange {
	return w.out
}

// initialInfo returns an uncommitted scopeInfo with the current set of units.
func (w *RelationScopeWatcher) initialInfo() (info *scopeInfo, err error) {
	relationScopes, closer := w.st.getCollection(relationScopesC)
	defer closer()

	docs := []relationScopeDoc{}
	sel := bson.D{
		{"key", bson.D{{"$regex", "^" + w.prefix}}},
		{"departing", bson.D{{"$ne", true}}},
	}
	if err = relationScopes.Find(sel).All(&docs); err != nil {
		return nil, err
	}
	info = &scopeInfo{
		base: map[string]bool{},
		diff: map[string]bool{},
	}
	for _, doc := range docs {
		if name := doc.unitName(); name != w.ignore {
			info.add(name)
		}
	}
	return info, nil
}

// mergeChanges updates info with the contents of the changes in ids. False
// values are always treated as removed; true values cause the associated
// document to be read, and whether it's treated as added or removed depends
// on the value of the document's Departing field.
func (w *RelationScopeWatcher) mergeChanges(info *scopeInfo, ids map[interface{}]bool) error {
	relationScopes, closer := w.st.getCollection(relationScopesC)
	defer closer()

	var existIds []string
	for id, exists := range ids {
		switch id := id.(type) {
		case string:
			if exists {
				existIds = append(existIds, id)
			} else {
				key, err := w.st.strictLocalID(id)
				if err != nil {
					return errors.Trace(err)
				}
				info.remove(unitNameFromScopeKey(key))
			}
		default:
			logger.Warningf("ignoring bad relation scope id: %#v", id)
		}
	}
	var docs []relationScopeDoc
	sel := bson.D{{"_id", bson.D{{"$in", existIds}}}}
	if err := relationScopes.Find(sel).All(&docs); err != nil {
		return err
	}
	for _, doc := range docs {
		name := doc.unitName()
		if doc.Departing {
			info.remove(name)
		} else if name != w.ignore {
			info.add(name)
		}
	}
	return nil
}

func (w *RelationScopeWatcher) loop() error {
	in := make(chan watcher.Change)
	fullPrefix := w.st.docID(w.prefix)
	filter := func(id interface{}) bool {
		return strings.HasPrefix(id.(string), fullPrefix)
	}
	w.st.watcher.WatchCollectionWithFilter(relationScopesC, in, filter)
	defer w.st.watcher.UnwatchCollection(relationScopesC, in)
	info, err := w.initialInfo()
	if err != nil {
		return err
	}
	sent := false
	out := w.out
	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case ch := <-in:
			latest, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			if err := w.mergeChanges(info, latest); err != nil {
				return err
			}
			if info.hasChanges() {
				out = w.out
			} else if sent {
				out = nil
			}
		case out <- info.changes():
			info.commit()
			sent = true
			out = nil
		}
	}
}

// relationUnitsWatcher sends notifications of units entering and leaving the
// scope of a RelationUnit, and changes to the settings of those units known
// to have entered.
type relationUnitsWatcher struct {
	commonWatcher
	sw       *RelationScopeWatcher
	watching set.Strings
	updates  chan watcher.Change
	out      chan multiwatcher.RelationUnitsChange
}

// TODO(dfc) this belongs in a test
var _ Watcher = (*relationUnitsWatcher)(nil)

// Watch returns a watcher that notifies of changes to conterpart units in
// the relation.
func (ru *RelationUnit) Watch() RelationUnitsWatcher {
	return newRelationUnitsWatcher(ru)
}

func newRelationUnitsWatcher(ru *RelationUnit) RelationUnitsWatcher {
	w := &relationUnitsWatcher{
		commonWatcher: commonWatcher{st: ru.st},
		sw:            ru.WatchScope(),
		watching:      make(set.Strings),
		updates:       make(chan watcher.Change),
		out:           make(chan multiwatcher.RelationUnitsChange),
	}
	go func() {
		defer w.finish()
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive the changes to
// counterpart units in a relation. The first event on the
// channel holds the initial state of the relation in its
// Changed field.
func (w *relationUnitsWatcher) Changes() <-chan multiwatcher.RelationUnitsChange {
	return w.out
}

func emptyRelationUnitsChanges(changes *multiwatcher.RelationUnitsChange) bool {
	return len(changes.Changed)+len(changes.Departed) == 0
}

func setRelationUnitChangeVersion(changes *multiwatcher.RelationUnitsChange, key string, revno int64) {
	name := unitNameFromScopeKey(key)
	settings := multiwatcher.UnitSettings{Version: revno}
	if changes.Changed == nil {
		changes.Changed = map[string]multiwatcher.UnitSettings{}
	}
	changes.Changed[name] = settings
}

// mergeSettings reads the relation settings node for the unit with the
// supplied id, and sets a value in the Changed field keyed on the unit's
// name. It returns the mgo/txn revision number of the settings node.
func (w *relationUnitsWatcher) mergeSettings(changes *multiwatcher.RelationUnitsChange, key string) (int64, error) {
	node, err := readSettings(w.st, key)
	if err != nil {
		return -1, err
	}
	setRelationUnitChangeVersion(changes, key, node.txnRevno)
	return node.txnRevno, nil
}

// mergeScope starts and stops settings watches on the units entering and
// leaving the scope in the supplied RelationScopeChange event, and applies
// the expressed changes to the supplied RelationUnitsChange event.
func (w *relationUnitsWatcher) mergeScope(changes *multiwatcher.RelationUnitsChange, c *RelationScopeChange) error {
	for _, name := range c.Entered {
		key := w.sw.prefix + name
		docID := w.st.docID(key)
		revno, err := w.mergeSettings(changes, key)
		if err != nil {
			return err
		}
		changes.Departed = remove(changes.Departed, name)
		w.st.watcher.Watch(settingsC, docID, revno, w.updates)
		w.watching.Add(docID)
	}
	for _, name := range c.Left {
		key := w.sw.prefix + name
		docID := w.st.docID(key)
		changes.Departed = append(changes.Departed, name)
		if changes.Changed != nil {
			delete(changes.Changed, name)
		}
		w.st.watcher.Unwatch(settingsC, docID, w.updates)
		w.watching.Remove(docID)
	}
	return nil
}

// remove removes s from strs and returns the modified slice.
func remove(strs []string, s string) []string {
	for i, v := range strs {
		if s == v {
			strs[i] = strs[len(strs)-1]
			return strs[:len(strs)-1]
		}
	}
	return strs
}

func (w *relationUnitsWatcher) finish() {
	watcher.Stop(w.sw, &w.tomb)
	for _, watchedValue := range w.watching.Values() {
		w.st.watcher.Unwatch(settingsC, watchedValue, w.updates)
	}
	close(w.updates)
	close(w.out)
	w.tomb.Done()
}

func (w *relationUnitsWatcher) loop() (err error) {
	var (
		sentInitial bool
		changes     multiwatcher.RelationUnitsChange
		out         chan<- multiwatcher.RelationUnitsChange
	)
	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c, ok := <-w.sw.Changes():
			if !ok {
				return watcher.EnsureErr(w.sw)
			}
			if err = w.mergeScope(&changes, c); err != nil {
				return err
			}
			if !sentInitial || !emptyRelationUnitsChanges(&changes) {
				out = w.out
			} else {
				out = nil
			}
		case c := <-w.updates:
			id, ok := c.Id.(string)
			if !ok {
				logger.Warningf("ignoring bad relation scope id: %#v", c.Id)
			}
			setRelationUnitChangeVersion(&changes, id, c.Revno)
			out = w.out
		case out <- changes:
			sentInitial = true
			changes = multiwatcher.RelationUnitsChange{}
			out = nil
		}
	}
}

// unitsWatcher notifies of changes to a set of units. Notifications will be
// sent when units enter or leave the set, and when units in the set change
// their lifecycle status. The initial event contains all units in the set,
// regardless of lifecycle status; once a unit observed to be Dead or removed
// has been reported, it will not be reported again.
type unitsWatcher struct {
	commonWatcher
	tag      string
	getUnits func() ([]string, error)
	life     map[string]Life
	in       chan watcher.Change
	out      chan []string
}

var _ Watcher = (*unitsWatcher)(nil)

// WatchSubordinateUnits returns a StringsWatcher tracking the unit's subordinate units.
func (u *Unit) WatchSubordinateUnits() StringsWatcher {
	u = &Unit{st: u.st, doc: u.doc}
	coll := unitsC
	getUnits := func() ([]string, error) {
		if err := u.Refresh(); err != nil {
			return nil, err
		}
		return u.doc.Subordinates, nil
	}
	return newUnitsWatcher(u.st, u.Tag(), getUnits, coll, u.doc.DocID)
}

// WatchPrincipalUnits returns a StringsWatcher tracking the machine's principal
// units.
func (m *Machine) WatchPrincipalUnits() StringsWatcher {
	m = &Machine{st: m.st, doc: m.doc}
	coll := machinesC
	getUnits := func() ([]string, error) {
		if err := m.Refresh(); err != nil {
			return nil, err
		}
		return m.doc.Principals, nil
	}
	return newUnitsWatcher(m.st, m.Tag(), getUnits, coll, m.doc.DocID)
}

func newUnitsWatcher(st *State, tag names.Tag, getUnits func() ([]string, error), coll, id string) StringsWatcher {
	w := &unitsWatcher{
		commonWatcher: commonWatcher{st: st},
		tag:           tag.String(),
		getUnits:      getUnits,
		life:          map[string]Life{},
		in:            make(chan watcher.Change),
		out:           make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(coll, id))
	}()
	return w
}

// Tag returns the tag of the entity whose units are being watched.
func (w *unitsWatcher) Tag() string {
	return w.tag
}

// Changes returns the UnitsWatcher's output channel.
func (w *unitsWatcher) Changes() <-chan []string {
	return w.out
}

// lifeWatchDoc holds the fields used in starting and maintaining a watch
// on a entity's lifecycle.
type lifeWatchDoc struct {
	Id       string `bson:"_id"`
	Life     Life
	TxnRevno int64 `bson:"txn-revno"`
}

// lifeWatchFields specifies the fields of a lifeWatchDoc.
var lifeWatchFields = bson.D{{"_id", 1}, {"life", 1}, {"txn-revno", 1}}

// initial returns every member of the tracked set.
func (w *unitsWatcher) initial() ([]string, error) {
	initialNames, err := w.getUnits()
	if err != nil {
		return nil, err
	}
	newUnits, closer := w.st.getCollection(unitsC)
	defer closer()
	query := bson.D{{"name", bson.D{{"$in", initialNames}}}}
	docs := []lifeWatchDoc{}
	if err := newUnits.Find(query).Select(lifeWatchFields).All(&docs); err != nil {
		return nil, err
	}
	changes := []string{}
	for _, doc := range docs {
		unitName, err := w.st.strictLocalID(doc.Id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		changes = append(changes, unitName)
		if doc.Life != Dead {
			w.life[unitName] = doc.Life
			w.st.watcher.Watch(unitsC, doc.Id, doc.TxnRevno, w.in)
		}
	}
	return changes, nil
}

// update adds to and returns changes, such that it contains the names of any
// non-Dead units to have entered or left the tracked set.
func (w *unitsWatcher) update(changes []string) ([]string, error) {
	latest, err := w.getUnits()
	if err != nil {
		return nil, err
	}
	for _, name := range latest {
		if _, known := w.life[name]; !known {
			changes, err = w.merge(changes, name)
			if err != nil {
				return nil, err
			}
		}
	}
	for name := range w.life {
		if hasString(latest, name) {
			continue
		}
		if !hasString(changes, name) {
			changes = append(changes, name)
		}
		delete(w.life, name)
		w.st.watcher.Unwatch(unitsC, w.st.docID(name), w.in)
	}
	return changes, nil
}

// merge adds to and returns changes, such that it contains the supplied unit
// name if that unit is unknown and non-Dead, or has changed lifecycle status.
func (w *unitsWatcher) merge(changes []string, name string) ([]string, error) {
	units, closer := w.st.getCollection(unitsC)
	defer closer()

	unitDocID := w.st.docID(name)
	doc := lifeWatchDoc{}
	err := units.FindId(unitDocID).Select(lifeWatchFields).One(&doc)
	gone := false
	if err == mgo.ErrNotFound {
		gone = true
	} else if err != nil {
		return nil, err
	} else if doc.Life == Dead {
		gone = true
	}
	life, known := w.life[name]
	switch {
	case known && gone:
		delete(w.life, name)
		w.st.watcher.Unwatch(unitsC, unitDocID, w.in)
	case !known && !gone:
		w.st.watcher.Watch(unitsC, unitDocID, doc.TxnRevno, w.in)
		w.life[name] = doc.Life
	case known && life != doc.Life:
		w.life[name] = doc.Life
	default:
		return changes, nil
	}
	if !hasString(changes, name) {
		changes = append(changes, name)
	}
	return changes, nil
}

func (w *unitsWatcher) loop(coll, id string) error {
	collection, closer := w.st.getCollection(coll)
	revno, err := getTxnRevno(collection, id)
	closer()
	if err != nil {
		return err
	}

	w.st.watcher.Watch(coll, id, revno, w.in)
	defer func() {
		w.st.watcher.Unwatch(coll, id, w.in)
		for name := range w.life {
			w.st.watcher.Unwatch(unitsC, w.st.docID(name), w.in)
		}
	}()
	changes, err := w.initial()
	if err != nil {
		return err
	}
	rootLocalID := w.st.localID(id)
	out := w.out
	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c := <-w.in:
			localID := w.st.localID(c.Id.(string))
			if localID == rootLocalID {
				changes, err = w.update(changes)
			} else {
				changes, err = w.merge(changes, localID)
			}
			if err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// EnvironConfigWatcher observes changes to the
// environment configuration.
type EnvironConfigWatcher struct {
	commonWatcher
	out chan *config.Config
}

var _ Watcher = (*EnvironConfigWatcher)(nil)

// WatchEnvironConfig returns a watcher for observing changes
// to the environment configuration.
func (st *State) WatchEnvironConfig() *EnvironConfigWatcher {
	return newEnvironConfigWatcher(st)
}

func newEnvironConfigWatcher(s *State) *EnvironConfigWatcher {
	w := &EnvironConfigWatcher{
		commonWatcher: commonWatcher{st: s},
		out:           make(chan *config.Config),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns a channel that will receive the new environment
// configuration when a change is detected. Note that multiple changes may
// be observed as a single event in the channel.
func (w *EnvironConfigWatcher) Changes() <-chan *config.Config {
	return w.out
}

func (w *EnvironConfigWatcher) loop() (err error) {
	sw := w.st.watchSettings(environGlobalKey)
	defer sw.Stop()
	out := w.out
	out = nil
	cfg := &config.Config{}
	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case settings, ok := <-sw.Changes():
			if !ok {
				return watcher.EnsureErr(sw)
			}
			cfg, err = config.New(config.NoDefaults, settings.Map())
			if err == nil {
				out = w.out
			} else {
				out = nil
			}
		case out <- cfg:
			out = nil
		}
	}
}

type settingsWatcher struct {
	commonWatcher
	out chan *Settings
}

var _ Watcher = (*settingsWatcher)(nil)

// watchSettings creates a watcher for observing changes to settings.
func (st *State) watchSettings(key string) *settingsWatcher {
	return newSettingsWatcher(st, key)
}

func newSettingsWatcher(s *State, key string) *settingsWatcher {
	w := &settingsWatcher{
		commonWatcher: commonWatcher{st: s},
		out:           make(chan *Settings),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(key))
	}()
	return w
}

// Changes returns a channel that will receive the new settings.
// Multiple changes may be observed as a single event in the channel.
func (w *settingsWatcher) Changes() <-chan *Settings {
	return w.out
}

func (w *settingsWatcher) loop(key string) (err error) {
	ch := make(chan watcher.Change)
	revno := int64(-1)
	settings, err := readSettings(w.st, key)
	if err == nil {
		revno = settings.txnRevno
	} else if !errors.IsNotFound(err) {
		return err
	}
	w.st.watcher.Watch(settingsC, w.st.docID(key), revno, ch)
	defer w.st.watcher.Unwatch(settingsC, w.st.docID(key), ch)
	out := w.out
	if revno == -1 {
		out = nil
	}
	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-ch:
			settings, err = readSettings(w.st, key)
			if err != nil {
				return err
			}
			out = w.out
		case out <- settings:
			out = nil
		}
	}
}

// entityWatcher generates an event when a document in the db changes
type entityWatcher struct {
	commonWatcher
	out chan struct{}
}

var _ Watcher = (*entityWatcher)(nil)

// WatchHardwareCharacteristics returns a watcher for observing changes to a machine's hardware characteristics.
func (m *Machine) WatchHardwareCharacteristics() NotifyWatcher {
	return newEntityWatcher(m.st, instanceDataC, m.doc.DocID)
}

// WatchStateServerInfo returns a NotifyWatcher for the stateServers collection
func (st *State) WatchStateServerInfo() NotifyWatcher {
	return newEntityWatcher(st, stateServersC, environGlobalKey)
}

// Watch returns a watcher for observing changes to a machine.
func (m *Machine) Watch() NotifyWatcher {
	return newEntityWatcher(m.st, machinesC, m.doc.DocID)
}

// Watch returns a watcher for observing changes to a service.
func (s *Service) Watch() NotifyWatcher {
	return newEntityWatcher(s.st, servicesC, s.doc.DocID)
}

// Watch returns a watcher for observing changes to a unit.
func (u *Unit) Watch() NotifyWatcher {
	return newEntityWatcher(u.st, unitsC, u.doc.DocID)
}

// Watch returns a watcher for observing changes to an environment.
func (e *Environment) Watch() NotifyWatcher {
	return newEntityWatcher(e.st, environmentsC, e.doc.UUID)
}

// WatchUpgradeInfo returns a watcher for observing changes to upgrade
// synchronisation state.
func (st *State) WatchUpgradeInfo() NotifyWatcher {
	return newEntityWatcher(st, upgradeInfoC, currentUpgradeId)
}

// WatchRestoreInfoChanges returns a NotifyWatcher that will inform
// when the restore status changes.
func (st *State) WatchRestoreInfoChanges() NotifyWatcher {
	return newEntityWatcher(st, restoreInfoC, currentRestoreId)
}

// WatchForEnvironConfigChanges returns a NotifyWatcher waiting for the Environ
// Config to change. This differs from WatchEnvironConfig in that the watcher
// is a NotifyWatcher that does not give content during Changes()
func (st *State) WatchForEnvironConfigChanges() NotifyWatcher {
	return newEntityWatcher(st, settingsC, st.docID(environGlobalKey))
}

// WatchAPIHostPorts returns a NotifyWatcher that notifies
// when the set of API addresses changes.
func (st *State) WatchAPIHostPorts() NotifyWatcher {
	return newEntityWatcher(st, stateServersC, apiHostPortsKey)
}

// WatchConfigSettings returns a watcher for observing changes to the
// unit's service configuration settings. The unit must have a charm URL
// set before this method is called, and the returned watcher will be
// valid only while the unit's charm URL is not changed.
// TODO(fwereade): this could be much smarter; if it were, uniter.Filter
// could be somewhat simpler.
func (u *Unit) WatchConfigSettings() (NotifyWatcher, error) {
	if u.doc.CharmURL == nil {
		return nil, fmt.Errorf("unit charm not set")
	}
	settingsKey := serviceSettingsKey(u.doc.Service, u.doc.CharmURL)
	return newEntityWatcher(u.st, settingsC, u.st.docID(settingsKey)), nil
}

// WatchMeterStatus returns a watcher observing the changes to the unit's
// meter status.
func (u *Unit) WatchMeterStatus() NotifyWatcher {
	return newEntityWatcher(u.st, meterStatusC, u.st.docID(u.globalKey()))
}

func newEntityWatcher(st *State, collName string, key interface{}) NotifyWatcher {
	w := &entityWatcher{
		commonWatcher: commonWatcher{st: st},
		out:           make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(collName, key))
	}()
	return w
}

// Changes returns the event channel for the entityWatcher.
func (w *entityWatcher) Changes() <-chan struct{} {
	return w.out
}

// getTxnRevno returns the transaction revision number of the
// given key in the given collection. It is useful to enable
// a watcher.Watcher to be primed with the correct revision
// id.
func getTxnRevno(coll stateCollection, key interface{}) (int64, error) {
	doc := struct {
		TxnRevno int64 `bson:"txn-revno"`
	}{}
	fields := bson.D{{"txn-revno", 1}}
	if err := coll.FindId(key).Select(fields).One(&doc); err == mgo.ErrNotFound {
		return -1, nil
	} else if err != nil {
		return 0, err
	}
	return doc.TxnRevno, nil
}

func (w *entityWatcher) loop(collName string, key interface{}) error {
	coll, closer := w.st.getCollection(collName)
	txnRevno, err := getTxnRevno(coll, key)
	closer()
	if err != nil {
		return err
	}
	in := make(chan watcher.Change)
	w.st.watcher.Watch(coll.Name(), key, txnRevno, in)
	defer w.st.watcher.Unwatch(coll.Name(), key, in)
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case ch := <-in:
			if _, ok := collect(ch, in, w.tomb.Dying()); !ok {
				return tomb.ErrDying
			}
			out = w.out
		case out <- struct{}{}:
			out = nil
		}
	}
}

// machineUnitsWatcher notifies about assignments and lifecycle changes
// for all units of a machine.
//
// The first event emitted contains the unit names of all units currently
// assigned to the machine, irrespective of their life state. From then on,
// a new event is emitted whenever a unit is assigned to or unassigned from
// the machine, or the lifecycle of a unit that is currently assigned to
// the machine changes.
//
// After a unit is found to be Dead, no further event will include it.
type machineUnitsWatcher struct {
	commonWatcher
	machine *Machine
	out     chan []string
	in      chan watcher.Change
	known   map[string]Life
}

var _ Watcher = (*machineUnitsWatcher)(nil)

// WatchUnits returns a new StringsWatcher watching m's units.
func (m *Machine) WatchUnits() StringsWatcher {
	return newMachineUnitsWatcher(m)
}

func newMachineUnitsWatcher(m *Machine) StringsWatcher {
	w := &machineUnitsWatcher{
		commonWatcher: commonWatcher{st: m.st},
		out:           make(chan []string),
		in:            make(chan watcher.Change),
		known:         make(map[string]Life),
		machine:       &Machine{st: m.st, doc: m.doc}, // Copy so it may be freely refreshed
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for w.
func (w *machineUnitsWatcher) Changes() <-chan []string {
	return w.out
}

func (w *machineUnitsWatcher) updateMachine(pending []string) (new []string, err error) {
	err = w.machine.Refresh()
	if err != nil {
		return nil, err
	}
	for _, unitName := range w.machine.doc.Principals {
		if _, ok := w.known[unitName]; !ok {
			pending, err = w.merge(pending, unitName)
			if err != nil {
				return nil, err
			}
		}
	}
	return pending, nil
}

func (w *machineUnitsWatcher) merge(pending []string, unitName string) (new []string, err error) {
	doc := unitDoc{}
	newUnits, closer := w.st.getCollection(unitsC)
	defer closer()
	err = newUnits.FindId(unitName).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, err
	}
	life, known := w.known[unitName]
	if err == mgo.ErrNotFound || doc.Principal == "" && (doc.MachineId == "" || doc.MachineId != w.machine.doc.Id) {
		// Unit was removed or unassigned from w.machine.
		if known {
			delete(w.known, unitName)
			w.st.watcher.Unwatch(unitsC, w.st.docID(unitName), w.in)
			if life != Dead && !hasString(pending, unitName) {
				pending = append(pending, unitName)
			}
			for _, subunitName := range doc.Subordinates {
				if sublife, subknown := w.known[subunitName]; subknown {
					delete(w.known, subunitName)
					w.st.watcher.Unwatch(unitsC, w.st.docID(subunitName), w.in)
					if sublife != Dead && !hasString(pending, subunitName) {
						pending = append(pending, subunitName)
					}
				}
			}
		}
		return pending, nil
	}
	if !known {
		w.st.watcher.Watch(unitsC, doc.DocID, doc.TxnRevno, w.in)
		pending = append(pending, unitName)
	} else if life != doc.Life && !hasString(pending, unitName) {
		pending = append(pending, unitName)
	}
	w.known[unitName] = doc.Life
	for _, subunitName := range doc.Subordinates {
		if _, ok := w.known[subunitName]; !ok {
			pending, err = w.merge(pending, subunitName)
			if err != nil {
				return nil, err
			}
		}
	}
	return pending, nil
}

func (w *machineUnitsWatcher) loop() error {
	defer func() {
		for unit := range w.known {
			w.st.watcher.Unwatch(unitsC, w.st.docID(unit), w.in)
		}
	}()

	machines, closer := w.st.getCollection(machinesC)
	revno, err := getTxnRevno(machines, w.machine.doc.DocID)
	closer()
	if err != nil {
		return err
	}
	machineCh := make(chan watcher.Change)
	w.st.watcher.Watch(machinesC, w.machine.doc.DocID, revno, machineCh)
	defer w.st.watcher.Unwatch(machinesC, w.machine.doc.DocID, machineCh)
	changes, err := w.updateMachine([]string(nil))
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-machineCh:
			changes, err = w.updateMachine(changes)
			if err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.out
			}
		case c := <-w.in:
			changes, err = w.merge(changes, w.st.localID(c.Id.(string)))
			if err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// machineAddressesWatcher notifies about changes to a machine's addresses.
//
// The first event emitted contains the addresses currently assigned to the
// machine. From then on, a new event is emitted whenever the machine's
// addresses change.
type machineAddressesWatcher struct {
	commonWatcher
	machine *Machine
	out     chan struct{}
}

var _ Watcher = (*machineAddressesWatcher)(nil)

// WatchAddresses returns a new NotifyWatcher watching m's addresses.
func (m *Machine) WatchAddresses() NotifyWatcher {
	return newMachineAddressesWatcher(m)
}

func newMachineAddressesWatcher(m *Machine) NotifyWatcher {
	w := &machineAddressesWatcher{
		commonWatcher: commonWatcher{st: m.st},
		out:           make(chan struct{}),
		machine:       &Machine{st: m.st, doc: m.doc}, // Copy so it may be freely refreshed
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for w.
func (w *machineAddressesWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *machineAddressesWatcher) loop() error {
	machines, closer := w.st.getCollection(machinesC)
	revno, err := getTxnRevno(machines, w.machine.doc.DocID)
	closer()
	if err != nil {
		return err
	}
	machineCh := make(chan watcher.Change)
	w.st.watcher.Watch(machinesC, w.machine.doc.DocID, revno, machineCh)
	defer w.st.watcher.Unwatch(machinesC, w.machine.doc.DocID, machineCh)
	addresses := w.machine.Addresses()
	out := w.out
	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-machineCh:
			if err := w.machine.Refresh(); err != nil {
				return err
			}
			newAddresses := w.machine.Addresses()
			if !addressesEqual(newAddresses, addresses) {
				addresses = newAddresses
				out = w.out
			}
		case out <- struct{}{}:
			out = nil
		}
	}
}

// cleanupWatcher notifies of changes in the cleanups collection.
type cleanupWatcher struct {
	commonWatcher
	out chan struct{}
}

var _ Watcher = (*cleanupWatcher)(nil)

// WatchCleanups starts and returns a CleanupWatcher.
func (st *State) WatchCleanups() NotifyWatcher {
	return newCleanupWatcher(st)
}

func newCleanupWatcher(st *State) NotifyWatcher {
	w := &cleanupWatcher{
		commonWatcher: commonWatcher{st: st},
		out:           make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for w.
func (w *cleanupWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *cleanupWatcher) loop() (err error) {
	in := make(chan watcher.Change)

	w.st.watcher.WatchCollection(cleanupsC, in)
	defer w.st.watcher.UnwatchCollection(cleanupsC, in)

	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case ch := <-in:
			if _, ok := collect(ch, in, w.tomb.Dying()); !ok {
				return tomb.ErrDying
			}
			out = w.out
		case out <- struct{}{}:
			out = nil
		}
	}
}

// actionStatusWatcher is a StringsWatcher that filters notifications
// to Action Id's that match the ActionReceiver and ActionStatus set
// provided.
type actionStatusWatcher struct {
	commonWatcher
	source         chan watcher.Change
	sink           chan []string
	receiverFilter bson.D
	statusFilter   bson.D
}

var _ StringsWatcher = (*actionStatusWatcher)(nil)

// newActionStatusWatcher returns the StringsWatcher that will notify
// on changes to Actions with the given ActionReceiver and ActionStatus
// filters.
func newActionStatusWatcher(st *State, receivers []ActionReceiver, statusSet ...ActionStatus) StringsWatcher {
	watchLogger.Debugf("newActionStatusWatcher receivers:'%+v', statuses'%+v'", receivers, statusSet)
	w := &actionStatusWatcher{
		commonWatcher:  commonWatcher{st: st},
		source:         make(chan watcher.Change),
		sink:           make(chan []string),
		receiverFilter: actionReceiverInCollectionOp(receivers...),
		statusFilter:   statusInCollectionOp(statusSet...),
	}

	go func() {
		defer w.tomb.Done()
		defer close(w.sink)
		w.tomb.Kill(w.loop())
	}()

	return w
}

// Changes returns the channel that sends the ids of any
// Actions that change in the actionsC collection, if they
// match the ActionReceiver and ActionStatus filters on the
// watcher.
func (w *actionStatusWatcher) Changes() <-chan []string {
	watchLogger.Tracef("actionStatusWatcher Changes()")
	return w.sink
}

// loop performs the main event loop cycle, polling for changes and
// responding to Changes requests
func (w *actionStatusWatcher) loop() error {
	watchLogger.Tracef("actionStatusWatcher loop()")
	var (
		changes []string
		in      <-chan watcher.Change = w.source
		out     chan<- []string       = w.sink
	)

	w.st.watcher.WatchCollection(actionsC, w.source)
	defer w.st.watcher.UnwatchCollection(actionsC, w.source)

	changes, err := w.initial()
	if err != nil {
		return err
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case ch := <-in:
			updates, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			if err := w.filterAndMergeIds(w.st, &changes, updates); err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.sink
			}
		case out <- changes:
			changes = nil
			out = nil
		}
	}
}

// initial pre-loads the id's that have already been added to the
// collection that would otherwise not normally trigger the watcher
func (w *actionStatusWatcher) initial() ([]string, error) {
	watchLogger.Tracef("actionStatusWatcher initial()")
	return w.matchingIds()
}

// matchingIds is a helper function that filters the actionsC collection
// on the ActionReceivers and ActionStatus set defined on the watcher.
// If ids are passed in the collection is further filtered to only
// Actions that also have one of the supplied _id's.
func (w *actionStatusWatcher) matchingIds(ids ...string) ([]string, error) {
	watchLogger.Tracef("actionStatusWatcher matchingIds() ids:'%+v'", ids)

	coll, closer := w.st.getCollection(actionsC)
	defer closer()

	idFilter := localIdInCollectionOp(w.st, ids...)
	query := bson.D{{"$and", []bson.D{idFilter, w.receiverFilter, w.statusFilter}}}
	iter := coll.Find(query).Iter()
	var found []string
	var doc actionDoc
	for iter.Next(&doc) {
		found = append(found, w.st.localID(doc.DocId))
	}
	watchLogger.Debugf("actionStatusWatcher matchingIds() ids:'%+v', found:'%+v'", ids, found)
	return found, iter.Close()
}

// filterAndMergeIds combines existing pending changes along with
// updates from the upstream watcher, and updates the changes set.
// If the upstream changes do not match the ActionReceivers and
// ActionStatus set filters defined on the watcher, they are silently
// dropped.
func (w *actionStatusWatcher) filterAndMergeIds(st *State, changes *[]string, updates map[interface{}]bool) error {
	watchLogger.Tracef("actionStatusWatcher filterAndMergeIds(changes:'%+v', updates:'%+v')", changes, updates)
	var adds []string
	for id, exists := range updates {
		switch id := id.(type) {
		case string:
			localId := st.localID(id)
			chIx, idAlreadyInChangeset := indexOf(localId, *changes)
			if exists {
				if !idAlreadyInChangeset {
					adds = append(adds, localId)
				}
			} else {
				if idAlreadyInChangeset {
					// remove id from changes
					*changes = append([]string(*changes)[:chIx], []string(*changes)[chIx+1:]...)
				}
			}
		default:
			return errors.Errorf("id is not of type string, got %T", id)
		}
	}
	if len(adds) > 0 {
		ids, err := w.matchingIds(adds...)
		if err != nil {
			return errors.Trace(err)
		}
		*changes = append(*changes, ids...)
	}
	return nil
}

// inCollectionOp takes a key name and a list of potential values and
// returns a bson.D Op that will match on the supplied key and values.
func inCollectionOp(key string, ids ...string) bson.D {
	ret := bson.D{}
	switch len(ids) {
	case 0:
	case 1:
		ret = append(ret, bson.DocElem{key, ids[0]})
	default:
		ret = append(ret, bson.DocElem{key, bson.D{{"$in", ids}}})
	}
	return ret
}

// localIdInCollectionOp is a special form of inCollectionOp that just
// converts id's to their env-uuid prefixed form.
func localIdInCollectionOp(st *State, localIds ...string) bson.D {
	ids := make([]string, len(localIds))
	for i, id := range localIds {
		ids[i] = st.docID(id)
	}
	return inCollectionOp("_id", ids...)
}

// actionReceiverInCollectionOp is a special form of inCollectionOp
// that just converts []ActionReceiver to a []string containing the
// ActionReceiver Name() values.
func actionReceiverInCollectionOp(receivers ...ActionReceiver) bson.D {
	ids := make([]string, len(receivers))
	for i, r := range receivers {
		ids[i] = r.Tag().Id()
	}
	return inCollectionOp("receiver", ids...)
}

// statusInCollectionOp is a special form of inCollectionOp that just
// converts []ActionStatus to a []string with the same values.
func statusInCollectionOp(statusSet ...ActionStatus) bson.D {
	ids := make([]string, len(statusSet))
	for i, s := range statusSet {
		ids[i] = string(s)
	}
	return inCollectionOp("status", ids...)
}

// idPrefixWatcher is a StringsWatcher that watches for changes on the
// specified collection that match common prefixes
type idPrefixWatcher struct {
	commonWatcher
	source   chan watcher.Change
	sink     chan []string
	filterFn func(interface{}) bool
	targetC  string
}

// ensure idPrefixWatcher is a StringsWatcher
// TODO(dfc) this needs to move to a test
var _ StringsWatcher = (*idPrefixWatcher)(nil)

// newIdPrefixWatcher starts and returns a new StringsWatcher configured
// with the given collection and filter function
func newIdPrefixWatcher(st *State, collectionName string, filter func(interface{}) bool) StringsWatcher {
	w := &idPrefixWatcher{
		commonWatcher: commonWatcher{st: st},
		source:        make(chan watcher.Change),
		sink:          make(chan []string),
		filterFn:      filter,
		targetC:       collectionName,
	}

	go func() {
		defer w.tomb.Done()
		defer close(w.sink)
		defer close(w.source)
		w.tomb.Kill(w.loop())
	}()

	return w
}

// Changes returns the event channel for this watcher
func (w *idPrefixWatcher) Changes() <-chan []string {
	return w.sink
}

// loop performs the main event loop cycle, polling for changes and
// responding to Changes requests
func (w *idPrefixWatcher) loop() error {
	var (
		changes []string
		in      = (<-chan watcher.Change)(w.source)
		out     = (chan<- []string)(w.sink)
	)

	w.st.watcher.WatchCollectionWithFilter(w.targetC, w.source, w.filterFn)
	defer w.st.watcher.UnwatchCollection(w.targetC, w.source)

	changes, err := w.initial()
	if err != nil {
		return err
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case ch := <-in:
			updates, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			if err := mergeIds(w.st, &changes, updates); err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.sink
			}
		case out <- changes:
			changes = []string{}
			out = nil
		}
	}
}

// makeIdFilter constructs a predicate to filter keys that have the
// prefix matching one of the passed in ActionReceivers, or returns nil
// if tags is empty
func makeIdFilter(st *State, marker string, receivers ...ActionReceiver) func(interface{}) bool {
	if len(receivers) == 0 {
		return nil
	}
	ensureMarkerFn := ensureSuffixFn(marker)
	prefixes := make([]string, len(receivers))
	for ix, receiver := range receivers {
		prefixes[ix] = st.docID(ensureMarkerFn(receiver.Tag().Id()))
	}

	return func(key interface{}) bool {
		switch key.(type) {
		case string:
			for _, prefix := range prefixes {
				if strings.HasPrefix(key.(string), prefix) {
					return true
				}
			}
		default:
			watchLogger.Errorf("key is not type string, got %T", key)
		}
		return false
	}
}

// initial pre-loads the id's that have already been added to the
// collection that would otherwise not normally trigger the watcher
func (w *idPrefixWatcher) initial() ([]string, error) {
	var ids []string
	var doc struct {
		DocId string `bson:"_id"`
	}
	coll, closer := w.st.getCollection(w.targetC)
	defer closer()
	iter := coll.Find(nil).Iter()
	for iter.Next(&doc) {
		if w.filterFn == nil || w.filterFn(doc.DocId) {
			actionId := actionNotificationIdToActionId(w.st.localID(doc.DocId))
			ids = append(ids, actionId)
		}
	}
	return ids, iter.Close()
}

// mergeIds is used for merging actionId's and actionResultId's that
// come in via the updates map. It cleans up the pending changes to
// account for id's being removed before the watcher consumes them,
// and to account for the potential overlap between the id's that were
// pending before the watcher started, and the new id's detected by the
// watcher.
// Additionally, mergeIds strips the environment UUID prefix from the id
// before emitting it through the watcher.
func mergeIds(st *State, changes *[]string, updates map[interface{}]bool) error {
	for id, idExists := range updates {
		switch id := id.(type) {
		case string:
			localId := st.localID(id)
			actionId := actionNotificationIdToActionId(localId)
			chIx, idAlreadyInChangeset := indexOf(actionId, *changes)
			if idExists {
				if !idAlreadyInChangeset {
					*changes = append(*changes, actionId)
				}
			} else {
				if idAlreadyInChangeset {
					// remove id from changes
					*changes = append([]string(*changes)[:chIx], []string(*changes)[chIx+1:]...)
				}
			}
		default:
			return errors.Errorf("id is not of type string, got %T", id)
		}
	}
	return nil
}

func actionNotificationIdToActionId(id string) string {
	ix := strings.Index(id, actionMarker)
	if ix == -1 {
		return id
	}
	return id[ix+len(actionMarker):]
}

func indexOf(find string, in []string) (int, bool) {
	for ix, cur := range in {
		if cur == find {
			return ix, true
		}
	}
	return -1, false
}

// ensureSuffixFn returns a function that will make sure the passed in
// string has the marker token at the end of it
func ensureSuffixFn(marker string) func(string) string {
	return func(p string) string {
		if !strings.HasSuffix(p, marker) {
			p = p + marker
		}
		return p
	}
}

// watchEnqueuedActions starts and returns a StringsWatcher that
// notifies on new Actions being enqueued.
func (st *State) watchEnqueuedActions() StringsWatcher {
	return newIdPrefixWatcher(st, actionNotificationsC, makeIdFilter(st, actionMarker))
}

// watchEnqueuedActionsFilteredBy starts and returns a StringsWatcher
// that notifies on new Actions being enqueued on the ActionRecevers
// being watched.
func (st *State) watchEnqueuedActionsFilteredBy(receivers ...ActionReceiver) StringsWatcher {
	return newIdPrefixWatcher(st, actionNotificationsC, makeIdFilter(st, actionMarker, receivers...))
}

// WatchActionResults starts and returns a StringsWatcher that
// notifies on new ActionResults being added.
func (st *State) WatchActionResults() StringsWatcher {
	return st.WatchActionResultsFilteredBy()
}

// WatchActionResultsFilteredBy starts and returns a StringsWatcher
// that notifies on new ActionResults being added for the ActionRecevers
// being watched.
func (st *State) WatchActionResultsFilteredBy(receivers ...ActionReceiver) StringsWatcher {
	return newActionStatusWatcher(st, receivers, []ActionStatus{ActionCompleted, ActionCancelled, ActionFailed}...)
}

// machineInterfacesWatcher notifies about changes to all network interfaces
// of a machine. Changes include adding, removing enabling or disabling interfaces.
type machineInterfacesWatcher struct {
	commonWatcher
	machineId string
	out       chan struct{}
	in        chan watcher.Change
}

var _ NotifyWatcher = (*machineInterfacesWatcher)(nil)

// WatchInterfaces returns a new NotifyWatcher watching m's network interfaces.
func (m *Machine) WatchInterfaces() NotifyWatcher {
	return newMachineInterfacesWatcher(m)
}

func newMachineInterfacesWatcher(m *Machine) NotifyWatcher {
	w := &machineInterfacesWatcher{
		commonWatcher: commonWatcher{st: m.st},
		machineId:     m.doc.Id,
		out:           make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for w.
func (w *machineInterfacesWatcher) Changes() <-chan struct{} {
	return w.out
}

// initial retrieves the currently known interfaces and stores
// them together with their activation.
func (w *machineInterfacesWatcher) initial() (map[bson.ObjectId]bool, error) {
	known := make(map[bson.ObjectId]bool)
	doc := networkInterfaceDoc{}
	query := bson.D{{"machineid", w.machineId}}
	fields := bson.D{{"_id", 1}, {"isdisabled", 1}}

	networkInterfaces, closer := w.st.getCollection(networkInterfacesC)
	defer closer()

	iter := networkInterfaces.Find(query).Select(fields).Iter()
	for iter.Next(&doc) {
		known[doc.Id] = doc.IsDisabled
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return known, nil
}

// merge compares a number of updates to the known state
// and modifies changes accordingly.
func (w *machineInterfacesWatcher) merge(changes, initial map[bson.ObjectId]bool, updates map[interface{}]bool) error {
	networkInterfaces, closer := w.st.getCollection(networkInterfacesC)
	defer closer()
	for id, exists := range updates {
		switch id := id.(type) {
		case bson.ObjectId:
			isDisabled, known := initial[id]
			if known && !exists {
				// Well known interface has been removed.
				delete(initial, id)
				changes[id] = true
				continue
			}
			doc := networkInterfaceDoc{}
			err := networkInterfaces.FindId(id).One(&doc)
			if err != nil && err != mgo.ErrNotFound {
				return err
			}
			if doc.MachineId != w.machineId {
				// Not our machine.
				continue
			}
			if !known || isDisabled != doc.IsDisabled {
				// New interface or activation change.
				initial[id] = doc.IsDisabled
				changes[id] = true
			}
		default:
			return errors.Errorf("id is not of type object ID, got %T", id)
		}
	}
	return nil
}

func (w *machineInterfacesWatcher) loop() error {
	changes := make(map[bson.ObjectId]bool)
	in := make(chan watcher.Change)
	out := w.out

	w.st.watcher.WatchCollection(networkInterfacesC, in)
	defer w.st.watcher.UnwatchCollection(networkInterfacesC, in)

	initial, err := w.initial()
	if err != nil {
		return err
	}
	changes = initial

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case ch := <-in:
			updates, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			if err := w.merge(changes, initial, updates); err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.out
			} else {
				out = nil
			}
		case out <- struct{}{}:
			changes = make(map[bson.ObjectId]bool)
			out = nil
		}
	}
}

// openedPortsWatcher notifies of changes in the openedPorts
// collection
type openedPortsWatcher struct {
	commonWatcher
	known map[string]int64
	out   chan []string
}

var _ Watcher = (*openedPortsWatcher)(nil)

// WatchOpenedPorts starts and returns a StringsWatcher notifying of
// changes to the openedPorts collection. Reported changes have the
// following format: "<machine-id>:<network-name>", i.e.
// "0:juju-public".
func (st *State) WatchOpenedPorts() StringsWatcher {
	return newOpenedPortsWatcher(st)
}

func newOpenedPortsWatcher(st *State) StringsWatcher {
	w := &openedPortsWatcher{
		commonWatcher: commonWatcher{st: st},
		known:         make(map[string]int64),
		out:           make(chan []string),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()

	return w
}

// Changes returns the event channel for w
func (w *openedPortsWatcher) Changes() <-chan []string {
	return w.out
}

// transformId converts a global key for a ports document (e.g.
// "m#42#n#juju-public") into a colon-separated string with the
// machine id and network name (e.g. "42:juju-public").
func (w *openedPortsWatcher) transformId(globalKey string) (string, error) {
	parts, err := extractPortsIdParts(globalKey)
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf("%s:%s", parts[machineIdPart], parts[networkNamePart]), nil
}

func (w *openedPortsWatcher) initial() (set.Strings, error) {
	ports, closer := w.st.getCollection(openedPortsC)
	defer closer()

	portDocs := set.NewStrings()
	var doc portsDoc
	iter := ports.Find(nil).Select(bson.D{{"_id", 1}, {"txn-revno", 1}}).Iter()
	for iter.Next(&doc) {
		id, err := w.st.strictLocalID(doc.DocID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if doc.TxnRevno != -1 {
			w.known[id] = doc.TxnRevno
		}
		if changeId, err := w.transformId(id); err != nil {
			logger.Errorf(err.Error())
		} else {
			portDocs.Add(changeId)
		}
	}
	return portDocs, errors.Trace(iter.Close())
}

func (w *openedPortsWatcher) loop() error {
	in := make(chan watcher.Change)
	changes, err := w.initial()
	if err != nil {
		return errors.Trace(err)
	}
	w.st.watcher.WatchCollection(openedPortsC, in)
	defer w.st.watcher.UnwatchCollection(openedPortsC, in)

	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case ch := <-in:
			if err = w.merge(changes, ch); err != nil {
				return errors.Trace(err)
			}
			if !changes.IsEmpty() {
				out = w.out
			}
		case out <- changes.Values():
			out = nil
			changes = set.NewStrings()
		}
	}
}

func (w *openedPortsWatcher) merge(ids set.Strings, change watcher.Change) error {
	id, ok := change.Id.(string)
	if !ok {
		return errors.Errorf("id %v is not of type string, got %T", id, id)
	}
	localID, err := w.st.strictLocalID(id)
	if err != nil {
		return errors.Trace(err)
	}
	if change.Revno == -1 {
		delete(w.known, localID)
		if changeId, err := w.transformId(localID); err != nil {
			logger.Errorf(err.Error())
		} else {
			// Report the removed id.
			ids.Add(changeId)
		}
		return nil
	}
	openedPorts, closer := w.st.getCollection(openedPortsC)
	currentRevno, err := getTxnRevno(openedPorts, id)
	closer()
	if err != nil {
		return err
	}
	knownRevno, isKnown := w.known[localID]
	w.known[localID] = currentRevno
	if !isKnown || currentRevno > knownRevno {
		if changeId, err := w.transformId(localID); err != nil {
			logger.Errorf(err.Error())
		} else {
			// Report the unknown-so-far id.
			ids.Add(changeId)
		}
	}
	return nil
}

// WatchForRebootEvent returns a notify watcher that will trigger an event
// when the reboot flag is set on our machine agent, our parent machine agent
// or grandparent machine agent
func (m *Machine) WatchForRebootEvent() (NotifyWatcher, error) {
	machineIds := m.machinesToCareAboutRebootsFor()
	machines := set.NewStrings(machineIds...)
	return newRebootWatcher(m.st, machines), nil
}

type rebootWatcher struct {
	commonWatcher
	machines set.Strings
	out      chan struct{}
}

func newRebootWatcher(st *State, machines set.Strings) NotifyWatcher {
	w := &rebootWatcher{
		commonWatcher: commonWatcher{st: st},
		machines:      machines,
		out:           make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for the rebootWatcher.
func (w *rebootWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *rebootWatcher) loop() error {
	in := make(chan watcher.Change)
	filter := func(key interface{}) bool {
		if id, ok := key.(string); ok {
			if id, err := w.st.strictLocalID(id); err == nil {
				return w.machines.Contains(id)
			} else {
				return false
			}
		}
		w.tomb.Kill(fmt.Errorf("expected string, got %T: %v", key, key))
		return false
	}
	w.st.watcher.WatchCollectionWithFilter(rebootC, in, filter)
	defer w.st.watcher.UnwatchCollection(rebootC, in)
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case ch := <-in:
			if _, ok := collect(ch, in, w.tomb.Dying()); !ok {
				return tomb.ErrDying
			}
			out = w.out
		case out <- struct{}{}:
			out = nil

		}
	}
}

// blockDevicesWatcher notifies about changes to all block devices
// attached to a machine, optionally filtering to those assigned to
// a specific unit's datastores.
type blockDevicesWatcher struct {
	commonWatcher
	machineId string // required
	unitName  string // optional
	out       chan struct{}
}

var _ NotifyWatcher = (*blockDevicesWatcher)(nil)

// WatchAttachedBlockDevices returns a new StringsWatcher watching block
// devices attached to u's datastores.
func (u *Unit) WatchAttachedBlockDevices() (NotifyWatcher, error) {
	machineId, err := u.AssignedMachineId()
	if err != nil {
		return nil, err
	}
	return newBlockDevicesWatcher(u.st, machineId, u.Name()), nil
}

func newBlockDevicesWatcher(st *State, machineId, unitName string) NotifyWatcher {
	w := &blockDevicesWatcher{
		commonWatcher: commonWatcher{st: st},
		machineId:     machineId,
		unitName:      unitName,
		out:           make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for w.
func (w *blockDevicesWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *blockDevicesWatcher) current() ([]blockDevice, error) {
	// TODO(axw) only get attached block devices.
	blockDevices, err := getBlockDevices(w.st, w.machineId)
	if err != nil {
		return nil, err
	}
	// TODO(axw) filter by those that are assigned to datastores
	// owned by the specified unit.
	return blockDevices, nil
}

func (w *blockDevicesWatcher) loop() error {
	// Get the initial revno and construct the watcher.
	docId := w.st.docID(w.machineId)
	blockDevicesColl, closer := w.st.getCollection(blockDevicesC)
	revno, err := getTxnRevno(blockDevicesColl, docId)
	closer()
	if err != nil {
		return err
	}
	in := make(chan watcher.Change)
	w.st.watcher.Watch(blockDevicesC, docId, revno, in)
	defer w.st.watcher.Unwatch(blockDevicesC, docId, in)

	// Get the current block devices.
	blockDevices, err := w.current()
	if err != nil {
		return err
	}
	out := w.out

	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-in:
			newBlockDevices, err := w.current()
			if err != nil {
				return err
			}
			if !blockDevicesEqual(newBlockDevices, blockDevices) {
				blockDevices = newBlockDevices
				out = w.out
			}
		case out <- struct{}{}:
			out = nil
		}
	}
}
