// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/juju/loggo"
	"labix.org/v2/mgo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils/set"
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
	Changes() <-chan params.RelationUnitsChange
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
	// coll is the collection holding all interesting entities.
	coll *mgo.Collection
	// members is used to select the initial set of interesting entities.
	members D
	// filter is used to exclude events not affecting interesting entities.
	filter func(interface{}) bool
	// life holds the most recent known life states of interesting entities.
	life map[string]Life
}

// WatchServices returns a StringsWatcher that notifies of changes to
// the lifecycles of the services in the environment.
func (st *State) WatchServices() StringsWatcher {
	return newLifecycleWatcher(st, st.services, nil, nil)
}

// WatchUnits returns a StringsWatcher that notifies of changes to the
// lifecycles of units of s.
func (s *Service) WatchUnits() StringsWatcher {
	members := D{{"service", s.doc.Name}}
	prefix := s.doc.Name + "/"
	filter := func(id interface{}) bool {
		return strings.HasPrefix(id.(string), prefix)
	}
	return newLifecycleWatcher(s.st, s.st.units, members, filter)
}

// WatchRelations returns a StringsWatcher that notifies of changes to the
// lifecycles of relations involving s.
func (s *Service) WatchRelations() StringsWatcher {
	members := D{{"endpoints.servicename", s.doc.Name}}
	prefix := s.doc.Name + ":"
	infix := " " + prefix
	filter := func(key interface{}) bool {
		k := key.(string)
		return strings.HasPrefix(k, prefix) || strings.Contains(k, infix)
	}
	return newLifecycleWatcher(s.st, s.st.relations, members, filter)
}

// WatchEnvironMachines returns a StringsWatcher that notifies of changes to
// the lifecycles of the machines (but not containers) in the environment.
func (st *State) WatchEnvironMachines() StringsWatcher {
	members := D{{"$or", []D{
		{{"containertype", ""}},
		{{"containertype", D{{"$exists", false}}}},
	}}}
	filter := func(id interface{}) bool {
		return !strings.Contains(id.(string), "/")
	}
	return newLifecycleWatcher(st, st.machines, members, filter)
}

// WatchContainers returns a StringsWatcher that notifies of changes to the
// lifecycles of containers of the specified type on a machine.
func (m *Machine) WatchContainers(ctype instance.ContainerType) StringsWatcher {
	isChild := fmt.Sprintf("^%s/%s/%s$", m.doc.Id, ctype, names.NumberSnippet)
	return m.containersWatcher(isChild)
}

// WatchAllContainers returns a StringsWatcher that notifies of changes to the
// lifecycles of all containers on a machine.
func (m *Machine) WatchAllContainers() StringsWatcher {
	isChild := fmt.Sprintf("^%s/%s/%s$", m.doc.Id, names.ContainerTypeSnippet, names.NumberSnippet)
	return m.containersWatcher(isChild)
}

func (m *Machine) containersWatcher(isChildRegexp string) StringsWatcher {
	members := D{{"_id", D{{"$regex", isChildRegexp}}}}
	compiled := regexp.MustCompile(isChildRegexp)
	filter := func(key interface{}) bool {
		return compiled.MatchString(key.(string))
	}
	return newLifecycleWatcher(m.st, m.st.machines, members, filter)
}

