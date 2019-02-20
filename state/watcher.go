// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/kr/pretty"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/watcher"

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
func newCommonWatcher(backend modelBackend) commonWatcher {
	return commonWatcher{
		backend: backend,
		db:      backend.db(),
		watcher: backend.txnLogWatcher(),
	}
}

// commonWatcher is part of all client watchers.
type commonWatcher struct {
	backend modelBackend
	db      Database
	watcher watcher.BaseWatcher
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
		result[ch.Id] = ch.Revno > 0
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

func collFactory(db Database, collName string) func() (mongo.Collection, func()) {
	return func() (mongo.Collection, func()) {
		return db.GetCollection(collName)
	}
}

// WatchModels returns a StringsWatcher that notifies of changes to
// any models. If a model is removed this *won't* signal that the
// model has gone away - it's based on a collectionWatcher which omits
// these events.
func (st *State) WatchModels() StringsWatcher {
	return newCollectionWatcher(st, colWCfg{
		col:    modelsC,
		global: true,
	})
}

// WatchModelLives returns a StringsWatcher that notifies of changes
// to any model life values. The watcher will not send any more events
// for a model after it has been observed to be Dead.
func (st *State) WatchModelLives() StringsWatcher {
	return newLifecycleWatcher(st, modelsC, nil, nil, nil)
}

// WatchModelVolumes returns a StringsWatcher that notifies of changes to
// the lifecycles of all model-scoped volumes.
func (sb *storageBackend) WatchModelVolumes() StringsWatcher {
	return sb.watchModelHostStorage(volumesC)
}

// WatchModelFilesystems returns a StringsWatcher that notifies of changes
// to the lifecycles of all model-scoped filesystems.
func (sb *storageBackend) WatchModelFilesystems() StringsWatcher {
	return sb.watchModelHostStorage(filesystemsC)
}

var machineOrUnitSnippet = "(" + names.NumberSnippet + "|" + names.UnitSnippet + ")"

func (sb *storageBackend) watchModelHostStorage(collection string) StringsWatcher {
	mb := sb.mb
	pattern := fmt.Sprintf("^%s$", mb.docID(machineOrUnitSnippet))
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	filter := func(id interface{}) bool {
		k, err := mb.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return !strings.Contains(k, "/")
	}
	return newLifecycleWatcher(mb, collection, members, filter, nil)
}

// WatchMachineVolumes returns a StringsWatcher that notifies of changes to
// the lifecycles of all volumes scoped to the specified machine.
func (sb *storageBackend) WatchMachineVolumes(m names.MachineTag) StringsWatcher {
	return sb.watchHostStorage(m, volumesC)
}

// WatchMachineFilesystems returns a StringsWatcher that notifies of changes
// to the lifecycles of all filesystems scoped to the specified machine.
func (sb *storageBackend) WatchMachineFilesystems(m names.MachineTag) StringsWatcher {
	return sb.watchHostStorage(m, filesystemsC)
}

// WatchUnitFilesystems returns a StringsWatcher that notifies of changes
// to the lifecycles of all filesystems scoped to units of the specified application.
func (sb *storageBackend) WatchUnitFilesystems(app names.ApplicationTag) StringsWatcher {
	return sb.watchHostStorage(app, filesystemsC)
}

func (sb *storageBackend) watchHostStorage(host names.Tag, collection string) StringsWatcher {
	mb := sb.mb
	// The regexp patterns below represent either machine or unit attached storage, <hostid>/<number>.
	// For machines, it can be something like 4/6.
	// For units the pattern becomes something like mariadb/0/6.
	// The host parameter passed into this method is the application name, any of whose units we are interested in.
	pattern := fmt.Sprintf("^%s(/%s)?/%s$", mb.docID(host.Id()), names.NumberSnippet, names.NumberSnippet)
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	prefix := fmt.Sprintf("%s(/%s)?/.*", host.Id(), names.NumberSnippet)
	matchExp := regexp.MustCompile(prefix)
	filter := func(id interface{}) bool {
		k, err := mb.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return matchExp.MatchString(k)
	}
	return newLifecycleWatcher(mb, collection, members, filter, nil)
}

// WatchMachineAttachmentsPlans returns a StringsWatcher that notifies machine agents
// that a volume has been attached to their instance by the environment provider.
// This allows machine agents to do extra initialization to the volume, in cases
// such as iSCSI disks, or other disks that have similar requirements
func (sb *storageBackend) WatchMachineAttachmentsPlans(m names.MachineTag) StringsWatcher {
	return sb.watchMachineVolumeAttachmentPlans(m)
}

func (sb *storageBackend) watchMachineVolumeAttachmentPlans(m names.MachineTag) StringsWatcher {
	mb := sb.mb
	pattern := fmt.Sprintf("^%s:%s$", mb.docID(m.Id()), names.NumberSnippet)
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	prefix := fmt.Sprintf("%s:", m.Id())
	filter := func(id interface{}) bool {
		k, err := mb.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(k, prefix)
	}
	return newLifecycleWatcher(mb, volumeAttachmentPlanC, members, filter, nil)
}

// WatchModelVolumeAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all volume attachments related to environ-
// scoped volumes.
func (sb *storageBackend) WatchModelVolumeAttachments() StringsWatcher {
	return sb.watchModelHostStorageAttachments(volumeAttachmentsC)
}

// WatchModelFilesystemAttachments returns a StringsWatcher that notifies
// of changes to the lifecycles of all filesystem attachments related to
// environ-scoped filesystems.
func (sb *storageBackend) WatchModelFilesystemAttachments() StringsWatcher {
	return sb.watchModelHostStorageAttachments(filesystemAttachmentsC)
}

func (sb *storageBackend) watchModelHostStorageAttachments(collection string) StringsWatcher {
	mb := sb.mb
	pattern := fmt.Sprintf("^%s.*:%s$", mb.docID(""), machineOrUnitSnippet)
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	filter := func(id interface{}) bool {
		k, err := mb.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		colon := strings.IndexRune(k, ':')
		if colon == -1 {
			return false
		}
		return !strings.Contains(k[colon+1:], "/")
	}
	return newLifecycleWatcher(mb, collection, members, filter, nil)
}

// WatchMachineVolumeAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all volume attachments related to the specified
// machine, for volumes scoped to the machine.
func (sb *storageBackend) WatchMachineVolumeAttachments(m names.MachineTag) StringsWatcher {
	return sb.watchHostStorageAttachments(m, volumeAttachmentsC)
}

// WatchMachineFilesystemAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all filesystem attachments related to the specified
// machine, for filesystems scoped to the machine.
func (sb *storageBackend) WatchMachineFilesystemAttachments(m names.MachineTag) StringsWatcher {
	return sb.watchHostStorageAttachments(m, filesystemAttachmentsC)
}

// WatchUnitVolumeAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all volume attachments related to the specified
// application's units, for volumes scoped to the application's units.
// TODO(caas) - currently untested since units don't directly support attached volumes
func (sb *storageBackend) WatchUnitVolumeAttachments(app names.ApplicationTag) StringsWatcher {
	return sb.watchHostStorageAttachments(app, volumeAttachmentsC)
}

// WatchUnitFilesystemAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all filesystem attachments related to the specified
// application's units, for filesystems scoped to the application's units.
func (sb *storageBackend) WatchUnitFilesystemAttachments(app names.ApplicationTag) StringsWatcher {
	return sb.watchHostStorageAttachments(app, filesystemAttachmentsC)
}

func (sb *storageBackend) watchHostStorageAttachments(host names.Tag, collection string) StringsWatcher {
	mb := sb.mb
	// Go's regex doesn't support lookbacks so the pattern match is a bit clumsy.
	// We look for either a machine attachment id, eg 0:0/42
	// or a unit attachment id, eg mariadb/0:mariadb/0/42
	// The host parameter passed into this method is the application name, any of whose units we are interested in.
	pattern := fmt.Sprintf("^%s(/%s)?:%s(/%s)?/.*", mb.docID(host.Id()), names.NumberSnippet, host.Id(), names.NumberSnippet)
	members := bson.D{{"_id", bson.D{{"$regex", pattern}}}}
	prefix := fmt.Sprintf("%s(/%s)?:%s(/%s)?/.*", host.Id(), names.NumberSnippet, host.Id(), names.NumberSnippet)
	matchExp := regexp.MustCompile(prefix)
	filter := func(id interface{}) bool {
		k, err := mb.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		matches := matchExp.FindStringSubmatch(k)
		return len(matches) == 3 && matches[1] == matches[2]
	}
	return newLifecycleWatcher(mb, collection, members, filter, nil)
}

// WatchApplications returns a StringsWatcher that notifies of changes to
// the lifecycles of the applications in the model.
func (st *State) WatchApplications() StringsWatcher {
	return newLifecycleWatcher(st, applicationsC, nil, isLocalID(st), nil)
}

// WatchRemoteApplications returns a StringsWatcher that notifies of changes to
// the lifecycles of the remote applications in the model.
func (st *State) WatchRemoteApplications() StringsWatcher {
	return newLifecycleWatcher(st, remoteApplicationsC, nil, isLocalID(st), nil)
}

// WatchStorageAttachments returns a StringsWatcher that notifies of
// changes to the lifecycles of all storage instances attached to the
// specified unit.
func (sb *storageBackend) WatchStorageAttachments(unit names.UnitTag) StringsWatcher {
	members := bson.D{{"unitid", unit.Id()}}
	prefix := unitGlobalKey(unit.Id()) + "#"
	filter := func(id interface{}) bool {
		k, err := sb.mb.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(k, prefix)
	}
	tr := func(id string) string {
		// Transform storage attachment document ID to storage ID.
		return id[len(prefix):]
	}
	return newLifecycleWatcher(sb.mb, storageAttachmentsC, members, filter, tr)
}

// WatchUnits returns a StringsWatcher that notifies of changes to the
// lifecycles of units of a.
func (a *Application) WatchUnits() StringsWatcher {
	members := bson.D{{"application", a.doc.Name}}
	prefix := a.doc.Name + "/"
	filter := func(unitDocID interface{}) bool {
		unitName, err := a.st.strictLocalID(unitDocID.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(unitName, prefix)
	}
	return newLifecycleWatcher(a.st, unitsC, members, filter, nil)
}

// WatchScale returns a new NotifyWatcher watching for
// changes to the specified application's scale value.
func (a *Application) WatchScale() NotifyWatcher {
	currentScale := -1
	filter := func(id interface{}) bool {
		k, err := a.st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		if k != a.doc.Name {
			return false
		}
		applications, closer := a.st.db().GetCollection(applicationsC)
		defer closer()

		var scaleField = bson.D{{"scale", 1}}
		var doc *applicationDoc
		if err := applications.FindId(k).Select(scaleField).One(&doc); err != nil {
			return false
		}
		match := doc.DesiredScale != currentScale
		currentScale = doc.DesiredScale
		return match
	}
	return newNotifyCollWatcher(a.st, applicationsC, filter)
}

// WatchRelations returns a StringsWatcher that notifies of changes to the
// lifecycles of relations involving a.
func (a *Application) WatchRelations() StringsWatcher {
	return watchApplicationRelations(a.st, a.doc.Name)
}

// WatchRelations returns a StringsWatcher that notifies of changes to the
// lifecycles of relations involving a.
func (s *RemoteApplication) WatchRelations() StringsWatcher {
	return watchApplicationRelations(s.st, s.doc.Name)
}

func watchApplicationRelations(backend modelBackend, applicationName string) StringsWatcher {
	prefix := applicationName + ":"
	infix := " " + prefix
	filter := func(id interface{}) bool {
		k, err := backend.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(k, prefix) || strings.Contains(k, infix)
	}
	members := bson.D{{"endpoints.applicationname", applicationName}}
	return newRelationLifeSuspendedWatcher(backend, members, filter, nil)
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
	backend modelBackend,
	collName string,
	members bson.D,
	filter func(key interface{}) bool,
	transform func(id string) string,
) StringsWatcher {
	w := &lifecycleWatcher{
		commonWatcher: newCommonWatcher(backend),
		coll:          collFactory(backend.db(), collName),
		collName:      collName,
		members:       members,
		filter:        filter,
		transform:     transform,
		life:          make(map[string]Life),
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
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
		// If no members criteria is specified, use the filter
		// to reject any unsuitable initial elements.
		if w.members == nil && w.filter != nil && !w.filter(doc.Id) {
			continue
		}
		id := w.backend.localID(doc.Id)
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
				latest[w.backend.localID(docID)] = Dead
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
		latest[w.backend.localID(doc.Id)] = doc.Life
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

// minUnitsWatcher notifies about MinUnits changes of the applications requiring
// a minimum number of units to be alive. The first event returned by the
// watcher is the set of application names requiring a minimum number of units.
// Subsequent events are generated when an application increases MinUnits, or when
// one or more units belonging to an application are destroyed.
type minUnitsWatcher struct {
	commonWatcher
	known map[string]int
	out   chan []string
}

var _ Watcher = (*minUnitsWatcher)(nil)

func newMinUnitsWatcher(backend modelBackend) StringsWatcher {
	w := &minUnitsWatcher{
		commonWatcher: newCommonWatcher(backend),
		known:         make(map[string]int),
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// WatchMinUnits returns a StringsWatcher for the minUnits collection
func (st *State) WatchMinUnits() StringsWatcher {
	return newMinUnitsWatcher(st)
}

func (w *minUnitsWatcher) initial() (set.Strings, error) {
	applicationnames := make(set.Strings)
	var doc minUnitsDoc
	newMinUnits, closer := w.db.GetCollection(minUnitsC)
	defer closer()

	iter := newMinUnits.Find(nil).Iter()
	for iter.Next(&doc) {
		w.known[doc.ApplicationName] = doc.Revno
		applicationnames.Add(doc.ApplicationName)
	}
	return applicationnames, iter.Close()
}

func (w *minUnitsWatcher) merge(applicationnames set.Strings, change watcher.Change) error {
	applicationname := w.backend.localID(change.Id.(string))
	if change.Revno < 0 {
		delete(w.known, applicationname)
		applicationnames.Remove(applicationname)
		return nil
	}
	doc := minUnitsDoc{}
	newMinUnits, closer := w.db.GetCollection(minUnitsC)
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
	w.watcher.WatchCollectionWithFilter(minUnitsC, ch, isLocalID(w.backend))
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

// WatchModelMachinesCharmProfiles returns a StringsWatcher that notifies of
// changes to the upgrade charm profile charm url for a machine.
func (st *State) WatchModelMachinesCharmProfiles() (StringsWatcher, error) {
	isMachineRegexp := fmt.Sprintf("^%s:%s#%s$", st.ModelUUID(), names.NumberSnippet, names.UnitSnippet)
	return st.watchCharmProfiles(isMachineRegexp, true)
}

// WatchContainersCharmProfiles starts a StringsWatcher to notify when
// the provisioner should update the charm profiles used by any container on
// the machine.
func (m *Machine) WatchContainersCharmProfiles(ctype instance.ContainerType) (StringsWatcher, error) {
	isChildRegexp := fmt.Sprintf("^%s/%s/%s#%s$", m.doc.DocID, ctype, names.NumberSnippet, names.UnitSnippet)
	return m.st.watchCharmProfiles(isChildRegexp, true)
}

// WatchModelMachinesCharmProfiles returns a StringsWatcher that notifies of
// changes to the upgrade charm profile charm url for a machine.
//
// TODO - is this needed?  2019-02-01
// the provisioner calling this lives on the controller
func (st *State) WatchModelMachinesCharmProfilesNewFormat() (StringsWatcher, error) {
	isMachineRegexp := fmt.Sprintf("^%s:%s#%s$", st.ModelUUID(), names.NumberSnippet, names.UnitSnippet)
	return st.watchCharmProfiles(isMachineRegexp, false)
}

// WatchContainersCharmProfiles starts a StringsWatcher to notify when
// the provisioner should update the charm profiles used by any container on
// the machine.
func (m *Machine) WatchContainersCharmProfilesNewFormat(ctype instance.ContainerType) (StringsWatcher, error) {
	isChildRegexp := fmt.Sprintf("^%s/%s/%s#%s$", m.doc.DocID, ctype, names.NumberSnippet, names.UnitSnippet)
	return m.st.watchCharmProfiles(isChildRegexp, false)
}

func (st *State) watchCharmProfiles(regExp string, useMachineId bool) (StringsWatcher, error) {
	members := bson.D{{"_id", bson.D{{"$regex", regExp}}}}
	compiled, err := regexp.Compile(regExp)
	if err != nil {
		return nil, errors.Trace(err)
	}

	filter := func(key interface{}) bool {
		k, ok := key.(string)
		if !ok {
			return false
		}
		_, err := st.strictLocalID(k)
		if err != nil {
			return false
		}
		return compiled.MatchString(k)
	}
	var accessor profileAccessorFunc
	if useMachineId {
		accessor = func(doc instanceCharmProfileData) (string, string) {
			return doc.UpgradeCharmProfileCharmURL, doc.MachineId
		}
	} else {
		accessor = func(doc instanceCharmProfileData) (string, string) {
			return doc.UpgradeCharmProfileCharmURL, st.localID(doc.DocID)
		}
	}
	completed := func(doc instanceCharmProfileData) bool {
		return lxdprofile.UpgradeStatusTerminal(doc.UpgradeCharmProfileComplete)
	}
	return newModelFieldChangeWatcher(st, members, filter, accessor, completed), nil
}

type profileAccessorFunc func(doc instanceCharmProfileData) (string, string)

// modelFieldChangeWatcher notifies about charm changes where a
// machine or container's field may need to be changed. At startup, the
// watcher gathers current values for a machine's field, no events are returned.
// Events are generated when there are changes to a machine or container's
// field.
type modelFieldChangeWatcher struct {
	commonWatcher
	// members is used to select the initial set of interesting entities.
	members bson.D
	// filter returns true, if the entity should be watched
	filter func(key interface{}) bool
	// accessor is used to extract the field from the instance charm profile
	// data doc in a generic way.  The second value returned, is what to pass
	// as the string from the watcher.
	accessor profileAccessorFunc
	// completed is used to determine if the state watched for has
	// occurred
	completed func(instanceCharmProfileData) bool
	known     map[string]string
	out       chan []string
}

var _ Watcher = (*modelFieldChangeWatcher)(nil)

func newModelFieldChangeWatcher(
	backend modelBackend,
	members bson.D,
	filter func(key interface{}) bool,
	accessor profileAccessorFunc,
	completed func(instanceCharmProfileData) bool,
) StringsWatcher {
	w := &modelFieldChangeWatcher{
		commonWatcher: newCommonWatcher(backend),
		members:       members,
		filter:        filter,
		accessor:      accessor,
		completed:     completed,
		known:         make(map[string]string),
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

func (w *modelFieldChangeWatcher) initial() (set.Strings, error) {
	collection, closer := w.db.GetCollection(instanceCharmProfileDataC)
	defer closer()

	var doc instanceCharmProfileData
	watchSet := make(set.Strings)
	iter := collection.Find(w.members).Iter()
	for iter.Next(&doc) {
		// If no members criteria is specified, use the filter
		// to reject any unsuitable initial elements.
		if w.members == nil && w.filter != nil && !w.filter(doc.DocID) {
			continue
		}

		if w.completed(doc) {
			logger.Tracef("field change NOT watching %s", doc.DocID)
			continue
		}

		docField, returnID := w.accessor(doc)
		docId := w.backend.localID(doc.DocID)
		w.known[docId] = docField
		watchSet.Add(returnID)
	}
	if watchSet.Size() > 0 {
		logger.Debugf("started field change watching %s", watchSet.Values())
	}
	return watchSet, iter.Close()
}

func (w *modelFieldChangeWatcher) merge(watchSet set.Strings, change watcher.Change) error {
	docId := w.backend.localID(change.Id.(string))
	if change.Revno == -1 {
		if _, ok := w.known[docId]; ok {
			logger.Tracef("stopped field change watching for %q", docId)
		}
		delete(w.known, docId)
		// TODO - check this correct.
		watchSet.Remove(docId)
		return nil
	}

	collection, closer := w.db.GetCollection(instanceCharmProfileDataC)
	defer closer()

	var doc instanceCharmProfileData
	if err := collection.FindId(change.Id).One(&doc); err != nil {
		return err
	}

	// get the document field from the accessor
	docField, returnField := w.accessor(doc)

	// check the field before adding to the docId
	field, isKnown := w.known[docId]
	w.known[docId] = docField
	if !w.completed(doc) && (!isKnown || docField != field) {
		logger.Debugf("added field change watching for %q", docId)
		watchSet.Add(returnField)
	}
	return nil
}

func (w *modelFieldChangeWatcher) loop() error {
	ch := make(chan watcher.Change)
	w.watcher.WatchCollectionWithFilter(instanceCharmProfileDataC, ch, w.filter)
	defer w.watcher.UnwatchCollection(instanceCharmProfileDataC, ch)

	watchSet, err := w.initial()
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
		case change := <-ch:
			if err = w.merge(watchSet, change); err != nil {
				return err
			}
			if !watchSet.IsEmpty() {
				out = w.out
			}
		case out <- watchSet.Values():
			out = nil
			watchSet = set.NewStrings()
		}
	}
}

func (w *modelFieldChangeWatcher) Changes() <-chan []string {
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

func newRelationScopeWatcher(backend modelBackend, scope, ignore string) *RelationScopeWatcher {
	w := &RelationScopeWatcher{
		commonWatcher: newCommonWatcher(backend),
		prefix:        scope + "#",
		ignore:        ignore,
		out:           make(chan *RelationScopeChange),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
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
	relationScopes, closer := w.db.GetCollection(relationScopesC)
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
	logger.Tracef("relationScopeWatcher prefix %q initializing with %# v",
		w.prefix, pretty.Formatter(info))
	return info, nil
}

// mergeChanges updates info with the contents of the changes in ids. False
// values are always treated as removed; true values cause the associated
// document to be read, and whether it's treated as added or removed depends
// on the value of the document's Departing field.
func (w *RelationScopeWatcher) mergeChanges(info *scopeInfo, ids map[interface{}]bool) error {
	relationScopes, closer := w.db.GetCollection(relationScopesC)
	defer closer()

	var existIds []string
	for id, exists := range ids {
		switch id := id.(type) {
		case string:
			if exists {
				existIds = append(existIds, id)
			} else {
				key, err := w.backend.strictLocalID(id)
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
	logger.Tracef("RelationScopeWatcher prefix %q merge scope to %# v from ids: %# v",
		w.prefix, pretty.Formatter(info), pretty.Formatter(ids))
	return nil
}

func (w *RelationScopeWatcher) loop() error {
	in := make(chan watcher.Change)
	fullPrefix := w.backend.docID(w.prefix)
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
	return newRelationUnitsWatcher(ru.st, ru.WatchScope())
}

// WatchUnits returns a watcher that notifies of changes to the units of the
// specified application endpoint in the relation. This method will return an error
// if the endpoint is not globally scoped.
func (r *Relation) WatchUnits(appName string) (RelationUnitsWatcher, error) {
	return r.watchUnits(appName, false)
}

func (r *Relation) watchUnits(applicationName string, counterpart bool) (RelationUnitsWatcher, error) {
	ep, err := r.Endpoint(applicationName)
	if err != nil {
		return nil, err
	}
	if ep.Scope != charm.ScopeGlobal {
		return nil, errors.Errorf("%q endpoint is not globally scoped", ep.Name)
	}
	role := ep.Role
	if counterpart {
		role = counterpartRole(role)
	}
	rsw := watchRelationScope(r.st, r.globalScope(), role, "")
	return newRelationUnitsWatcher(r.st, rsw), nil
}

func newRelationUnitsWatcher(backend modelBackend, sw *RelationScopeWatcher) RelationUnitsWatcher {
	w := &relationUnitsWatcher{
		commonWatcher: newCommonWatcher(backend),
		sw:            sw,
		watching:      make(set.Strings),
		updates:       make(chan watcher.Change),
		out:           make(chan params.RelationUnitsChange),
	}
	w.tomb.Go(func() error {
		defer w.finish()
		return w.loop()
	})
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
func (w *relationUnitsWatcher) mergeSettings(changes *params.RelationUnitsChange, key string) error {
	var doc struct {
		TxnRevno int64 `bson:"txn-revno"`
		Version  int64 `bson:"version"`
	}
	if err := readSettingsDocInto(w.backend.db(), settingsC, key, &doc); err != nil {
		logger.Tracef("relationUnitsWatcher %q merging key %q (not found)", w.sw.prefix, key)
		return err
	}
	logger.Tracef("relationUnitsWatcher %q merging key %q revno: %d version: %d", w.sw.prefix, key, doc.TxnRevno, doc.Version)
	setRelationUnitChangeVersion(changes, key, doc.Version)
	return nil
}

// mergeScope starts and stops settings watches on the units entering and
// leaving the scope in the supplied RelationScopeChange event, and applies
// the expressed changes to the supplied RelationUnitsChange event.
func (w *relationUnitsWatcher) mergeScope(changes *params.RelationUnitsChange, c *RelationScopeChange) error {
	docIds := make([]interface{}, len(c.Entered))
	for i, name := range c.Entered {
		key := w.sw.prefix + name
		docID := w.backend.docID(key)
		docIds[i] = docID
	}
	logger.Tracef("relationUnitsWatcher %q watching newly entered: %v, and unwatching left %v", w.sw.prefix, c.Entered, c.Left)
	if err := w.watcher.WatchMulti(settingsC, docIds, w.updates); err != nil {
		return errors.Trace(err)
	}
	for _, docID := range docIds {
		w.watching.Add(docID.(string))
	}
	for _, name := range c.Entered {
		key := w.sw.prefix + name
		if err := w.mergeSettings(changes, key); err != nil {
			return errors.Annotatef(err, "while merging settings for %q entering relation scope", name)
		}
		changes.Departed = remove(changes.Departed, name)
	}
	for _, name := range c.Left {
		key := w.sw.prefix + name
		docID := w.backend.docID(key)
		changes.Departed = append(changes.Departed, name)
		if changes.Changed != nil {
			delete(changes.Changed, name)
		}
		w.watcher.Unwatch(settingsC, docID, w.updates)
		w.watching.Remove(docID)
	}
	logger.Tracef("relationUnitsWatcher %q Change updated to: %# v", w.sw.prefix, changes)
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
	// w.tomb.Done()
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
			if err := w.mergeSettings(&changes, id); err != nil {
				return errors.Annotatef(err, "relation scope id %q", id)
			}
			out = w.out
		case out <- changes:
			logger.Tracef("relationUnitsWatcher %q sent changes %# v", w.sw.prefix, pretty.Formatter(changes))
			sentInitial = true
			changes = params.RelationUnitsChange{}
			out = nil
		}
	}
}

// WatchLifeSuspendedStatus returns a watcher that notifies of changes to the life
// or suspended status of the relation.
func (r *Relation) WatchLifeSuspendedStatus() StringsWatcher {
	filter := func(id interface{}) bool {
		k, err := r.st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return k == r.Tag().Id()
	}
	members := bson.D{{"id", r.Id()}}
	return newRelationLifeSuspendedWatcher(r.st, members, filter, nil)
}

type relationLifeSuspended struct {
	life      Life
	suspended bool
}

// relationLifeSuspendedWatcher sends notifications of changes to the life or
// suspended status of specific relations.
type relationLifeSuspendedWatcher struct {
	commonWatcher
	out           chan []string
	lifeSuspended map[string]relationLifeSuspended

	members   bson.D
	filter    func(interface{}) bool
	transform func(id string) string
}

// newRelationLifeSuspendedWatcher creates a watcher that sends changes when the
// life or suspended status of specific relations change.
func newRelationLifeSuspendedWatcher(
	backend modelBackend,
	members bson.D,
	filter func(key interface{}) bool,
	transform func(id string) string,
) *relationLifeSuspendedWatcher {
	w := &relationLifeSuspendedWatcher{
		commonWatcher: newCommonWatcher(backend),
		out:           make(chan []string),
		members:       members,
		filter:        filter,
		transform:     transform,
		lifeSuspended: make(map[string]relationLifeSuspended),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

func (w *relationLifeSuspendedWatcher) Changes() <-chan []string {
	return w.out
}

type relationLifeSuspendedDoc struct {
	DocId     string `bson:"_id"`
	Life      Life   `bson:"life"`
	Suspended bool   `bson:"suspended"`
}

var relationLifeSuspendedFields = bson.D{{"_id", 1}, {"life", 1}, {"suspended", 1}}

func (w *relationLifeSuspendedWatcher) initial() (set.Strings, error) {
	coll, closer := w.db.GetCollection(relationsC)
	defer closer()

	ids := make(set.Strings)
	var doc relationLifeSuspendedDoc
	iter := coll.Find(w.members).Select(relationLifeSuspendedFields).Iter()
	for iter.Next(&doc) {
		// If no members criteria is specified, use the filter
		// to reject any unsuitable initial elements.
		if w.members == nil && w.filter != nil && !w.filter(doc.DocId) {
			continue
		}
		id := w.backend.localID(doc.DocId)
		ids.Add(id)
		if doc.Life != Dead {
			w.lifeSuspended[id] = relationLifeSuspended{life: doc.Life, suspended: doc.Suspended}
		}
	}
	return ids, iter.Close()
}

func (w *relationLifeSuspendedWatcher) merge(ids set.Strings, updates map[interface{}]bool) error {
	coll, closer := w.db.GetCollection(relationsC)
	defer closer()

	// Separate ids into those thought to exist and those known to be removed.
	var changed []string
	latest := make(map[string]relationLifeSuspended)
	for docID, exists := range updates {
		switch docID := docID.(type) {
		case string:
			if exists {
				changed = append(changed, docID)
			} else {
				latest[w.backend.localID(docID)] = relationLifeSuspended{life: Dead}
			}
		default:
			return errors.Errorf("id is not of type string, got %T", docID)
		}
	}

	// Collect life states from ids thought to exist. Any that don't actually
	// exist are ignored (we'll hear about them in the next set of updates --
	// all that's actually happened in that situation is that the watcher
	// events have lagged a little behind reality).
	iter := coll.Find(bson.D{{"_id", bson.D{{"$in", changed}}}}).Select(relationLifeSuspendedFields).Iter()
	var doc relationLifeSuspendedDoc
	for iter.Next(&doc) {
		latest[w.backend.localID(doc.DocId)] = relationLifeSuspended{life: doc.Life, suspended: doc.Suspended}
	}
	if err := iter.Close(); err != nil {
		return err
	}

	// Add to ids any whose life state is known to have changed.
	for id, newLifeSuspended := range latest {
		gone := newLifeSuspended.life == Dead
		oldLifeSuspended, known := w.lifeSuspended[id]
		switch {
		case known && gone:
			delete(w.lifeSuspended, id)
		case !known && !gone:
			w.lifeSuspended[id] = newLifeSuspended
		case known &&
			(newLifeSuspended.life != oldLifeSuspended.life || newLifeSuspended.suspended != oldLifeSuspended.suspended):
			w.lifeSuspended[id] = newLifeSuspended
		default:
			continue
		}
		ids.Add(id)
	}
	return nil
}

func (w *relationLifeSuspendedWatcher) loop() error {
	in := make(chan watcher.Change)
	w.watcher.WatchCollectionWithFilter(relationsC, in, w.filter)
	defer w.watcher.UnwatchCollection(relationsC, in)
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

func newUnitsWatcher(backend modelBackend, tag names.Tag, getUnits func() ([]string, error), coll, id string) StringsWatcher {
	w := &unitsWatcher{
		commonWatcher: newCommonWatcher(backend),
		tag:           tag.String(),
		getUnits:      getUnits,
		life:          map[string]Life{},
		in:            make(chan watcher.Change),
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop(coll, id)
	})
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
	return w.watchUnits(initialNames, nil)
}

func (w *unitsWatcher) watchUnits(names, changes []string) ([]string, error) {
	docs := []lifeWatchDoc{}
	ids := make([]interface{}, len(names))
	for i := range names {
		ids[i] = w.backend.docID(names[i])
	}
	if err := w.watcher.WatchMulti(unitsC, ids, w.in); err != nil {
		logger.Tracef("error watching %q in %q: %v", ids, unitsC, err)
		return nil, errors.Trace(err)
	}
	logger.Tracef("watching %q ids: %q", unitsC, ids)
	newUnits, closer := w.db.GetCollection(unitsC)
	err := newUnits.Find(bson.M{"_id": bson.M{"$in": names}}).Select(lifeWatchFields).All(&docs)
	closer()
	if err != nil {
		return nil, errors.Trace(err)
	}

	found := set.NewStrings()
	for _, doc := range docs {
		localId, err := w.backend.strictLocalID(doc.Id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		found.Add(localId)
		if !hasString(changes, localId) {
			logger.Tracef("marking change for %q", localId)
			changes = append(changes, localId)
		}
		if doc.Life != Dead {
			logger.Tracef("setting life of %q to %q", localId, doc.Life)
			w.life[localId] = doc.Life
		} else {
			// Note(jam): 2019-01-31 This was done to match existing behavior, it is not guaranteed
			// to be the behavior we want. Specifically, if we see a Dead unit we will report that
			// it exists in the initial event. However, we stop watching because
			// the object is dead, so you don't get an event when the doc is
			// removed from the database. It seems better if we either/
			// a) don't tell you about Dead documents
			// b) give you an event if a Dead document goes away.
			logger.Tracef("unwatching Dead unit: %q", localId)
			w.watcher.Unwatch(unitsC, doc.Id, w.in)
			delete(w.life, localId)
		}
	}
	// See if there are any entries that we wanted to watch but are actually gone
	for _, name := range names {
		if !found.Contains(name) {
			logger.Tracef("looking for unit %q, found it gone, Unwatching", name)
			if _, ok := w.life[name]; ok {
				// we see this doc, but it doesn't exist
				if !hasString(changes, name) {
					changes = append(changes, name)
				}
				delete(w.life, name)
			}
			w.watcher.Unwatch(unitsC, w.backend.docID(name), w.in)
		}
	}
	logger.Tracef("changes: %q", changes)
	return changes, nil
}

// update adds to and returns changes, such that it contains the names of any
// non-Dead units to have entered or left the tracked set.
func (w *unitsWatcher) update(changes []string) ([]string, error) {
	latest, err := w.getUnits()
	if err != nil {
		return nil, err
	}
	var unknown []string
	for _, name := range latest {
		if _, found := w.life[name]; !found {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		changes, err = w.watchUnits(unknown, changes)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	for name := range w.life {
		if hasString(latest, name) {
			continue
		}
		if !hasString(changes, name) {
			changes = append(changes, name)
		}
		logger.Tracef("unit %q %q no longer in latest, removing watch", unitsC, name)
		delete(w.life, name)
		w.watcher.Unwatch(unitsC, w.backend.docID(name), w.in)
	}
	logger.Tracef("update reports changes: %q", changes)
	return changes, nil
}

// merge adds to and returns changes, such that it contains the supplied unit
// name if that unit is unknown and non-Dead, or has changed lifecycle status.
func (w *unitsWatcher) merge(changes []string, name string) ([]string, error) {
	logger.Tracef("merging change for %q %q", unitsC, name)
	var doc lifeWatchDoc
	units, closer := w.db.GetCollection(unitsC)
	err := units.FindId(name).Select(lifeWatchFields).One(&doc)
	closer()
	gone := false
	if err == mgo.ErrNotFound {
		gone = true
	} else if err != nil {
		return nil, err
	} else if doc.Life == Dead {
		gone = true
	}
	life := w.life[name]
	switch {
	case gone:
		delete(w.life, name)
		logger.Tracef("document gone, unwatching %q %q", unitsC, name)
		w.watcher.Unwatch(unitsC, w.backend.docID(name), w.in)
	case life != doc.Life:
		logger.Tracef("updating doc life %q %q to %q", unitsC, name, doc.Life)
		w.life[name] = doc.Life
	default:
		return changes, nil
	}
	if !hasString(changes, name) {
		changes = append(changes, name)
	}
	logger.Tracef("merge reporting changes: %q", changes)
	return changes, nil
}

func (w *unitsWatcher) loop(coll, id string) error {
	logger.Tracef("watching root channel %q %q", coll, id)
	rootCh := make(chan watcher.Change)
	w.watcher.Watch(coll, id, rootCh)
	defer func() {
		w.watcher.Unwatch(coll, id, rootCh)
		for name := range w.life {
			w.watcher.Unwatch(unitsC, w.backend.docID(name), w.in)
		}
	}()
	changes, err := w.initial()
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
		case <-rootCh:
			changes, err = w.update(changes)
			if len(changes) > 0 {
				out = w.out
			}
		case c := <-w.in:
			localID := w.backend.localID(c.Id.(string))
			changes, err = w.merge(changes, localID)
			if err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			logger.Tracef("watcher reported changes: %q", changes)
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

// WatchControllerConfig returns a NotifyWatcher for controller settings.
func (st *State) WatchControllerConfig() NotifyWatcher {
	return newEntityWatcher(st, controllersC, controllerSettingsGlobalKey)
}

// Watch returns a watcher for observing changes to a machine.
func (m *Machine) Watch() NotifyWatcher {
	return newEntityWatcher(m.st, machinesC, m.doc.DocID)
}

// Watch returns a watcher for observing changes to an application.
func (a *Application) Watch() NotifyWatcher {
	return newEntityWatcher(a.st, applicationsC, a.doc.DocID)
}

// WatchLeaderSettings returns a watcher for observing changed to an application's
// leader settings.
func (a *Application) WatchLeaderSettings() NotifyWatcher {
	docId := a.st.docID(leadershipSettingsKey(a.Name()))
	return newEntityWatcher(a.st, settingsC, docId)
}

// Watch returns a watcher for observing changes to a unit.
func (u *Unit) Watch() NotifyWatcher {
	return newEntityWatcher(u.st, unitsC, u.doc.DocID)
}

// Watch returns a watcher for observing changes to a model.
func (m *Model) Watch() NotifyWatcher {
	return newEntityWatcher(m.st, modelsC, m.doc.UUID)
}

// WatchInstanceData returns a watcher for observing changes to a model.
func (m *Machine) WatchInstanceData() NotifyWatcher {
	return newEntityWatcher(m.st, instanceDataC, m.doc.Id)
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
func (model *Model) WatchForModelConfigChanges() NotifyWatcher {
	return newEntityWatcher(model.st, settingsC, model.st.docID(modelGlobalKey))
}

// WatchModelEntityReferences returns a NotifyWatcher waiting for the Model
// Entity references to change for specified model.
func (st *State) WatchModelEntityReferences(mUUID string) NotifyWatcher {
	return newEntityWatcher(st, modelEntityRefsC, mUUID)
}

// WatchForUnitAssignment watches for new applications that request units to be
// assigned to machines.
func (st *State) WatchForUnitAssignment() StringsWatcher {
	return newCollectionWatcher(st, colWCfg{col: assignUnitC})
}

// WatchAPIHostPortsForClients returns a NotifyWatcher that notifies
// when the set of API addresses changes.
func (st *State) WatchAPIHostPortsForClients() NotifyWatcher {
	return newEntityWatcher(st, controllersC, apiHostPortsKey)
}

// WatchAPIHostPortsForAgents returns a NotifyWatcher that notifies
// when the set of API addresses usable by agents changes.
func (st *State) WatchAPIHostPortsForAgents() NotifyWatcher {
	return newEntityWatcher(st, controllersC, apiHostPortsForAgentsKey)
}

// WatchStorageAttachment returns a watcher for observing changes
// to a storage attachment.
func (sb *storageBackend) WatchStorageAttachment(s names.StorageTag, u names.UnitTag) NotifyWatcher {
	id := storageAttachmentId(u.Id(), s.Id())
	return newEntityWatcher(sb.mb, storageAttachmentsC, sb.mb.docID(id))
}

// WatchVolumeAttachment returns a watcher for observing changes
// to a volume attachment.
func (sb *storageBackend) WatchVolumeAttachment(host names.Tag, v names.VolumeTag) NotifyWatcher {
	id := volumeAttachmentId(host.Id(), v.Id())
	return newEntityWatcher(sb.mb, volumeAttachmentsC, sb.mb.docID(id))
}

// WatchFilesystemAttachment returns a watcher for observing changes
// to a filesystem attachment.
func (sb *storageBackend) WatchFilesystemAttachment(host names.Tag, f names.FilesystemTag) NotifyWatcher {
	id := filesystemAttachmentId(host.Id(), f.Id())
	return newEntityWatcher(sb.mb, filesystemAttachmentsC, sb.mb.docID(id))
}

// WatchCharmConfig returns a watcher for observing changes to the
// application's charm configuration settings. The returned watcher will be
// valid only while the application's charm URL is not changed.
func (a *Application) WatchCharmConfig() (NotifyWatcher, error) {
	configKey := a.charmConfigKey()
	return newEntityWatcher(a.st, settingsC, a.st.docID(configKey)), nil
}

// WatchConfigSettings returns a watcher for observing changes to the
// unit's application configuration settings. The unit must have a charm URL
// set before this method is called, and the returned watcher will be
// valid only while the unit's charm URL is not changed.
// TODO(fwereade): this could be much smarter; if it were, uniter.Filter
// could be somewhat simpler.
func (u *Unit) WatchConfigSettings() (NotifyWatcher, error) {
	if u.doc.CharmURL == nil {
		return nil, fmt.Errorf("unit charm not set")
	}
	charmConfigKey := applicationCharmConfigKey(u.doc.Application, u.doc.CharmURL)
	return newEntityWatcher(u.st, settingsC, u.st.docID(charmConfigKey)), nil
}

// WatchApplicationConfigSettings is the same as WatchConfigSettings but
// notifies on changes to application configuration not charm configuration.
func (u *Unit) WatchApplicationConfigSettings() (NotifyWatcher, error) {
	applicationConfigKey := applicationConfigKey(u.ApplicationName())
	return newEntityWatcher(u.st, settingsC, u.st.docID(applicationConfigKey)), nil
}

// WatchConfigSettingsHash returns a watcher that yields a hash of the
// unit's charm config settings whenever they are changed. The
// returned watcher will be valid only while the application's charm
// URL is not changed.
func (u *Unit) WatchConfigSettingsHash() (StringsWatcher, error) {
	if u.doc.CharmURL == nil {
		return nil, fmt.Errorf("unit charm not set")
	}
	charmConfigKey := applicationCharmConfigKey(u.doc.Application, u.doc.CharmURL)
	return newSettingsHashWatcher(u.st, charmConfigKey), nil
}

// WatchApplicationConfigSettingsHash is the same as
// WatchConfigSettingsHash but watches the application's config rather
// than charm configuration. Yields a hash of the application config
// with each change.
func (u *Unit) WatchApplicationConfigSettingsHash() (StringsWatcher, error) {
	applicationConfigKey := applicationConfigKey(u.ApplicationName())
	return newSettingsHashWatcher(u.st, applicationConfigKey), nil
}

// WatchMeterStatus returns a watcher observing changes that affect the meter status
// of a unit.
func (u *Unit) WatchMeterStatus() NotifyWatcher {
	return newDocWatcher(u.st, []docKey{
		{
			meterStatusC,
			u.st.docID(u.globalMeterStatusKey()),
		}, {
			meterStatusC,
			metricsManagerKey(u.st),
		},
	})
}

// WatchLXDProfileUpgradeNotifications returns a watcher that observes the status
// of a lxd profile upgrade by monitoring changes on the unit machine's lxd profile
// upgrade completed field that is specific to the unit.
func (m *Machine) WatchLXDProfileUpgradeNotifications(unitName string) (StringsWatcher, error) {
	filter := func(id interface{}) bool {
		docId, err := m.st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return docId == m.instanceCharmProfileDataId(unitName)
	}
	docId := m.instanceCharmProfileDataId(unitName)
	return newInstanceCharmProfileDataWatcher(m.st, docId, filter), nil
}

// WatchLXDProfileUpgradeNotifications returns a watcher that observes the status
// of a lxd profile upgrade by monitoring changes on the unit machine's lxd profile
// upgrade completed field that is specific to itself.
func (u *Unit) WatchLXDProfileUpgradeNotifications() (StringsWatcher, error) {
	m, err := u.machine()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m.WatchLXDProfileUpgradeNotifications(u.Name())
}

// instanceCharmProfileDataWatcher notifies about any changes to the
// instanceCharmProfileData document. The watcher looks for changes to the
// upgrading of a charm lxd profile, that belongs to an application, which the
// provisioner updates the document field. At start up the watcher gathers the
// current values of the instance charm profile data, if the document doesn't
// exist, then the status is set to not known. The document are transient and
// not expected to there all the time, so the code deals with that with the
// usage of the not know status.
// Events are generated when there are changes to a instance charm profile
// data document.
type instanceCharmProfileDataWatcher struct {
	commonWatcher
	// docId is used to select the initial interesting entities.
	docId  string
	known  string
	filter func(interface{}) bool
	out    chan []string
}

var _ Watcher = (*instanceCharmProfileDataWatcher)(nil)

func newInstanceCharmProfileDataWatcher(backend modelBackend, memberId string, filter func(interface{}) bool) StringsWatcher {
	w := &instanceCharmProfileDataWatcher{
		commonWatcher: newCommonWatcher(backend),
		docId:         memberId,
		filter:        filter,
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

func (w *instanceCharmProfileDataWatcher) initial() error {
	instanceDataCol, instanceDataCloser := w.db.GetCollection(instanceCharmProfileDataC)
	defer instanceDataCloser()

	statusField := lxdprofile.NotKnownStatus

	var instanceData instanceCharmProfileData
	if err := instanceDataCol.Find(bson.D{
		{"_id", w.docId},
	}).One(&instanceData); err == nil {
		statusField = instanceData.UpgradeCharmProfileComplete
	}
	w.known = statusField

	logger.Tracef("Started watching instanceCharmProfileData for %q: %q", w.docId, statusField)
	return nil
}

func (w *instanceCharmProfileDataWatcher) merge(change watcher.Change) (bool, error) {
	machineId := change.Id.(string)
	if change.Revno < 0 {
		return false, nil
	}
	instanceDataCol, instanceCloser := w.db.GetCollection(instanceCharmProfileDataC)
	defer instanceCloser()

	var instanceData instanceCharmProfileData
	if err := instanceDataCol.Find(bson.D{
		{"_id", w.docId},
	}).One(&instanceData); err != nil {
		if err != mgo.ErrNotFound {
			logger.Debugf("instanceCharmProfileData NOT mgo err not found")
			return false, err
		}
		logger.Tracef("instanceCharmProfileData for %q: mgo err not found", machineId)
		return false, nil
	}

	// check the field before adding to the machineId
	currentField := instanceData.UpgradeCharmProfileComplete
	if w.known != currentField {
		w.known = currentField

		logger.Tracef("Changes in watching instanceCharmProfileData for %q: %q", w.docId, currentField)
		return true, nil
	}
	return false, nil
}

func (w *instanceCharmProfileDataWatcher) loop() error {
	err := w.initial()
	if err != nil {
		return err
	}

	ch := make(chan watcher.Change)
	w.watcher.WatchCollectionWithFilter(instanceCharmProfileDataC, ch, w.filter)
	defer w.watcher.UnwatchCollection(instanceCharmProfileDataC, ch)

	out := w.out
	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case change := <-ch:
			isChanged, err := w.merge(change)
			if err != nil {
				return err
			}
			if isChanged {
				out = w.out
			}
		case out <- []string{w.known}:
			out = nil
		}
	}
}

func (w *instanceCharmProfileDataWatcher) Changes() <-chan []string {
	return w.out
}

// WatchUpgradeSeriesNotifications returns a watcher that observes the status of
// a series upgrade by monitoring changes to its parent machine's upgrade series
// lock.
func (m *Machine) WatchUpgradeSeriesNotifications() (NotifyWatcher, error) {
	watch := newEntityWatcher(m.st, machineUpgradeSeriesLocksC, m.doc.DocID)
	if _, ok := <-watch.Changes(); ok {
		return watch, nil
	}

	return nil, watcher.EnsureErr(watch)
}

func newEntityWatcher(backend modelBackend, collName string, key interface{}) NotifyWatcher {
	return newDocWatcher(backend, []docKey{{collName, key}})
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
func newDocWatcher(backend modelBackend, docKeys []docKey) NotifyWatcher {
	w := &docWatcher{
		commonWatcher: newCommonWatcher(backend),
		out:           make(chan struct{}),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop(docKeys)
	})
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
	logger.Tracef("watching docs: %v", docKeys)
	for _, k := range docKeys {
		w.watcher.Watch(k.coll, k.docId, in)
		defer w.watcher.Unwatch(k.coll, k.docId, in)
	}
	// Check to see if there is a backing event that should be coalesced with the
	// first event
	if _, ok := collect(watcher.Change{}, in, w.tomb.Dying()); !ok {
		return tomb.ErrDying
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
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
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
	var unknown []string
	for _, unitName := range w.machine.doc.Principals {
		if _, ok := w.known[unitName]; !ok {
			unknown = append(unknown, unitName)
		}
	}
	if len(unknown) > 0 {
		pending, err = w.watchNewUnits(unknown, pending, nil)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return pending, nil
}

// watchNewUnits sets up a watcher for all of the named units and then updates pending changes.
// There is an assumption that all unitNames being passed are unknown and do not have a watch active for them.
func (w *machineUnitsWatcher) watchNewUnits(unitNames, pending []string, unitColl mongo.Collection) ([]string, error) {
	if len(unitNames) == 0 {
		return pending, nil
	}
	ids := make([]interface{}, len(unitNames))
	for i := range unitNames {
		ids[i] = w.backend.docID(unitNames[i])
	}
	logger.Tracef("for machine %q watching new units %q", w.machine.doc.DocID, unitNames)
	err := w.watcher.WatchMulti(unitsC, ids, w.in)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if unitColl == nil {
		var closer SessionCloser
		unitColl, closer = w.db.GetCollection(unitsC)
		defer closer()
	}
	var doc unitDoc
	iter := unitColl.Find(bson.M{"_id": bson.M{"$in": unitNames}}).Iter()
	unknownSubs := set.NewStrings()
	notfound := set.NewStrings(unitNames...)
	for iter.Next(&doc) {
		notfound.Remove(doc.Name)
		w.known[doc.Name] = doc.Life
		pending = append(pending, doc.Name)
		// now load subordinates
		for _, subunitName := range doc.Subordinates {
			if _, subknown := w.known[subunitName]; !subknown {
				unknownSubs.Add(subunitName)
			}
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	for name := range notfound {
		logger.Debugf("unit %q referenced but not found", name)
		w.watcher.Unwatch(unitsC, w.backend.docID(name), w.in)
	}
	if !unknownSubs.IsEmpty() {
		pending, err = w.watchNewUnits(unknownSubs.Values(), pending, unitColl)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return pending, nil
}

// removeWatchedUnit stops watching the unit, and all subordinates for this unit
func (w *machineUnitsWatcher) removeWatchedUnit(unitName string, doc unitDoc, pending []string) ([]string, error) {
	logger.Tracef("machineUnitsWatcher removing unit %q for life %q", doc.Name, doc.Life)
	life, known := w.known[unitName]
	// Unit was removed or unassigned from w.machine
	if known {
		delete(w.known, unitName)
		docID := w.backend.docID(unitName)
		w.watcher.Unwatch(unitsC, docID, w.in)
		if life != Dead && !hasString(pending, unitName) {
			pending = append(pending, unitName)
		}
		for _, subunitName := range doc.Subordinates {
			if sublife, subknown := w.known[subunitName]; subknown {
				delete(w.known, subunitName)
				w.watcher.Unwatch(unitsC, w.backend.docID(subunitName), w.in)
				if sublife != Dead && !hasString(pending, subunitName) {
					pending = append(pending, subunitName)
				}
			}
		}
	}
	return pending, nil
}

// merge checks if this unitName has been modified and if so, updates pending accordingly.
// merge() should only be called for documents that are already being watched and part of known
// use watchNewUnits if you have a new object
func (w *machineUnitsWatcher) merge(pending []string, unitName string) (new []string, err error) {
	doc := unitDoc{}
	newUnits, closer := w.db.GetCollection(unitsC)
	defer closer()
	err = newUnits.FindId(unitName).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}
	if err == mgo.ErrNotFound || doc.Principal == "" && (doc.MachineId == "" || doc.MachineId != w.machine.doc.Id) {
		// We always pass the unitName because the document may be deleted, and thus not have a name on the object
		pending, err := w.removeWatchedUnit(unitName, doc, pending)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return pending, nil
	}
	life, known := w.known[unitName]
	if !known {
		return nil, errors.Errorf("merge() called with an unknown document: %q", doc.DocID)
	}
	if life != doc.Life && !hasString(pending, doc.Name) {
		logger.Tracef("machineUnitsWatcher found life changed to %q => %q for %q", life, doc.Life, doc.Name)
		pending = append(pending, doc.Name)
	}
	w.known[doc.Name] = doc.Life
	unknownSubordinates := set.NewStrings()
	for _, subunitName := range doc.Subordinates {
		if _, ok := w.known[subunitName]; !ok {
			unknownSubordinates.Add(subunitName)
		}
	}
	if !unknownSubordinates.IsEmpty() {
		pending, err = w.watchNewUnits(unknownSubordinates.Values(), pending, nil)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return pending, nil
}

func (w *machineUnitsWatcher) loop() error {
	defer func() {
		for unit := range w.known {
			w.watcher.Unwatch(unitsC, w.backend.docID(unit), w.in)
		}
	}()

	machineCh := make(chan watcher.Change)
	w.watcher.Watch(machinesC, w.machine.doc.DocID, machineCh)
	defer w.watcher.Unwatch(machinesC, w.machine.doc.DocID, machineCh)
	changes, err := w.updateMachine(nil)
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
		case <-machineCh:
			changes, err = w.updateMachine(changes)
			if err != nil {
				return errors.Trace(err)
			}
			if len(changes) > 0 {
				out = w.out
			}
		case c := <-w.in:
			changes, err = w.merge(changes, w.backend.localID(c.Id.(string)))
			if err != nil {
				return errors.Trace(err)
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
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns the event channel for w.
func (w *machineAddressesWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *machineAddressesWatcher) loop() error {
	machineCh := make(chan watcher.Change)
	w.watcher.Watch(machinesC, w.machine.doc.DocID, machineCh)
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

// WatchCleanups starts and returns a CleanupWatcher.
func (st *State) WatchCleanups() NotifyWatcher {
	return newNotifyCollWatcher(st, cleanupsC, isLocalID(st))
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
func newActionStatusWatcher(backend modelBackend, receivers []ActionReceiver, statusSet ...ActionStatus) StringsWatcher {
	watchLogger.Debugf("newActionStatusWatcher receivers:'%+v', statuses'%+v'", receivers, statusSet)
	w := &actionStatusWatcher{
		commonWatcher:  newCommonWatcher(backend),
		source:         make(chan watcher.Change),
		sink:           make(chan []string),
		receiverFilter: actionReceiverInCollectionOp(receivers...),
		statusFilter:   statusInCollectionOp(statusSet...),
	}

	w.tomb.Go(func() error {
		defer close(w.sink)
		return w.loop()
	})

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
	w.watcher.WatchCollectionWithFilter(actionsC, w.source, isLocalID(w.backend))
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

	coll, closer := w.db.GetCollection(actionsC)
	defer closer()

	idFilter := localIdInCollectionOp(w.backend, ids...)
	query := bson.D{{"$and", []bson.D{idFilter, w.receiverFilter, w.statusFilter}}}
	iter := coll.Find(query).Iter()
	var found []string
	var doc actionDoc
	for iter.Next(&doc) {
		found = append(found, w.backend.localID(doc.DocId))
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
			localId := w.backend.localID(id)
			chIx, idAlreadyInChangeset := indexOf(localId, *changes)
			if exists {
				if !idAlreadyInChangeset {
					adds = append(adds, localId)
				}
			} else {
				if idAlreadyInChangeset {
					// remove id from changes
					*changes = append((*changes)[:chIx], (*changes)[chIx+1:]...)
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

// colWCfg contains the parameters for watching a collection.
type colWCfg struct {
	col    string
	filter func(interface{}) bool
	idconv func(string) string

	// If global is true the watcher won't be limited to this model.
	global bool
}

// newCollectionWatcher starts and returns a new StringsWatcher configured
// with the given collection and filter function
func newCollectionWatcher(backend modelBackend, cfg colWCfg) StringsWatcher {
	if cfg.global {
		if cfg.filter == nil {
			cfg.filter = func(x interface{}) bool {
				return true
			}
		}
	} else {
		// Always ensure that there is at least filtering on the
		// model in place.
		backstop := isLocalID(backend)
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
	}

	w := &collectionWatcher{
		colWCfg:       cfg,
		commonWatcher: newCommonWatcher(backend),
		source:        make(chan watcher.Change),
		sink:          make(chan []string),
	}

	w.tomb.Go(func() error {
		defer close(w.sink)
		defer close(w.source)
		return w.loop()
	})

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
func makeIdFilter(backend modelBackend, marker string, receivers ...ActionReceiver) func(interface{}) bool {
	if len(receivers) == 0 {
		return nil
	}
	ensureMarkerFn := ensureSuffixFn(marker)
	prefixes := make([]string, len(receivers))
	for ix, receiver := range receivers {
		prefixes[ix] = backend.docID(ensureMarkerFn(receiver.Tag().Id()))
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
	coll, closer := w.db.GetCollection(w.col)
	defer closer()
	iter := coll.Find(nil).Iter()
	for iter.Next(&doc) {
		if w.filter == nil || w.filter(doc.DocId) {
			id := doc.DocId
			if !w.colWCfg.global {
				id = w.backend.localID(id)
			}
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
	return mergeIds(w.backend, changes, updates, w.convertId)
}

func (w *collectionWatcher) convertId(id string) (string, error) {
	if !w.colWCfg.global {
		// Strip off the env UUID prefix.
		// We only expect ids for a single model.
		var err error
		id, err = w.backend.strictLocalID(id)
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	if w.idconv != nil {
		id = w.idconv(id)
	}
	return id, nil
}

func mergeIds(st modelBackend, changes *[]string, updates map[interface{}]bool, idconv func(string) (string, error)) error {
	for val, idExists := range updates {
		id, ok := val.(string)
		if !ok {
			return errors.Errorf("id is not of type string, got %T", val)
		}

		id, err := idconv(id)
		if err != nil {
			return errors.Annotatef(err, "collection watcher")
		}

		chIx, idAlreadyInChangeset := indexOf(id, *changes)
		if idExists {
			if !idAlreadyInChangeset {
				*changes = append(*changes, id)
			}
		} else {
			if idAlreadyInChangeset {
				// remove id from changes
				*changes = append((*changes)[:chIx], (*changes)[chIx+1:]...)
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
	return newCollectionWatcher(st, colWCfg{
		col:    actionNotificationsC,
		filter: makeIdFilter(st, actionMarker, receivers...),
		idconv: actionNotificationIdToActionId,
	})
}

// WatchControllerStatusChanges starts and returns a StringsWatcher that
// notifies when the status of a controller machine changes.
// TODO(cherylj) Add unit tests for this, as per bug 1543408.
func (st *State) WatchControllerStatusChanges() StringsWatcher {
	return newCollectionWatcher(st, colWCfg{
		col:    statusesC,
		filter: makeControllerIdFilter(st),
	})
}

func makeControllerIdFilter(st *State) func(interface{}) bool {
	initialInfo, err := st.ControllerInfo()
	if err != nil {
		logger.Debugf("unable to get controller info: %v", err)
		return nil
	}

	filter := controllerIdFilter{
		st:           st,
		lastMachines: initialInfo.MachineIds,
	}
	return filter.match
}

// controllerIdFilter is a stateful watcher filter function - if it
// can't get the current machines from controller info it uses the
// last machines retrieved. Since this is called from multiple
// goroutines getting/updating lastMachines is protected by a mutex.
type controllerIdFilter struct {
	mu           sync.Mutex
	st           *State
	lastMachines []string
}

func (f *controllerIdFilter) machines() []string {
	var result []string
	info, err := f.st.ControllerInfo()
	f.mu.Lock()
	if err != nil {
		// Most likely, things will be killed and
		// restarted if we hit this error.  Just use
		// the machine list we knew about last time.
		logger.Debugf("unable to get controller info: %v", err)
		result = f.lastMachines
	} else {
		f.lastMachines = info.MachineIds
		result = info.MachineIds
	}
	f.mu.Unlock()
	return result
}

func (f *controllerIdFilter) match(key interface{}) bool {
	switch key.(type) {
	case string:
		machines := f.machines()
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

// WatchActionResults starts and returns a StringsWatcher that
// notifies on new ActionResults being added.
func (m *Model) WatchActionResults() StringsWatcher {
	return m.WatchActionResultsFilteredBy()
}

// WatchActionResultsFilteredBy starts and returns a StringsWatcher
// that notifies on new ActionResults being added for the ActionRecevers
// being watched.
func (m *Model) WatchActionResultsFilteredBy(receivers ...ActionReceiver) StringsWatcher {
	return newActionStatusWatcher(m.st, receivers, []ActionStatus{ActionCompleted, ActionCancelled, ActionFailed}...)
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

func newOpenedPortsWatcher(backend modelBackend) StringsWatcher {
	w := &openedPortsWatcher{
		commonWatcher: newCommonWatcher(backend),
		known:         make(map[string]int64),
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})

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
	ports, closer := w.db.GetCollection(openedPortsC)
	defer closer()

	portDocs := set.NewStrings()
	var doc portsDoc
	iter := ports.Find(nil).Select(bson.D{{"_id", 1}, {"txn-revno", 1}}).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		id, err := w.backend.strictLocalID(doc.DocID)
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
	w.watcher.WatchCollectionWithFilter(openedPortsC, in, isLocalID(w.backend))
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
	localID, err := w.backend.strictLocalID(id)
	if err != nil {
		return errors.Trace(err)
	}
	if change.Revno < 0 {
		delete(w.known, localID)
		if changeID, err := w.transformID(localID); err != nil {
			logger.Errorf(err.Error())
		} else {
			// Report the removed id.
			ids.Add(changeID)
		}
		return nil
	}
	openedPorts, closer := w.db.GetCollection(openedPortsC)
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
func (m *Machine) WatchForRebootEvent() NotifyWatcher {
	machineIds := m.machinesToCareAboutRebootsFor()
	machines := set.NewStrings(machineIds...)

	filter := func(key interface{}) bool {
		if id, ok := key.(string); ok {
			if id, err := m.st.strictLocalID(id); err == nil {
				return machines.Contains(id)
			} else {
				return false
			}
		}
		return false
	}
	return newNotifyCollWatcher(m.st, rebootC, filter)
}

// blockDevicesWatcher notifies about changes to all block devices
// associated with a machine.
type blockDevicesWatcher struct {
	commonWatcher
	machineId string
	out       chan struct{}
}

var _ NotifyWatcher = (*blockDevicesWatcher)(nil)

func newBlockDevicesWatcher(backend modelBackend, machineId string) NotifyWatcher {
	w := &blockDevicesWatcher{
		commonWatcher: newCommonWatcher(backend),
		machineId:     machineId,
		out:           make(chan struct{}),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns the event channel for w.
func (w *blockDevicesWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *blockDevicesWatcher) loop() error {
	docID := w.backend.docID(w.machineId)
	changes := make(chan watcher.Change)
	w.watcher.Watch(blockDevicesC, docID, changes)
	defer w.watcher.Unwatch(blockDevicesC, docID, changes)
	blockDevices, err := getBlockDevices(w.db, w.machineId)
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
			newBlockDevices, err := getBlockDevices(w.db, w.machineId)
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

// WatchForMigration returns a notify watcher which reports when
// a migration is in progress for the model associated with the
// State.
func (st *State) WatchForMigration() NotifyWatcher {
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
	w.tomb.Go(func() error {
		defer close(w.sink)
		return w.loop()
	})
	return w
}

// Changes returns the event channel for this watcher.
func (w *migrationActiveWatcher) Changes() <-chan struct{} {
	return w.sink
}

func (w *migrationActiveWatcher) loop() error {
	in := make(chan watcher.Change)
	w.watcher.Watch(w.collName, w.id, in)
	defer w.watcher.Unwatch(w.collName, w.id, in)

	// check if there are any pending changes before the first event
	if _, ok := collect(watcher.Change{}, in, w.tomb.Dying()); !ok {
		return tomb.ErrDying
	}
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
func (st *State) WatchMigrationStatus() NotifyWatcher {
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
	return newNotifyCollWatcher(st, migrationsStatusC, isLocalID(st))
}

// WatchMachineRemovals returns a NotifyWatcher which triggers
// whenever machine removal records are added or removed.
func (st *State) WatchMachineRemovals() NotifyWatcher {
	return newNotifyCollWatcher(st, machineRemovalsC, isLocalID(st))
}

// notifyCollWatcher implements NotifyWatcher, triggering when a
// change is seen in a specific collection matching the provided
// filter function.
type notifyCollWatcher struct {
	commonWatcher
	collName string
	filter   func(interface{}) bool
	sink     chan struct{}
}

func newNotifyCollWatcher(backend modelBackend, collName string, filter func(interface{}) bool) NotifyWatcher {
	w := &notifyCollWatcher{
		commonWatcher: newCommonWatcher(backend),
		collName:      collName,
		filter:        filter,
		sink:          make(chan struct{}),
	}
	w.tomb.Go(func() error {
		defer close(w.sink)
		return w.loop()
	})
	return w
}

// Changes returns the event channel for this watcher.
func (w *notifyCollWatcher) Changes() <-chan struct{} {
	return w.sink
}

func (w *notifyCollWatcher) loop() error {
	in := make(chan watcher.Change)

	w.watcher.WatchCollectionWithFilter(w.collName, in, w.filter)
	defer w.watcher.UnwatchCollection(w.collName, in)

	// check if there are any pending changes before the first event
	if _, ok := collect(watcher.Change{}, in, w.tomb.Dying()); !ok {
		return tomb.ErrDying
	}
	out := w.sink // out set so that initial event is sent.
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

// WatchRemoteRelations returns a StringsWatcher that notifies of changes to
// the lifecycles of the remote relations in the model.
func (st *State) WatchRemoteRelations() StringsWatcher {
	// Use a no-op transform func to record the known ids.
	known := make(map[interface{}]bool)
	tr := func(id string) string {
		known[id] = true
		return id
	}

	filter := func(id interface{}) bool {
		id, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}

		// Gather the remote app names.
		remoteApps, closer := st.db().GetCollection(remoteApplicationsC)
		defer closer()

		type remoteAppDoc struct {
			Name string
		}
		remoteAppNameField := bson.D{{"name", 1}}
		var apps []remoteAppDoc
		err = remoteApps.Find(nil).Select(remoteAppNameField).All(&apps)
		if err != nil {
			watchLogger.Errorf("could not lookup remote application names: %v", err)
			return false
		}
		remoteAppNames := set.NewStrings()
		for _, a := range apps {
			remoteAppNames.Add(a.Name)
		}

		// Run a query to pickup any relations to those remote apps.
		relations, closer := st.db().GetCollection(relationsC)
		defer closer()

		query := bson.D{
			{"key", id},
			{"endpoints.applicationname", bson.D{{"$in", remoteAppNames.Values()}}},
		}
		num, err := relations.Find(query).Count()
		if err != nil {
			watchLogger.Errorf("could not lookup remote relations: %v", err)
			return false
		}
		// The relation (or remote app) may have been deleted, but if it has been
		// seen previously, return true.
		if num == 0 {
			_, seen := known[id]
			delete(known, id)
			return seen
		}
		return num > 0
	}
	return newRelationLifeSuspendedWatcher(st, nil, filter, tr)
}

// WatchSubnets returns a StringsWatcher that notifies of changes to
// the lifecycles of the subnets in the model.
func (st *State) WatchSubnets(subnetFilter func(id interface{}) bool) StringsWatcher {
	filter := func(id interface{}) bool {
		subnet, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		if subnetFilter == nil {
			return true
		}
		return subnetFilter(subnet)
	}

	return newLifecycleWatcher(st, subnetsC, nil, filter, nil)
}

// isLocalID returns a watcher filter func that rejects ids not specific
// to the supplied modelBackend.
func isLocalID(st modelBackend) func(interface{}) bool {
	return func(id interface{}) bool {
		key, ok := id.(string)
		if !ok {
			return false
		}
		_, err := st.strictLocalID(key)
		return err == nil
	}
}

// relationNetworksWatcher notifies of changes in the
// relationNetworks collection, either ingress of egress.
type relationNetworksWatcher struct {
	commonWatcher
	key         string
	direction   string
	filter      func(key interface{}) bool
	knownTxnRev int64
	knownCidrs  set.Strings
	out         chan []string
}

var _ Watcher = (*relationNetworksWatcher)(nil)

// WatchRelationIngressNetworks starts and returns a StringsWatcher notifying
// of ingress changes to the relationNetworks collection for the relation.
func (r *Relation) WatchRelationIngressNetworks() StringsWatcher {
	return newrelationNetworksWatcher(r.st, r.Tag().Id(), ingress)
}

// WatchRelationEgressNetworks starts and returns a StringsWatcher notifying
// of egress changes to the relationNetworks collection for the relation.
func (r *Relation) WatchRelationEgressNetworks() StringsWatcher {
	return newrelationNetworksWatcher(r.st, r.Tag().Id(), egress)
}

func newrelationNetworksWatcher(st modelBackend, relationKey, direction string) StringsWatcher {
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return strings.HasPrefix(k, relationKey+":"+direction+":")
	}
	w := &relationNetworksWatcher{
		commonWatcher: newCommonWatcher(st),
		key:           relationKey,
		direction:     direction,
		filter:        filter,
		knownCidrs:    set.NewStrings(),
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})

	return w
}

// Changes returns the event channel for watching changes
// to a relation's ingress networks.
func (w *relationNetworksWatcher) Changes() <-chan []string {
	return w.out
}

func (w *relationNetworksWatcher) loadCIDRs() (bool, error) {
	coll, closer := w.db.GetCollection(relationNetworksC)
	defer closer()

	var doc struct {
		TxnRevno int64    `bson:"txn-revno"`
		Id       string   `bson:"_id"`
		CIDRs    []string `bson:"cidrs"`
	}
	err := coll.FindId(relationNetworkDocID(w.key, w.direction, relationNetworkAdmin)).One(&doc)
	if err == mgo.ErrNotFound {
		err = coll.FindId(relationNetworkDocID(w.key, w.direction, relationNetworkDefault)).One(&doc)
	}
	if err == mgo.ErrNotFound {
		// Record deleted.
		changed := w.knownCidrs.Size() > 0
		w.knownCidrs = set.NewStrings()
		return changed, nil
	}
	if err != nil {
		return false, errors.Trace(err)

	}
	cidrs := w.knownCidrs
	if doc.TxnRevno == -1 {
		// Record deleted.
		cidrs = set.NewStrings()
	}
	if doc.TxnRevno > w.knownTxnRev {
		cidrs = set.NewStrings(doc.CIDRs...)
	}
	w.knownTxnRev = doc.TxnRevno
	changed := !cidrs.Difference(w.knownCidrs).IsEmpty() || !w.knownCidrs.Difference(cidrs).IsEmpty()
	w.knownCidrs = cidrs
	return changed, nil
}

func (w *relationNetworksWatcher) loop() error {
	in := make(chan watcher.Change)
	w.watcher.WatchCollectionWithFilter(relationNetworksC, in, w.filter)
	defer w.watcher.UnwatchCollection(relationNetworksC, in)

	var (
		sentInitial bool
		changed     bool
		out         chan<- []string
		err         error
	)
	if _, err = w.loadCIDRs(); err != nil {
		return errors.Trace(err)
	}
	for {
		if !sentInitial || changed {
			changed = false
			out = w.out
		}
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case _, ok := <-in:
			if !ok {
				return tomb.ErrDying
			}
			if changed, err = w.loadCIDRs(); err != nil {
				return errors.Trace(err)
			}
		case out <- w.knownCidrs.Values():
			out = nil
			sentInitial = true
		}
		if w.knownTxnRev == -1 {
			// Record deleted
			return tomb.ErrDying
		}
	}
}

// externalControllersWatcher notifies about addition and removal of
// external controller references.
type externalControllersWatcher struct {
	commonWatcher
	coll func() (mongo.Collection, func())
	out  chan []string
}

var _ Watcher = (*externalControllersWatcher)(nil)

func newExternalControllersWatcher(st *State) StringsWatcher {
	w := &externalControllersWatcher{
		commonWatcher: newCommonWatcher(st),
		coll:          collFactory(st.db(), externalControllersC),
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns the event channel for w.
func (w *externalControllersWatcher) Changes() <-chan []string {
	return w.out
}

func (w *externalControllersWatcher) initial() (set.Strings, error) {
	// Get the initial documents in the collection.
	type idDoc struct {
		Id string `bson:"_id"`
	}
	coll, closer := w.coll()
	defer closer()
	changes := make(set.Strings)
	iter := coll.Find(nil).Select(bson.D{{"_id", 1}}).Iter()
	var doc idDoc
	for iter.Next(&doc) {
		changes.Add(doc.Id)
	}
	return changes, iter.Close()
}

func (w *externalControllersWatcher) loop() error {
	in := make(chan watcher.Change)
	w.watcher.WatchCollection(externalControllersC, in)
	defer w.watcher.UnwatchCollection(externalControllersC, in)

	changes, err := w.initial()
	if err != nil {
		return errors.Trace(err)
	}

	reported := make(set.Strings)
	out := w.out
	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case ch := <-in:
			updates, ok := collect(ch, in, w.tomb.Dying())
			if !ok {
				return tomb.ErrDying
			}
			for id, updated := range updates {
				id := id.(string)
				if updated != reported.Contains(id) {
					// updated and hasn't been reported, or
					// removed and has been reported
					changes.Add(id)
				}
			}
			if changes.Size() > 0 {
				out = w.out
			}
		case out <- changes.SortedValues():
			out = nil
			for _, id := range changes.Values() {
				if reported.Contains(id) {
					reported.Remove(id)
				} else {
					reported.Add(id)
				}
			}
			changes = make(set.Strings)
		}
	}
}

// WatchPodSpec returns a watcher observing changes that affect the
// pod spec for an application or unit.
func (m *CAASModel) WatchPodSpec(appTag names.ApplicationTag) (NotifyWatcher, error) {
	docKeys := []docKey{{
		podSpecsC,
		m.st.docID(applicationGlobalKey(appTag.Id())),
	}}
	return newDocWatcher(m.st, docKeys), nil
}

// containerAddressesWatcher notifies about changes to a unit's pod address(es).
type containerAddressesWatcher struct {
	commonWatcher
	unit *Unit
	out  chan struct{}
}

var _ Watcher = (*containerAddressesWatcher)(nil)

// WatchContainerAddresses returns a new NotifyWatcher watching the unit's pod address(es).
func (u *Unit) WatchContainerAddresses() NotifyWatcher {
	return newContainerAddressesWatcher(u)
}

func newContainerAddressesWatcher(u *Unit) NotifyWatcher {
	w := &containerAddressesWatcher{
		commonWatcher: newCommonWatcher(u.st),
		out:           make(chan struct{}),
		unit:          u,
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns the event channel for w.
func (w *containerAddressesWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *containerAddressesWatcher) loop() error {
	id := w.backend.docID(w.unit.globalKey())
	containerCh := make(chan watcher.Change)
	w.watcher.Watch(cloudContainersC, id, containerCh)
	defer w.watcher.Unwatch(cloudContainersC, id, containerCh)

	var currentAddress *address
	container, err := w.unit.cloudContainer()
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if err == nil {
		currentAddress = container.Address
	}
	out := w.out
	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-containerCh:
			container, err := w.unit.cloudContainer()
			if err != nil {
				return err
			}
			addressValue := func(addr *address) string {
				if addr == nil {
					return ""
				}
				return addr.Value
			}
			newAddress := container.Address
			if addressValue(newAddress) != addressValue(currentAddress) {
				currentAddress = newAddress
				out = w.out
			}
		case out <- struct{}{}:
			out = nil
		}
	}
}

func newSettingsHashWatcher(st *State, localID string) StringsWatcher {
	docID := st.docID(localID)
	w := &hashWatcher{
		commonWatcher: newCommonWatcher(st),
		out:           make(chan []string),
		collection:    settingsC,
		id:            docID,
		hash: func() (string, error) {
			return hashSettings(st.db(), docID, localID)
		},
	}
	w.start()
	return w
}

func hashSettings(db Database, id string, name string) (string, error) {
	settings, closer := db.GetCollection(settingsC)
	defer closer()
	var doc settingsDoc
	if err := settings.FindId(id).One(&doc); err == mgo.ErrNotFound {
		return "", nil
	} else if err != nil {
		return "", errors.Trace(err)
	}
	// Ensure elements are in a consistent order. If any are maps,
	// replace them with the equivalent sorted bson.Ds.
	items := toSortedBsonD(doc.Settings)
	data, err := bson.Marshal(items)
	if err != nil {
		return "", errors.Trace(err)
	}
	hash := sha256.New()
	hash.Write([]byte(name))
	hash.Write(data)
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// WatchAddressesHash returns a StringsWatcher that emits the hash of
// the machine's (sorted) addresses whenever they change.
func (m *Machine) WatchAddressesHash() StringsWatcher {
	mCopy := &Machine{
		st:  m.st,
		doc: m.doc,
	}
	w := &hashWatcher{
		commonWatcher: newCommonWatcher(m.st),
		out:           make(chan []string),
		collection:    machinesC,
		id:            m.doc.DocID,
		hash: func() (string, error) {
			return hashMachineAddresses(mCopy)
		},
	}
	w.start()
	return w
}

func hashMachineAddresses(m *Machine) (string, error) {
	if err := m.Refresh(); err != nil {
		return "", errors.Trace(err)
	}
	addresses := m.Addresses()
	sort.Slice(addresses, func(i, j int) bool {
		// Addresses guarantees that each value will only be
		// returned once - addresses from provider take
		// precedence over those from the machine.
		return addresses[i].Value < addresses[j].Value
	})
	hash := sha256.New()
	for _, address := range addresses {
		hash.Write([]byte(address.Value))
		hash.Write([]byte(address.Type))
		hash.Write([]byte(address.Scope))
		hash.Write([]byte(address.SpaceName))
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// WatchContainerAddressesHash returns a StringsWatcher that emits a
// hash of the unit's container address whenever it changes.
func (u *Unit) WatchContainerAddressesHash() StringsWatcher {
	firstCall := true
	w := &hashWatcher{
		commonWatcher: newCommonWatcher(u.st),
		out:           make(chan []string),
		collection:    cloudContainersC,
		id:            u.st.docID(u.globalKey()),
		hash: func() (string, error) {
			result, err := hashContainerAddresses(u, firstCall)
			firstCall = false
			return result, err
		},
	}
	w.start()
	return w
}

func hashContainerAddresses(u *Unit, firstCall bool) (string, error) {
	container, err := u.cloudContainer()
	if errors.IsNotFound(err) && firstCall {
		// To keep behaviour the same as
		// WatchContainerAddresses, we need to ignore NotFound
		// errors on the first call but propagate them after
		// that.
		return "", nil
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	address := container.Address
	if address == nil {
		return "", nil
	}
	hash := sha256.New()
	hash.Write([]byte(address.Value))
	hash.Write([]byte(address.AddressType))
	hash.Write([]byte(address.Scope))
	hash.Write([]byte(address.Origin))
	hash.Write([]byte(address.SpaceName))
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

type hashWatcher struct {
	commonWatcher
	collection string
	id         string
	hash       func() (string, error)
	out        chan []string
}

func (w *hashWatcher) Changes() <-chan []string {
	return w.out
}

func (w *hashWatcher) start() {
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
}

func (w *hashWatcher) loop() error {
	changesCh := make(chan watcher.Change)
	w.watcher.Watch(w.collection, w.id, changesCh)
	defer w.watcher.Unwatch(w.collection, w.id, changesCh)

	lastHash, err := w.hash()
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
		case change := <-changesCh:
			if _, ok := collect(change, changesCh, w.tomb.Dying()); !ok {
				return tomb.ErrDying
			}
			newHash, err := w.hash()
			if err != nil {
				return errors.Trace(err)
			}
			if lastHash != newHash {
				lastHash = newHash
				out = w.out
			}
		case out <- []string{lastHash}:
			out = nil
		}
	}
}

func toSortedBsonD(values map[string]interface{}) bson.D {
	var items bson.D
	for name, value := range values {
		if mapValue, ok := value.(map[string]interface{}); ok {
			value = toSortedBsonD(mapValue)
		}
		items = append(items, bson.DocElem{Name: name, Value: value})
	}
	// We know that there aren't any equal names because the source is
	// a map.
	sort.Slice(items, func(i int, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}
