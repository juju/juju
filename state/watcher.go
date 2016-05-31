// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/tomb"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/state/workers"

	// TODO(fwereade): 2015-11-18 lp:1517428
	//
	// This gets an import block of its own because it's such staggeringly bad
	// practice. It's here because (1) it always has been, just not quite so
	// explicitly and (2) even if we had the state watchers implemented as
	// juju/watcher~s rather than juju/state/watcher~s -- which we don't, so
	// it's misleading to use those *Chan types etc -- we don't yet have any
	// ability to transform watcher output in the apiserver layer, so we're
	// kinda stuck producing what we always have.
	//
	// See RelationUnitsWatcher below.
	"github.com/juju/juju/apiserver/params"
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

	// Note that it's not very nice exposing a params type directly here. This
	// is a continuation of existing bad behaviour and not good practice; do
	// not use this as a model. (FWIW, it used to be in multiwatcher; which is
	// also api-ey; and the multiwatcher type was used directly in params
	// anyway.)
	Changes() <-chan params.RelationUnitsChange
}

// newCommonWatcher exists so that all embedders have a place from which
// to get a single TxnLogWatcher that will not be replaced in the lifetime
// of the embedder (and also to restrict the width of the interface by
// which they can access the rest of State, by storing st as a
// modelBackend).
func newCommonWatcher(st *State) commonWatcher {
	return commonWatcher{
		st:      st,
		watcher: st.workers.TxnLogWatcher(),
	}
}

// commonWatcher is part of all client watchers.
type commonWatcher struct {
	st      modelBackend
	watcher workers.TxnLogWatcher
	tomb    tomb.Tomb
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
	// TODO(fwereade): 2016-03-17 lp:1558657
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
// the same kind. The first event emitted will contain the ids of all
// entities; subsequent events are emitted whenever one or more entities are
// added, or change their lifecycle state. After an entity is found to be
// Dead, no further event will include it.
type lifecycleWatcher struct {
	commonWatcher
	out chan []string

	// coll is a function returning the mongo.Collection holding all
	// interesting entities
	coll     func() (mongo.Collection, func())
	collName string

	// members is used to select the initial set of interesting entities.
	members bson.D
	// filter is used to exclude events not affecting interesting entities.
	filter func(interface{}) bool
	// transform, if non-nil, is used to transform a document ID immediately
	// prior to emitting to the out channel.
	transform func(string) string
	// life holds the most recent known life states of interesting entities.
	life map[string]Life
}

func collFactory(st *State, collName string) func() (mongo.Collection, func()) {
	return func() (mongo.Collection, func()) {
		return st.getCollection(collName)
	}
}

// WatchModels returns a StringsWatcher that notifies of changes
// to the lifecycles of all models.
func (st *State) WatchModels() StringsWatcher {
	return newLifecycleWatcher(st, modelsC, nil, nil, nil)
}

// WatchIPAddresses returns a StringsWatcher that notifies of changes to the
// lifecycles of IP addresses.
func (st *State) WatchIPAddresses() StringsWatcher {
	return newLifecycleWatcher(st, legacyipaddressesC, nil, nil, nil)
}

// WatchModelVolumes returns a StringsWatcher that notifies of changes to
// the lifecycles of all model-scoped volumes.
func (st *State) WatchModelVolumes() StringsWatcher {
	return st.watchModelMachinestorage(volumesC)
}

// WatchModelFilesystems returns a StringsWatcher that notifies of changes
// to the lifecycles of all model-scoped filesystems.
func (st *State) WatchModelFilesystems() StringsWatcher {
	return st.watchModelMachinestorage(filesystemsC)
}

func (st *State) watchModelMachinestorage(collection string) StringsWatcher {
	pattern := fmt.Sprintf("^%s$", st.docID(names.NumberSnippet))
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return !strings.Contains(k, "/")
	}
	return newLifecycleWatcher(st, collection, members, filter, nil)
}

// WatchMachineVolumes returns a StringsWatcher that notifies of changes to
// the lifecycles of all volumes scoped to the specified machine.
func (st *State) WatchMachineVolumes(m names.MachineTag) StringsWatcher {
	return st.watchMachineStorage(m, volumesC)
}

// WatchMachineFilesystems returns a StringsWatcher that notifies of changes
// to the lifecycles of all filesystems scoped to the specified machine.
func (st *State) WatchMachineFilesystems(m names.MachineTag) StringsWatcher {
	return st.watchMachineStorage(m, filesystemsC)
}