func newLifecycleWatcher(st *State, coll *mgo.Collection, members D, filter func(key interface{}) bool) StringsWatcher {
	w := &lifecycleWatcher{
		commonWatcher: commonWatcher{st: st},
		coll:          coll,
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

var lifeFields = D{{"_id", 1}, {"life", 1}}

// Changes returns the event channel for the LifecycleWatcher.
func (w *lifecycleWatcher) Changes() <-chan []string {
	return w.out
}

func (w *lifecycleWatcher) initial() (ids *set.Strings, err error) {
	ids = &set.Strings{}
	var doc lifeDoc
	iter := w.coll.Find(w.members).Select(lifeFields).Iter()
	for iter.Next(&doc) {
		ids.Add(doc.Id)
		if doc.Life != Dead {
			w.life[doc.Id] = doc.Life
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (w *lifecycleWatcher) merge(ids *set.Strings, updates map[interface{}]bool) error {
	// Separate ids into those thought to exist and those known to be removed.
	changed := []string{}
	latest := map[string]Life{}
	for id, exists := range updates {
		id := id.(string)
		if exists {
			changed = append(changed, id)
		} else {
			latest[id] = Dead
		}
	}

	// Collect life states from ids thought to exist. Any that don't actually
	// exist are ignored (we'll hear about them in the next set of updates --
	// all that's actually happened in that situation is that the watcher
	// events have lagged a little behind reality).
	iter := w.coll.Find(D{{"_id", D{{"$in", changed}}}}).Select(lifeFields).Iter()
	var doc lifeDoc
	for iter.Next(&doc) {
		latest[doc.Id] = doc.Life
	}
	if err := iter.Err(); err != nil {
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
	w.st.watcher.WatchCollectionWithFilter(w.coll.Name, in, w.filter)
	defer w.st.watcher.UnwatchCollection(w.coll.Name, in)
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
			ids = &set.Strings{}
			out = nil
		}
	}
	return nil
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

func (st *State) WatchMinUnits() StringsWatcher {
	return newMinUnitsWatcher(st)
}

func (w *minUnitsWatcher) initial() (*set.Strings, error) {
	serviceNames := new(set.Strings)
	doc := &minUnitsDoc{}
	iter := w.st.minUnits.Find(nil).Iter()
	for iter.Next(doc) {
		w.known[doc.ServiceName] = doc.Revno
		serviceNames.Add(doc.ServiceName)
	}
	return serviceNames, iter.Err()
}

func (w *minUnitsWatcher) merge(serviceNames *set.Strings, change watcher.Change) error {
	serviceName := change.Id.(string)
	if change.Revno == -1 {
		delete(w.known, serviceName)
		serviceNames.Remove(serviceName)
		return nil
	}
	doc := minUnitsDoc{}
	if err := w.st.minUnits.FindId(serviceName).One(&doc); err != nil {
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
	w.st.watcher.WatchCollection(w.st.minUnits.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.minUnits.Name, ch)
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
			serviceNames = new(set.Strings)
		}
	}
	return nil
}

func (w *minUnitsWatcher) Changes() <-chan []string {
	return w.out
}

// RelationScopeWatcher observes changes to the set of units
// in a particular relation scope.
type RelationScopeWatcher struct {
	commonWatcher
	prefix     string
	ignore     string
	knownUnits set.Strings
	out        chan *RelationScopeChange
}

var _ Watcher = (*RelationScopeWatcher)(nil)

// RelationScopeChange contains information about units that have
// entered or left a particular scope.
type RelationScopeChange struct {
	Entered []string
	Left    []string
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

func (changes *RelationScopeChange) isEmpty() bool {
	return len(changes.Entered)+len(changes.Left) == 0
}

func (w *RelationScopeWatcher) mergeChange(changes *RelationScopeChange, ch watcher.Change) (err error) {
	doc := &relationScopeDoc{ch.Id.(string)}
	if !strings.HasPrefix(doc.Key, w.prefix) {
		return nil
	}
	name := doc.unitName()
	if name == w.ignore {
		return nil
	}
	if ch.Revno == -1 {
		if w.knownUnits.Contains(name) {
			changes.Left = append(changes.Left, name)
			w.knownUnits.Remove(name)
		}
		return nil
	}
	if !w.knownUnits.Contains(name) {
		changes.Entered = append(changes.Entered, name)
		w.knownUnits.Add(name)
	}
	return nil
}

func (w *RelationScopeWatcher) getInitialEvent() (initial *RelationScopeChange, err error) {
	changes := &RelationScopeChange{}
	docs := []relationScopeDoc{}
	sel := D{{"_id", D{{"$regex", "^" + w.prefix}}}}
	err = w.st.relationScopes.Find(sel).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, doc := range docs {
		if name := doc.unitName(); name != w.ignore {
			changes.Entered = append(changes.Entered, name)
			w.knownUnits.Add(name)
		}
	}
	return changes, nil
}

func (w *RelationScopeWatcher) loop() error {
	ch := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.relationScopes.Name, ch)
	defer w.st.watcher.UnwatchCollection(w.st.relationScopes.Name, ch)
	changes, err := w.getInitialEvent()
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
		case c := <-ch:
			if err := w.mergeChange(changes, c); err != nil {
				return err
			}
			if !changes.isEmpty() {
				out = w.out
			}
		case out <- changes:
			changes = &RelationScopeChange{}
			out = nil
		}
	}
	return nil
}

// relationUnitsWatcher sends notifications of units entering and leaving the
// scope of a RelationUnit, and changes to the settings of those units known
// to have entered.
type relationUnitsWatcher struct {
	commonWatcher
	sw       *RelationScopeWatcher
	watching set.Strings
	updates  chan watcher.Change
	out      chan params.RelationUnitsChange
}

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
		updates:       make(chan watcher.Change),
		out:           make(chan params.RelationUnitsChange),
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
func (w *relationUnitsWatcher) Changes() <-chan params.RelationUnitsChange {
	return w.out
}

func emptyRelationUnitsChanges(changes *params.RelationUnitsChange) bool {
	return len(changes.Changed)+len(changes.Departed) == 0
}

// mergeSettings reads the relation settings node for the unit with the
// supplied id, and sets a value in the Changed field keyed on the unit's
// name. It returns the mgo/txn revision number of the settings node.
func (w *relationUnitsWatcher) mergeSettings(changes *params.RelationUnitsChange, key string) (int64, error) {
	node, err := readSettings(w.st, key)
	if err != nil {
		return -1, err
	}
	name := (&relationScopeDoc{key}).unitName()
	settings := params.UnitSettings{Version: node.txnRevno}
	if changes.Changed == nil {
		changes.Changed = map[string]params.UnitSettings{name: settings}
	} else {
		changes.Changed[name] = settings
	}
	return node.txnRevno, nil
}

// mergeScope starts and stops settings watches on the units entering and
// leaving the scope in the supplied RelationScopeChange event, and applies
// the expressed changes to the supplied RelationUnitsChange event.
func (w *relationUnitsWatcher) mergeScope(changes *params.RelationUnitsChange, c *RelationScopeChange) error {
	for _, name := range c.Entered {
		key := w.sw.prefix + name
		revno, err := w.mergeSettings(changes, key)
		if err != nil {
			return err
		}
		changes.Departed = remove(changes.Departed, name)
		w.st.watcher.Watch(w.st.settings.Name, key, revno, w.updates)
		w.watching.Add(key)
	}
	for _, name := range c.Left {
		key := w.sw.prefix + name
		changes.Departed = append(changes.Departed, name)
		if changes.Changed != nil {
			delete(changes.Changed, name)
		}
		w.st.watcher.Unwatch(w.st.settings.Name, key, w.updates)
		w.watching.Remove(key)
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
		w.st.watcher.Unwatch(w.st.settings.Name, watchedValue, w.updates)
	}
	close(w.updates)
	close(w.out)
	w.tomb.Done()
}

func (w *relationUnitsWatcher) loop() (err error) {
	sentInitial := false
	changes := params.RelationUnitsChange{}
	out := w.out
	out = nil
	for {
		select {
		case <-w.st.watcher.Dead():
			return stateWatcherDeadError(w.st.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case c, ok := <-w.sw.Changes():
			if !ok {
				return watcher.MustErr(w.sw)
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
			if _, err = w.mergeSettings(&changes, c.Id.(string)); err != nil {
				return err
			}
			out = w.out
		case out <- changes:
			sentInitial = true
			changes = params.RelationUnitsChange{}
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
	coll := u.st.units.Name
	getUnits := func() ([]string, error) {
		if err := u.Refresh(); err != nil {
			return nil, err
		}
		return u.doc.Subordinates, nil
	}
	return newUnitsWatcher(u.st, u.Tag(), getUnits, coll, u.doc.Name)
}

// WatchPrincipalUnits returns a StringsWatcher tracking the machine's principal
// units.
func (m *Machine) WatchPrincipalUnits() StringsWatcher {
	m = &Machine{st: m.st, doc: m.doc}
	coll := m.st.machines.Name
	getUnits := func() ([]string, error) {
		if err := m.Refresh(); err != nil {
			return nil, err
		}
		return m.doc.Principals, nil
	}
	return newUnitsWatcher(m.st, m.Tag(), getUnits, coll, m.doc.Id)
}

func newUnitsWatcher(st *State, tag string, getUnits func() ([]string, error), coll, id string) StringsWatcher {
	w := &unitsWatcher{
		commonWatcher: commonWatcher{st: st},
		tag:           tag,
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
var lifeWatchFields = D{{"_id", 1}, {"life", 1}, {"txn-revno", 1}}

// initial returns every member of the tracked set.
func (w *unitsWatcher) initial() ([]string, error) {
	initial, err := w.getUnits()
	if err != nil {
		return nil, err
	}
	docs := []lifeWatchDoc{}
	query := D{{"_id", D{{"$in", initial}}}}
	if err := w.st.units.Find(query).Select(lifeWatchFields).All(&docs); err != nil {
		return nil, err
	}
	changes := []string{}
	for _, doc := range docs {
		changes = append(changes, doc.Id)
		if doc.Life != Dead {
			w.life[doc.Id] = doc.Life
			w.st.watcher.Watch(w.st.units.Name, doc.Id, doc.TxnRevno, w.in)
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
		w.st.watcher.Unwatch(w.st.units.Name, name, w.in)
	}
	return changes, nil
}

// merge adds to and returns changes, such that it contains the supplied unit
// name if that unit is unknown and non-Dead, or has changed lifecycle status.
func (w *unitsWatcher) merge(changes []string, name string) ([]string, error) {
	doc := lifeWatchDoc{}
	err := w.st.units.FindId(name).Select(lifeWatchFields).One(&doc)
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
		w.st.watcher.Unwatch(w.st.units.Name, name, w.in)
	case !known && !gone:
		w.st.watcher.Watch(w.st.units.Name, name, doc.TxnRevno, w.in)
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
	revno, err := getTxnRevno(w.st.db.C(coll), id)
	if err != nil {
		return err
	}
	w.st.watcher.Watch(coll, id, revno, w.in)
	defer func() {
		w.st.watcher.Unwatch(coll, id, w.in)
		for name := range w.life {
			w.st.watcher.Unwatch(w.st.units.Name, name, w.in)
		}
	}()
	changes, err := w.initial()
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
		case c := <-w.in:
			name := c.Id.(string)
			if name == id {
				changes, err = w.update(changes)
			} else {
				changes, err = w.merge(changes, name)
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
	return nil
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
func (s *State) WatchEnvironConfig() *EnvironConfigWatcher {
	return newEnvironConfigWatcher(s)
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
				return watcher.MustErr(sw)
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
	return nil
}

type settingsWatcher struct {
	commonWatcher
	out chan *Settings
}

var _ Watcher = (*settingsWatcher)(nil)

// watchSettings creates a watcher for observing changes to settings.
func (s *State) watchSettings(key string) *settingsWatcher {
	return newSettingsWatcher(s, key)
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
	} else if !errors.IsNotFoundError(err) {
		return err
	}
	w.st.watcher.Watch(w.st.settings.Name, key, revno, ch)
	defer w.st.watcher.Unwatch(w.st.settings.Name, key, ch)
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
	return nil
}

// entityWatcher generates an event when a document in the db changes
type entityWatcher struct {
	commonWatcher
	out chan struct{}
}

var _ Watcher = (*entityWatcher)(nil)

// WatchHardwareCharacteristics returns a watcher for observing changes to a machine's hardware characteristics.
func (m *Machine) WatchHardwareCharacteristics() NotifyWatcher {
	return newEntityWatcher(m.st, m.st.instanceData, m.doc.Id)
}

func (st *State) WatchStateServerInfo() NotifyWatcher {
	return newEntityWatcher(st, st.stateServers, environGlobalKey)
}

// Watch returns a watcher for observing changes to a machine.
func (m *Machine) Watch() NotifyWatcher {
	return newEntityWatcher(m.st, m.st.machines, m.doc.Id)
}

// Watch returns a watcher for observing changes to a service.
func (s *Service) Watch() NotifyWatcher {
	return newEntityWatcher(s.st, s.st.services, s.doc.Name)
}

// Watch returns a watcher for observing changes to a unit.
func (u *Unit) Watch() NotifyWatcher {
	return newEntityWatcher(u.st, u.st.units, u.doc.Name)
}

// Watch returns a watcher for observing changes to an environment.
func (e *Environment) Watch() NotifyWatcher {
	return newEntityWatcher(e.st, e.st.environments, e.doc.UUID)
}

// WatchForEnvironConfigChanges return a NotifyWatcher waiting for the Environ
// Config to change. This differs from WatchEnvironConfig in that the watcher
// is a NotifyWatcher that does not give content during Changes()
func (st *State) WatchForEnvironConfigChanges() NotifyWatcher {
	return newEntityWatcher(st, st.settings, environGlobalKey)
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
	return newEntityWatcher(u.st, u.st.settings, settingsKey), nil
}

func newEntityWatcher(st *State, coll *mgo.Collection, key string) NotifyWatcher {
	w := &entityWatcher{
		commonWatcher: commonWatcher{st: st},
		out:           make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(coll, key))
	}()
	return w
}

// Changes returns the event channel for the entityWatcher.
func (w *entityWatcher) Changes() <-chan struct{} {
	return w.out
}

// getTxnRevo returns the transaction revision number of the
// given key in the given collection. It is useful to enable
// a watcher.Watcher to be primed with the correct revision
// id.
func getTxnRevno(coll *mgo.Collection, key string) (int64, error) {
	doc := &struct {
		TxnRevno int64 `bson:"txn-revno"`
	}{}
	fields := D{{"txn-revno", 1}}
	if err := coll.FindId(key).Select(fields).One(doc); err == mgo.ErrNotFound {
		return -1, nil
	} else if err != nil {
		return 0, err
	}
	return doc.TxnRevno, nil
}

func (w *entityWatcher) loop(coll *mgo.Collection, key string) error {
	txnRevno, err := getTxnRevno(coll, key)
	if err != nil {
		return err
	}
	in := make(chan watcher.Change)
	w.st.watcher.Watch(coll.Name, key, txnRevno, in)
	defer w.st.watcher.Unwatch(coll.Name, key, in)
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
	return nil
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
	for _, unit := range w.machine.doc.Principals {
		if _, ok := w.known[unit]; !ok {
			pending, err = w.merge(pending, unit)
			if err != nil {
				return nil, err
			}
		}
	}
	return pending, nil
}

func (w *machineUnitsWatcher) merge(pending []string, unit string) (new []string, err error) {
	doc := unitDoc{}
	err = w.st.units.FindId(unit).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, err
	}
	life, known := w.known[unit]
	if err == mgo.ErrNotFound || doc.Principal == "" && (doc.MachineId == "" || doc.MachineId != w.machine.doc.Id) {
		// Unit was removed or unassigned from w.machine.
		if known {
			delete(w.known, unit)
			w.st.watcher.Unwatch(w.st.units.Name, unit, w.in)
			if life != Dead && !hasString(pending, unit) {
				pending = append(pending, unit)
			}
			for _, subunit := range doc.Subordinates {
				if sublife, subknown := w.known[subunit]; subknown {
					delete(w.known, subunit)
					w.st.watcher.Unwatch(w.st.units.Name, subunit, w.in)
					if sublife != Dead && !hasString(pending, subunit) {
						pending = append(pending, subunit)
					}
				}
			}
		}
		return pending, nil
	}
	if !known {
		w.st.watcher.Watch(w.st.units.Name, unit, doc.TxnRevno, w.in)
		pending = append(pending, unit)
	} else if life != doc.Life && !hasString(pending, unit) {
		pending = append(pending, unit)
	}
	w.known[unit] = doc.Life
	for _, subunit := range doc.Subordinates {
		if _, ok := w.known[subunit]; !ok {
			pending, err = w.merge(pending, subunit)
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
			w.st.watcher.Unwatch(w.st.units.Name, unit, w.in)
		}
	}()
	revno, err := getTxnRevno(w.st.machines, w.machine.doc.Id)
	if err != nil {
		return err
	}
	machineCh := make(chan watcher.Change)
	w.st.watcher.Watch(w.st.machines.Name, w.machine.doc.Id, revno, machineCh)
	defer w.st.watcher.Unwatch(w.st.machines.Name, w.machine.doc.Id, machineCh)
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
			changes, err = w.merge(changes, c.Id.(string))
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

	w.st.watcher.WatchCollection(w.st.cleanups.Name, in)
	defer w.st.watcher.UnwatchCollection(w.st.cleanups.Name, in)

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