func (st *State) watchMachineStorage(m names.MachineTag, collection string) StringsWatcher {
	pattern := fmt.Sprintf("^%s/%s$", st.docID(m.Id()), names.NumberSnippet)
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	prefix := m.Id() + "/"
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(k, prefix)
	}
	return newLifecycleWatcher(st, collection, members, filter, nil)
}

// WatchEnvironVolumeAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all volume attachments related to environ-
// scoped volumes.
func (st *State) WatchEnvironVolumeAttachments() StringsWatcher {
	return st.watchModelMachinestorageAttachments(volumeAttachmentsC)
}

// WatchEnvironFilesystemAttachments returns a StringsWatcher that notifies
// of changes to the lifecycles of all filesystem attachments related to
// environ-scoped filesystems.
func (st *State) WatchEnvironFilesystemAttachments() StringsWatcher {
	return st.watchModelMachinestorageAttachments(filesystemAttachmentsC)
}

func (st *State) watchModelMachinestorageAttachments(collection string) StringsWatcher {
	pattern := fmt.Sprintf("^%s.*:%s$", st.docID(""), names.NumberSnippet)
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		colon := strings.IndexRune(k, ':')
		if colon == -1 {
			return false
		}
		return !strings.Contains(k[colon+1:], "/")
	}
	return newLifecycleWatcher(st, collection, members, filter, nil)
}

// WatchMachineVolumeAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all volume attachments related to the specified
// machine, for volumes scoped to the machine.
func (st *State) WatchMachineVolumeAttachments(m names.MachineTag) StringsWatcher {
	return st.watchMachineStorageAttachments(m, volumeAttachmentsC)
}

// WatchMachineFilesystemAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all filesystem attachments related to the specified
// machine, for filesystems scoped to the machine.
func (st *State) WatchMachineFilesystemAttachments(m names.MachineTag) StringsWatcher {
	return st.watchMachineStorageAttachments(m, filesystemAttachmentsC)
}

func (st *State) watchMachineStorageAttachments(m names.MachineTag, collection string) StringsWatcher {
	pattern := fmt.Sprintf("^%s:%s/.*", st.docID(m.Id()), m.Id())
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	prefix := m.Id() + fmt.Sprintf(":%s/", m.Id())
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(k, prefix)
	}
	return newLifecycleWatcher(st, collection, members, filter, nil)
}

// WatchServices returns a StringsWatcher that notifies of changes to
// the lifecycles of the services in the model.
func (st *State) WatchServices() StringsWatcher {
	return newLifecycleWatcher(st, applicationsC, nil, isLocalID(st), nil)
}

// WatchStorageAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all storage instances attached to the
// specified unit.
func (st *State) WatchStorageAttachments(unit names.UnitTag) StringsWatcher {
	members := bson.D{{"unitid", unit.Id()}}
	prefix := unitGlobalKey(unit.Id()) + "#"
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(k, prefix)
	}
	tr := func(id string) string {
		// Transform storage attachment document ID to storage ID.
		return id[len(prefix):]
	}
	return newLifecycleWatcher(st, storageAttachmentsC, members, filter, tr)
}

// WatchUnits returns a StringsWatcher that notifies of changes to the
// lifecycles of units of s.
func (s *Application) WatchUnits() StringsWatcher {
	members := bson.D{{"application", s.doc.Name}}
	prefix := s.doc.Name + "/"
	filter := func(unitDocID interface{}) bool {
		unitName, err := s.st.strictLocalID(unitDocID.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(unitName, prefix)
	}
	return newLifecycleWatcher(s.st, unitsC, members, filter, nil)
}

// WatchRelations returns a StringsWatcher that notifies of changes to the
// lifecycles of relations involving s.
func (s *Application) WatchRelations() StringsWatcher {
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

	members := bson.D{{"endpoints.applicationname", s.doc.Name}}
	return newLifecycleWatcher(s.st, relationsC, members, filter, nil)
}

// WatchModelMachines returns a StringsWatcher that notifies of changes to
// the lifecycles of the machines (but not containers) in the model.
func (st *State) WatchModelMachines() StringsWatcher {
	members := bson.D{{"$or", []bson.D{
		{{"containertype", ""}},
		{{"containertype", bson.D{{"$exists", false}}}},
	}}}
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return !strings.Contains(k, "/")
	}
	return newLifecycleWatcher(st, machinesC, members, filter, nil)
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
		k := key.(string)
		_, err := m.st.strictLocalID(k)
		if err != nil {
			return false
		}
		return compiled.MatchString(k)
	}
	return newLifecycleWatcher(m.st, machinesC, members, filter, nil)
}

func newLifecycleWatcher(
	st *State,
	collName string,
	members bson.D,
	filter func(key interface{}) bool,
	transform func(id string) string,
) StringsWatcher {
	w := &lifecycleWatcher{
		commonWatcher: newCommonWatcher(st),
		coll:          collFactory(st, collName),
		collName:      collName,
		members:       members,
		filter:        filter,
		transform:     transform,
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
	w.watcher.WatchCollectionWithFilter(w.collName, in, w.filter)
	defer w.watcher.UnwatchCollection(w.collName, in)
	ids, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		values := ids.Values()
		if w.transform != nil {
			for i, v := range values {
				values[i] = w.transform(v)
			}
		}
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
		case out <- values:
			ids = make(set.Strings)
			out = nil
		}
	}
}

// minUnitsWatcher notifies about MinUnits changes of the services requiring
// a minimum number of units to be alive. The first event returned by the
// watcher is the set of application names requiring a minimum number of units.
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
		commonWatcher: newCommonWatcher(st),
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
	applicationnames := make(set.Strings)
	var doc minUnitsDoc
	newMinUnits, closer := w.st.getCollection(minUnitsC)
	defer closer()

	iter := newMinUnits.Find(nil).Iter()
	for iter.Next(&doc) {
		w.known[doc.ApplicationName] = doc.Revno
		applicationnames.Add(doc.ApplicationName)
	}
	return applicationnames, iter.Close()
}

func (w *minUnitsWatcher) merge(applicationnames set.Strings, change watcher.Change) error {
	applicationname := w.st.localID(change.Id.(string))
	if change.Revno == -1 {
		delete(w.known, applicationname)
		applicationnames.Remove(applicationname)
		return nil
	}
	doc := minUnitsDoc{}
	newMinUnits, closer := w.st.getCollection(minUnitsC)
	defer closer()
	if err := newMinUnits.FindId(change.Id).One(&doc); err != nil {
		return err
	}
	revno, known := w.known[applicationname]
	w.known[applicationname] = doc.Revno
	if !known || doc.Revno > revno {
		applicationnames.Add(applicationname)
	}
	return nil
}

func (w *minUnitsWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.watcher.WatchCollectionWithFilter(minUnitsC, ch, isLocalID(w.st))
	defer w.watcher.UnwatchCollection(minUnitsC, ch)
	applicationnames, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case change := <-ch:
			if err = w.merge(applicationnames, change); err != nil {
				return err
			}
			if !applicationnames.IsEmpty() {
				out = w.out
			}
		case out <- applicationnames.Values():
			out = nil
			applicationnames = set.NewStrings()
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
		commonWatcher: newCommonWatcher(st),
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
	w.watcher.WatchCollectionWithFilter(relationScopesC, in, filter)
	defer w.watcher.UnwatchCollection(relationScopesC, in)
	info, err := w.initialInfo()
	if err != nil {
		return err
	}
	sent := false
	out := w.out
	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
	out      chan params.RelationUnitsChange
}

// Watch returns a watcher that notifies of changes to conterpart units in
// the relation.
func (ru *RelationUnit) Watch() RelationUnitsWatcher {
	return newRelationUnitsWatcher(ru)
}

func newRelationUnitsWatcher(ru *RelationUnit) RelationUnitsWatcher {
	w := &relationUnitsWatcher{
		commonWatcher: newCommonWatcher(ru.st),
		sw:            ru.WatchScope(),
		watching:      make(set.Strings),
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

func setRelationUnitChangeVersion(changes *params.RelationUnitsChange, key string, version int64) {
	name := unitNameFromScopeKey(key)
	settings := params.UnitSettings{Version: version}
	if changes.Changed == nil {
		changes.Changed = map[string]params.UnitSettings{}
	}
	changes.Changed[name] = settings
}

// mergeSettings reads the relation settings node for the unit with the
// supplied key, and sets a value in the Changed field keyed on the unit's
// name. It returns the mgo/txn revision number of the settings node.
func (w *relationUnitsWatcher) mergeSettings(changes *params.RelationUnitsChange, key string) (int64, error) {
	var doc struct {
		TxnRevno int64 `bson:"txn-revno"`
		Version  int64 `bson:"version"`
	}
	if err := readSettingsDocInto(w.st, key, &doc); err != nil {
		return -1, err
	}
	setRelationUnitChangeVersion(changes, key, doc.Version)
	return doc.TxnRevno, nil
}

// mergeScope starts and stops settings watches on the units entering and
// leaving the scope in the supplied RelationScopeChange event, and applies
// the expressed changes to the supplied RelationUnitsChange event.
func (w *relationUnitsWatcher) mergeScope(changes *params.RelationUnitsChange, c *RelationScopeChange) error {
	for _, name := range c.Entered {
		key := w.sw.prefix + name
		docID := w.st.docID(key)
		revno, err := w.mergeSettings(changes, key)
		if err != nil {
			return err
		}
		changes.Departed = remove(changes.Departed, name)
		w.watcher.Watch(settingsC, docID, revno, w.updates)
		w.watching.Add(docID)
	}
	for _, name := range c.Left {
		key := w.sw.prefix + name
		docID := w.st.docID(key)
		changes.Departed = append(changes.Departed, name)
		if changes.Changed != nil {
			delete(changes.Changed, name)
		}
		w.watcher.Unwatch(settingsC, docID, w.updates)
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
		w.watcher.Unwatch(settingsC, watchedValue, w.updates)
	}
	close(w.updates)
	close(w.out)
	w.tomb.Done()
}

func (w *relationUnitsWatcher) loop() (err error) {
	var (
		sentInitial bool
		changes     params.RelationUnitsChange
		out         chan<- params.RelationUnitsChange
	)
	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
			if _, err := w.mergeSettings(&changes, id); err != nil {
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
		commonWatcher: newCommonWatcher(st),
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
			w.watcher.Watch(unitsC, doc.Id, doc.TxnRevno, w.in)
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
		w.watcher.Unwatch(unitsC, w.st.docID(name), w.in)
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
		w.watcher.Unwatch(unitsC, unitDocID, w.in)
	case !known && !gone:
		w.watcher.Watch(unitsC, unitDocID, doc.TxnRevno, w.in)
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

	w.watcher.Watch(coll, id, revno, w.in)
	defer func() {
		w.watcher.Unwatch(coll, id, w.in)
		for name := range w.life {
			w.watcher.Unwatch(unitsC, w.st.docID(name), w.in)
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
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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

// WatchHardwareCharacteristics returns a watcher for observing changes to a machine's hardware characteristics.
func (m *Machine) WatchHardwareCharacteristics() NotifyWatcher {
	return newEntityWatcher(m.st, instanceDataC, m.doc.DocID)
}

// WatchControllerInfo returns a NotifyWatcher for the controllers collection
func (st *State) WatchControllerInfo() NotifyWatcher {
	return newEntityWatcher(st, controllersC, modelGlobalKey)
}

// Watch returns a watcher for observing changes to a machine.
func (m *Machine) Watch() NotifyWatcher {
	return newEntityWatcher(m.st, machinesC, m.doc.DocID)
}

// Watch returns a watcher for observing changes to a service.
func (s *Application) Watch() NotifyWatcher {
	return newEntityWatcher(s.st, applicationsC, s.doc.DocID)
}

// WatchLeaderSettings returns a watcher for observing changed to a service's
// leader settings.
func (s *Application) WatchLeaderSettings() NotifyWatcher {
	docId := s.st.docID(leadershipSettingsKey(s.Name()))
	return newEntityWatcher(s.st, settingsC, docId)
}

// Watch returns a watcher for observing changes to a unit.
func (u *Unit) Watch() NotifyWatcher {
	return newEntityWatcher(u.st, unitsC, u.doc.DocID)
}

// Watch returns a watcher for observing changes to an model.
func (e *Model) Watch() NotifyWatcher {
	return newEntityWatcher(e.st, modelsC, e.doc.UUID)
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

// WatchForModelConfigChanges returns a NotifyWatcher waiting for the Model
// Config to change.
func (st *State) WatchForModelConfigChanges() NotifyWatcher {
	return newEntityWatcher(st, settingsC, st.docID(modelGlobalKey))
}

// WatchForUnitAssignment watches for new services that request units to be
// assigned to machines.
func (st *State) WatchForUnitAssignment() StringsWatcher {
	return newcollectionWatcher(st, colWCfg{col: assignUnitC})
}

// WatchAPIHostPorts returns a NotifyWatcher that notifies
// when the set of API addresses changes.
func (st *State) WatchAPIHostPorts() NotifyWatcher {
	return newEntityWatcher(st, controllersC, apiHostPortsKey)
}

// WatchStorageAttachment returns a watcher for observing changes
// to a storage attachment.
func (st *State) WatchStorageAttachment(s names.StorageTag, u names.UnitTag) NotifyWatcher {
	id := storageAttachmentId(u.Id(), s.Id())
	return newEntityWatcher(st, storageAttachmentsC, st.docID(id))
}

// WatchVolumeAttachment returns a watcher for observing changes
// to a volume attachment.
func (st *State) WatchVolumeAttachment(m names.MachineTag, v names.VolumeTag) NotifyWatcher {
	id := volumeAttachmentId(m.Id(), v.Id())
	return newEntityWatcher(st, volumeAttachmentsC, st.docID(id))
}

// WatchFilesystemAttachment returns a watcher for observing changes
// to a filesystem attachment.
func (st *State) WatchFilesystemAttachment(m names.MachineTag, f names.FilesystemTag) NotifyWatcher {
	id := filesystemAttachmentId(m.Id(), f.Id())
	return newEntityWatcher(st, filesystemAttachmentsC, st.docID(id))
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
	settingsKey := serviceSettingsKey(u.doc.Application, u.doc.CharmURL)
	return newEntityWatcher(u.st, settingsC, u.st.docID(settingsKey)), nil
}

// WatchMeterStatus returns a watcher observing changes that affect the meter status
// of a unit.
func (u *Unit) WatchMeterStatus() NotifyWatcher {
	return newDocWatcher(u.st, []docKey{
		{
			meterStatusC,
			u.st.docID(u.globalMeterStatusKey()),
		}, {
			metricsManagerC,
			metricsManagerKey,
		},
	})
}

func newEntityWatcher(st *State, collName string, key interface{}) NotifyWatcher {
	return newDocWatcher(st, []docKey{{collName, key}})
}

// docWatcher watches for changes in 1 or more mongo documents
// across collections.
type docWatcher struct {
	commonWatcher
	out chan struct{}
}

var _ Watcher = (*docWatcher)(nil)

// docKey identifies a single item in a single collection.
// It's used as a parameter to newDocWatcher to specify
// which documents should be watched.
type docKey struct {
	coll  string
	docId interface{}
}

// newDocWatcher returns a new docWatcher.
// docKeys identifies the documents that should be watched (their id and which collection they are in)
func newDocWatcher(st *State, docKeys []docKey) NotifyWatcher {
	w := &docWatcher{
		commonWatcher: newCommonWatcher(st),
		out:           make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop(docKeys))
	}()
	return w
}

// Changes returns the event channel for the docWatcher.
func (w *docWatcher) Changes() <-chan struct{} {
	return w.out
}

// getTxnRevno returns the transaction revision number of the
// given document id in the given collection. It is useful to enable
// a watcher.Watcher to be primed with the correct revision
// id.
func getTxnRevno(coll mongo.Collection, id interface{}) (int64, error) {
	doc := struct {
		TxnRevno int64 `bson:"txn-revno"`
	}{}
	fields := bson.D{{"txn-revno", 1}}
	if err := coll.FindId(id).Select(fields).One(&doc); err == mgo.ErrNotFound {
		return -1, nil
	} else if err != nil {
		return 0, err
	}
	return doc.TxnRevno, nil
}

func (w *docWatcher) loop(docKeys []docKey) error {
	in := make(chan watcher.Change)
	for _, k := range docKeys {
		coll, closer := w.st.getCollection(k.coll)
		txnRevno, err := getTxnRevno(coll, k.docId)
		closer()
		if err != nil {
			return err
		}
		w.watcher.Watch(coll.Name(), k.docId, txnRevno, in)
		defer w.watcher.Unwatch(coll.Name(), k.docId, in)
	}
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
		commonWatcher: newCommonWatcher(m.st),
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
			w.watcher.Unwatch(unitsC, w.st.docID(unitName), w.in)
			if life != Dead && !hasString(pending, unitName) {
				pending = append(pending, unitName)
			}
			for _, subunitName := range doc.Subordinates {
				if sublife, subknown := w.known[subunitName]; subknown {
					delete(w.known, subunitName)
					w.watcher.Unwatch(unitsC, w.st.docID(subunitName), w.in)
					if sublife != Dead && !hasString(pending, subunitName) {
						pending = append(pending, subunitName)
					}
				}
			}
		}
		return pending, nil
	}
	if !known {
		w.watcher.Watch(unitsC, doc.DocID, doc.TxnRevno, w.in)
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
			w.watcher.Unwatch(unitsC, w.st.docID(unit), w.in)
		}
	}()

	machines, closer := w.st.getCollection(machinesC)
	revno, err := getTxnRevno(machines, w.machine.doc.DocID)
	closer()
	if err != nil {
		return err
	}
	machineCh := make(chan watcher.Change)
	w.watcher.Watch(machinesC, w.machine.doc.DocID, revno, machineCh)
	defer w.watcher.Unwatch(machinesC, w.machine.doc.DocID, machineCh)
	changes, err := w.updateMachine([]string(nil))
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
		commonWatcher: newCommonWatcher(m.st),
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
	w.watcher.Watch(machinesC, w.machine.doc.DocID, revno, machineCh)
	defer w.watcher.Unwatch(machinesC, w.machine.doc.DocID, machineCh)
	addresses := w.machine.Addresses()
	out := w.out
	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
		commonWatcher: newCommonWatcher(st),
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
	w.watcher.WatchCollectionWithFilter(cleanupsC, in, isLocalID(w.st))
	defer w.watcher.UnwatchCollection(cleanupsC, in)

	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
		commonWatcher:  newCommonWatcher(st),
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
	w.watcher.WatchCollectionWithFilter(actionsC, w.source, isLocalID(w.st))
	defer w.watcher.UnwatchCollection(actionsC, w.source)

	changes, err := w.initial()
	if err != nil {
		return err
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case ch := <-in:
			updates, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			if err := w.filterAndMergeIds(&changes, updates); err != nil {
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
func (w *actionStatusWatcher) filterAndMergeIds(changes *[]string, updates map[interface{}]bool) error {
	watchLogger.Tracef("actionStatusWatcher filterAndMergeIds(changes:'%+v', updates:'%+v')", changes, updates)
	var adds []string
	for id, exists := range updates {
		switch id := id.(type) {
		case string:
			localId := w.st.localID(id)
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
// converts id's to their model-uuid prefixed form.
func localIdInCollectionOp(st modelBackend, localIds ...string) bson.D {
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

// collectionWatcher is a StringsWatcher that watches for changes on the
// specified collection that match a filter on the id.
type collectionWatcher struct {
	commonWatcher
	colWCfg
	source chan watcher.Change
	sink   chan []string
}

// ensure collectionWatcher is a StringsWatcher
// TODO(dfc) this needs to move to a test
var _ StringsWatcher = (*collectionWatcher)(nil)

// colWCfg contains the parameters for watching a collection.
type colWCfg struct {
	col    string
	filter func(interface{}) bool
	idconv func(string) string
}

// newcollectionWatcher starts and returns a new StringsWatcher configured
// with the given collection and filter function
func newcollectionWatcher(st *State, cfg colWCfg) StringsWatcher {
	// Always ensure that there is at least filtering on the
	// model in place.
	backstop := isLocalID(st)
	if cfg.filter == nil {
		cfg.filter = backstop
	} else {
		innerFilter := cfg.filter
		cfg.filter = func(id interface{}) bool {
			if !backstop(id) {
				return false
			}
			return innerFilter(id)
		}
	}

	w := &collectionWatcher{
		colWCfg:       cfg,
		commonWatcher: newCommonWatcher(st),
		source:        make(chan watcher.Change),
		sink:          make(chan []string),
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
func (w *collectionWatcher) Changes() <-chan []string {
	return w.sink
}

// loop performs the main event loop cycle, polling for changes and
// responding to Changes requests
func (w *collectionWatcher) loop() error {
	var (
		changes []string
		in      = (<-chan watcher.Change)(w.source)
		out     = (chan<- []string)(w.sink)
	)

	w.watcher.WatchCollectionWithFilter(w.col, w.source, w.filter)
	defer w.watcher.UnwatchCollection(w.col, w.source)

	changes, err := w.initial()
	if err != nil {
		return err
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case ch := <-in:
			updates, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			if err := w.mergeIds(&changes, updates); err != nil {
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
func (w *collectionWatcher) initial() ([]string, error) {
	var ids []string
	var doc struct {
		DocId string `bson:"_id"`
	}
	coll, closer := w.st.getCollection(w.col)
	defer closer()
	iter := coll.Find(nil).Iter()
	for iter.Next(&doc) {
		if w.filter == nil || w.filter(doc.DocId) {
			id := w.st.localID(doc.DocId)
			if w.idconv != nil {
				id = w.idconv(id)
			}
			ids = append(ids, id)
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
// Additionally, mergeIds strips the model UUID prefix from the id
// before emitting it through the watcher.
func (w *collectionWatcher) mergeIds(changes *[]string, updates map[interface{}]bool) error {
	return mergeIds(w.st, changes, updates, w.idconv)
}

func mergeIds(st modelBackend, changes *[]string, updates map[interface{}]bool, idconv func(string) string) error {
	for val, idExists := range updates {
		id, ok := val.(string)
		if !ok {
			return errors.Errorf("id is not of type string, got %T", val)
		}

		// Strip off the env UUID prefix. We only expect ids for a
		// single model.
		id, err := st.strictLocalID(id)
		if err != nil {
			return errors.Annotatef(err, "collection watcher")
		}

		if idconv != nil {
			id = idconv(id)
		}

		chIx, idAlreadyInChangeset := indexOf(id, *changes)
		if idExists {
			if !idAlreadyInChangeset {
				*changes = append(*changes, id)
			}
		} else {
			if idAlreadyInChangeset {
				// remove id from changes
				*changes = append([]string(*changes)[:chIx], []string(*changes)[chIx+1:]...)
			}
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

// watchEnqueuedActionsFilteredBy starts and returns a StringsWatcher
// that notifies on new Actions being enqueued on the ActionRecevers
// being watched.
func (st *State) watchEnqueuedActionsFilteredBy(receivers ...ActionReceiver) StringsWatcher {
	return newcollectionWatcher(st, colWCfg{
		col:    actionNotificationsC,
		filter: makeIdFilter(st, actionMarker, receivers...),
		idconv: actionNotificationIdToActionId,
	})
}

// WatchControllerStatusChanges starts and returns a StringsWatcher that
// notifies when the status of a controller machine changes.
// TODO(cherylj) Add unit tests for this, as per bug 1543408.
func (st *State) WatchControllerStatusChanges() StringsWatcher {
	return newcollectionWatcher(st, colWCfg{
		col:    statusesC,
		filter: makeControllerIdFilter(st),
	})
}

func makeControllerIdFilter(st *State) func(interface{}) bool {
	initialInfo, err := st.ControllerInfo()
	if err != nil {
		return nil
	}
	machines := initialInfo.MachineIds
	return func(key interface{}) bool {
		switch key.(type) {
		case string:
			info, err := st.ControllerInfo()
			if err != nil {
				// Most likely, things will be killed and
				// restarted if we hit this error.  Just use
				// the machine list we knew about last time.
				logger.Debugf("unable to get controller info: %v", err)
			} else {
				machines = info.MachineIds
			}
			for _, machine := range machines {
				if strings.HasSuffix(key.(string), fmt.Sprintf("m#%s", machine)) {
					return true
				}
			}
		default:
			watchLogger.Errorf("key is not type string, got %T", key)
		}
		return false
	}

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

// openedPortsWatcher notifies of changes in the openedPorts
// collection
type openedPortsWatcher struct {
	commonWatcher
	known map[string]int64
	out   chan []string
}

var _ Watcher = (*openedPortsWatcher)(nil)

// WatchOpenedPorts starts and returns a StringsWatcher notifying of changes to
// the openedPorts collection. Reported changes have the following format:
// "<machine-id>:[<subnet-CIDR>]", i.e. "0:10.20.0.0/16" or "1:" (empty subnet
// ID is allowed for backwards-compatibility).
func (st *State) WatchOpenedPorts() StringsWatcher {
	return newOpenedPortsWatcher(st)
}

func newOpenedPortsWatcher(st *State) StringsWatcher {
	w := &openedPortsWatcher{
		commonWatcher: newCommonWatcher(st),
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
// "m#42#0.1.2.0/24") into a colon-separated string with the machine and subnet
// IDs (e.g. "42:0.1.2.0/24"). Subnet ID (a.k.a. CIDR) can be empty for
// backwards-compatibility.
func (w *openedPortsWatcher) transformID(globalKey string) (string, error) {
	parts, err := extractPortsIDParts(globalKey)
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprintf("%s:%s", parts[machineIDPart], parts[subnetIDPart]), nil
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
		if changeID, err := w.transformID(id); err != nil {
			logger.Errorf(err.Error())
		} else {
			portDocs.Add(changeID)
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
	w.watcher.WatchCollectionWithFilter(openedPortsC, in, isLocalID(w.st))
	defer w.watcher.UnwatchCollection(openedPortsC, in)

	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
		if changeID, err := w.transformID(localID); err != nil {
			logger.Errorf(err.Error())
		} else {
			// Report the removed id.
			ids.Add(changeID)
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
		if changeID, err := w.transformID(localID); err != nil {
			logger.Errorf(err.Error())
		} else {
			// Report the unknown-so-far id.
			ids.Add(changeID)
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
		commonWatcher: newCommonWatcher(st),
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
	w.watcher.WatchCollectionWithFilter(rebootC, in, filter)
	defer w.watcher.UnwatchCollection(rebootC, in)
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
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
// associated with a machine.
type blockDevicesWatcher struct {
	commonWatcher
	machineId string
	out       chan struct{}
}

var _ NotifyWatcher = (*blockDevicesWatcher)(nil)

func newBlockDevicesWatcher(st *State, machineId string) NotifyWatcher {
	w := &blockDevicesWatcher{
		commonWatcher: newCommonWatcher(st),
		machineId:     machineId,
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

func (w *blockDevicesWatcher) loop() error {
	docID := w.st.docID(w.machineId)
	coll, closer := w.st.getCollection(blockDevicesC)
	revno, err := getTxnRevno(coll, docID)
	closer()
	if err != nil {
		return errors.Trace(err)
	}
	changes := make(chan watcher.Change)
	w.watcher.Watch(blockDevicesC, docID, revno, changes)
	defer w.watcher.Unwatch(blockDevicesC, docID, changes)
	blockDevices, err := getBlockDevices(w.st, w.machineId)
	if err != nil {
		return errors.Trace(err)
	}
	out := w.out
	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-changes:
			newBlockDevices, err := getBlockDevices(w.st, w.machineId)
			if err != nil {
				return errors.Trace(err)
			}
			if !reflect.DeepEqual(newBlockDevices, blockDevices) {
				blockDevices = newBlockDevices
				out = w.out
			}
		case out <- struct{}{}:
			out = nil
		}
	}
}

// WatchForModelMigration returns a notify watcher which reports when
// a migration is in progress for the model associated with the
// State.
func (st *State) WatchForModelMigration() NotifyWatcher {
	return newMigrationActiveWatcher(st)
}

type migrationActiveWatcher struct {
	commonWatcher
	collName string
	id       string
	sink     chan struct{}
}

func newMigrationActiveWatcher(st *State) NotifyWatcher {
	w := &migrationActiveWatcher{
		commonWatcher: newCommonWatcher(st),
		collName:      migrationsActiveC,
		id:            st.ModelUUID(),
		sink:          make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.sink)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for this watcher.
func (w *migrationActiveWatcher) Changes() <-chan struct{} {
	return w.sink
}

func (w *migrationActiveWatcher) loop() error {
	collection, closer := w.st.getCollection(w.collName)
	revno, err := getTxnRevno(collection, w.id)
	closer()
	if err != nil {
		return errors.Trace(err)
	}

	in := make(chan watcher.Change)
	w.watcher.Watch(w.collName, w.id, revno, in)
	defer w.watcher.Unwatch(w.collName, w.id, in)

	out := w.sink
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case change := <-in:
			if _, ok := collect(change, in, w.tomb.Dying()); !ok {
				return tomb.ErrDying
			}
			out = w.sink
		case out <- struct{}{}:
			out = nil
		}
	}
}

// WatchMigrationStatus returns a NotifyWatcher which triggers
// whenever the status of latest migration for the State's model
// changes. One instance can be used across migrations. The watcher
// will report changes when one migration finishes and another one
// begins.
//
// Note that this watcher does not produce an initial event if there's
// never been a migration attempt for the model.
func (st *State) WatchMigrationStatus() (NotifyWatcher, error) {
	return newMigrationStatusWatcher(st), nil
}

type migrationStatusWatcher struct {
	commonWatcher
	collName string
	sink     chan struct{}
}

func newMigrationStatusWatcher(st *State) NotifyWatcher {
	w := &migrationStatusWatcher{
		commonWatcher: newCommonWatcher(st),
		collName:      migrationsStatusC,
		sink:          make(chan struct{}),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.sink)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// Changes returns the event channel for this watcher.
func (w *migrationStatusWatcher) Changes() <-chan struct{} {
	return w.sink
}

func (w *migrationStatusWatcher) loop() error {
	in := make(chan watcher.Change)

	// Watch the entire migrationsStatusC collection for migration
	// status updates related to the State's model. This is more
	// efficient and simpler than tracking the current active
	// migration (and changing watchers when one migration finishes
	// and another starts.
	//
	// This approach is safe because there are strong guarantees that
	// there will only be one active migration per model. The watcher
	// will only see changes for one migration status document at a
	// time for the model.
	w.watcher.WatchCollectionWithFilter(w.collName, in, isLocalID(w.st))
	defer w.watcher.UnwatchCollection(w.collName, in)

	out := w.sink // out set so that initial event is sent.
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case change := <-in:
			if change.Revno == -1 {
				return errors.New("model migration status disappeared (shouldn't happen)")
			}
			if _, ok := collect(change, in, w.tomb.Dying()); !ok {
				return tomb.ErrDying
			}
			out = w.sink
		case out <- struct{}{}:
			out = nil
		}
	}
}
