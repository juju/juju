// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v6"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/actions"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/state/watcher"
)

var watchLogger = internallogger.GetLogger("juju.state.watch")

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

// collect combines the effects of the one change, and any further
// changes read from more in the next 10ms. The result map describes the
// existence, or not, of every id observed to have changed. If a value is read
// from the supplied stop chan, collect returns false immediately.
func collect(one watcher.Change, more <-chan watcher.Change, stop <-chan struct{}) (map[interface{}]bool, bool) {
	return collectWhereRevnoGreaterThan(one, more, stop, 0)
}

// collectWhereRevnoGreaterThan combines the effects of the one change, and any
// further changes read from more in the next 10ms. The result map describes
// the existence, or not, of every id observed to have changed. If a value is
// read from the supplied stop chan, collect returns false immediately.
//
// The implementation will flag result doc IDs as existing iff the doc revno
// is greater than the provided revnoThreshold value.
func collectWhereRevnoGreaterThan(one watcher.Change, more <-chan watcher.Change, stop <-chan struct{}, revnoThreshold int64) (map[interface{}]bool, bool) {
	var count int
	result := map[interface{}]bool{}
	handle := func(ch watcher.Change) {
		count++
		result[ch.Id] = ch.Revno > revnoThreshold
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
	watchLogger.Tracef(context.TODO(), "read %d events for %d documents", count, len(result))
	return result, true
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

// WatchModelLives returns a StringsWatcher that notifies of changes
// to any model life values. The watcher will not send any more events
// for a model after it has been observed to be Dead.
func (st *State) WatchModelLives() StringsWatcher {
	return newLifecycleWatcher(st, modelsC, nil, nil, nil)
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

// WatchModelMachineStartTimes watches the non-container machines in the model
// for changes to the Life or AgentStartTime fields and reports them as a batch
// after the specified quiesceInterval time has passed without seeing any new
// change events.
func (st *State) WatchModelMachineStartTimes(quiesceInterval time.Duration) StringsWatcher {
	return newModelMachineStartTimeWatcher(st, st.clock(), quiesceInterval)
}

type modelMachineStartTimeFieldDoc struct {
	Id             string    `bson:"_id"`
	Life           Life      `bson:"life"`
	AgentStartedAt time.Time `bson:"agent-started-at"`
}

var (
	notContainerQuery = bson.D{{"$or", []bson.D{
		{{"containertype", ""}},
		{{"containertype", bson.D{{"$exists", false}}}},
	}}}

	modelMachineStartTimeFields = bson.D{
		{"_id", 1}, {"life", 1}, {"agent-started-at", 1},
	}
)

type modelMachineStartTimeWatcher struct {
	commonWatcher
	outCh chan []string

	clk             clock.Clock
	quiesceInterval time.Duration
	seenDocs        map[string]modelMachineStartTimeFieldDoc
}

func newModelMachineStartTimeWatcher(backend modelBackend, clk clock.Clock, quiesceInterval time.Duration) StringsWatcher {
	w := &modelMachineStartTimeWatcher{
		commonWatcher:   newCommonWatcher(backend),
		outCh:           make(chan []string),
		clk:             clk,
		quiesceInterval: quiesceInterval,
		seenDocs:        make(map[string]modelMachineStartTimeFieldDoc),
	}

	w.tomb.Go(func() error {
		defer close(w.outCh)
		return w.loop()
	})
	return w
}

// Changes returns the event channel for the watcher.
func (w *modelMachineStartTimeWatcher) Changes() <-chan []string {
	return w.outCh
}

func (w *modelMachineStartTimeWatcher) loop() error {
	docWatcher := newCollectionWatcher(w.backend, colWCfg{col: machinesC})
	defer func() { _ = docWatcher.Stop() }()

	var (
		timer      = w.clk.NewTimer(w.quiesceInterval)
		timerArmed = true
		// unprocessedDocs is a list of document IDs that need to be processed
		// with a deadline they must be sent by.
		unprocessedDocs = make(map[string]time.Time)
		outCh           chan []string
		changeSet       []string
	)
	defer func() { _ = timer.Stop() }()

	// Collect and initial set of machine IDs; this makes the worker
	// compatible with other workers that expect the full state to be
	// immediately emitted once the worker starts.
	initialSet, err := w.initialMachineSet()
	if err != nil {
		return errors.Trace(err)
	}
	changeSet = initialSet.Values()
	outCh = w.outCh

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case changes := <-docWatcher.Changes():
			if len(changes) == 0 {
				continue
			}
			for _, docID := range changes {
				// filter out doc IDs that correspond to containers
				if strings.ContainsRune(docID, '/') {
					continue
				}
				id := w.backend.docID(docID)
				if _, ok := unprocessedDocs[id]; ok {
					continue
				}
				unprocessedDocs[id] = w.clk.Now().Add(w.quiesceInterval)
			}

			// Restart the timer if currently stopped.
			if !timerArmed {
				_ = timer.Reset(w.quiesceInterval)
				timerArmed = true
			}
		case <-timer.Chan():
			timerArmed = false
			if len(unprocessedDocs) == 0 {
				continue // nothing to process
			}

			visible := make(set.Strings)
			now := w.clk.Now()
			var next time.Time
			hasNext := false
			for k, due := range unprocessedDocs {
				if due.After(now) {
					if !hasNext || due.Before(next) {
						hasNext = true
						next = due
					}
					continue
				}
				delete(unprocessedDocs, k)
				visible.Add(k)
			}
			if hasNext {
				_ = timer.Reset(next.Sub(now))
				timerArmed = true
			}

			changedIDs, err := w.processChanges(visible)
			if err != nil {
				return err
			} else if changedIDs.IsEmpty() {
				continue // nothing to report
			}

			if len(changeSet) == 0 {
				changeSet = changedIDs.Values()
				outCh = w.outCh
			} else {
				// Append new set of changes to the not yet consumed changeset
				changeSet = append(changeSet, changedIDs.Values()...)
			}
		case outCh <- changeSet:
			changeSet = changeSet[:0]
			outCh = nil
		}
	}
}

func (w *modelMachineStartTimeWatcher) initialMachineSet() (set.Strings, error) {
	coll, closer := w.db.GetCollection(machinesC)
	defer closer()

	// Select the fields we need from documents that are not referring to
	// containers.
	iter := coll.Find(notContainerQuery).Select(modelMachineStartTimeFields).Iter()

	var (
		doc modelMachineStartTimeFieldDoc
		ids = make(set.Strings)
	)
	for iter.Next(&doc) {
		id := w.backend.localID(doc.Id)
		ids.Add(id)
		if doc.Life != Dead {
			w.seenDocs[id] = doc
		}
	}
	return ids, iter.Close()
}

func (w *modelMachineStartTimeWatcher) processChanges(pendingDocs set.Strings) (set.Strings, error) {
	coll, closer := w.db.GetCollection(machinesC)
	defer closer()

	// Select the fields we need from the changed documents that are not
	// referring to containers.
	iter := coll.Find(
		append(
			bson.D{{"_id", bson.D{{"$in", pendingDocs.Values()}}}},
			notContainerQuery...,
		),
	).Select(modelMachineStartTimeFields).Iter()

	var (
		doc          modelMachineStartTimeFieldDoc
		ids          = make(set.Strings)
		notFoundDocs = set.NewStrings(pendingDocs.Values()...)
	)
	for iter.Next(&doc) {
		id := w.backend.localID(doc.Id)
		old, exists := w.seenDocs[id]
		if !exists || old.Life != doc.Life || old.AgentStartedAt != doc.AgentStartedAt {
			w.seenDocs[id] = doc
			ids.Add(id)
		}

		// If the machine is now dead we won't see a change for it again
		// and therefore can permanently remove its entry from docHash
		if doc.Life == Dead {
			delete(w.seenDocs, id)
		}

		notFoundDocs.Remove(doc.Id)
	}

	// Assume that any doc in the notFound list belongs to a dead machine
	// that has been reaped from the DB.
	for docId := range notFoundDocs {
		id := w.backend.localID(docId)
		ids.Add(id)
		delete(w.seenDocs, id)
	}

	return ids, iter.Close()
}

// WatchModelMachines returns a StringsWatcher that notifies of changes to
// the lifecycles of the machines (but not containers) in the model.
func (st *State) WatchModelMachines() StringsWatcher {
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return !strings.Contains(k, "/")
	}
	return newLifecycleWatcher(st, machinesC, notContainerQuery, filter, nil)
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

// WatchControllerInfo returns a StringsWatcher for the controllers collection
func (st *State) WatchControllerInfo() StringsWatcher {
	return newCollectionWatcher(st, colWCfg{col: controllerNodesC})
}

// Watch returns a watcher for observing changes to a machine.
func (m *Machine) Watch() NotifyWatcher {
	return newEntityWatcher(m.st, machinesC, m.doc.DocID)
}

// Watch returns a watcher for observing changes to an application.
func (a *Application) Watch() NotifyWatcher {
	return newEntityWatcher(a.st, applicationsC, a.doc.DocID)
}

// Watch returns a watcher for observing changes to a unit.
func (u *Unit) Watch() NotifyWatcher {
	return newEntityWatcher(u.st, unitsC, u.doc.DocID)
}

// Watch returns a watcher for observing changes to a model.
func (m *Model) Watch() NotifyWatcher {
	return newEntityWatcher(m.st, modelsC, m.doc.UUID)
}

// WatchModelEntityReferences returns a NotifyWatcher waiting for the Model
// Entity references to change for specified model.
func (st *State) WatchModelEntityReferences(mUUID string) NotifyWatcher {
	return newEntityWatcher(st, modelEntityRefsC, mUUID)
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

func (w *docWatcher) loop(docKeys []docKey) error {
	in := make(chan watcher.Change)
	logger.Tracef(context.TODO(), "watching docs: %v", docKeys)
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
	n := 1
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
			// TODO(quiescence): reimplement quiescence
			// increment the number of notifications to send.
			n++
			out = w.out
		case out <- struct{}{}:
			n--
			if n == 0 {
				out = nil
			}
		}
	}
}

// WatchCleanups starts and returns a CleanupWatcher.
func (st *State) WatchCleanups() NotifyWatcher {
	return newNotifyCollWatcher(st, cleanupsC, isLocalID(st))
}

// WatchActionLogs starts and returns a StringsWatcher that
// notifies on new log messages for a specified action being added.
// The strings are json encoded action messages.
func (st *State) WatchActionLogs(actionId string) StringsWatcher {
	return newActionLogsWatcher(st, actionId)
}

// actionLogsWatcher reports new action progress messages.
type actionLogsWatcher struct {
	commonWatcher
	coll func() (mongo.Collection, func())
	out  chan []string

	actionId string
}

var _ Watcher = (*actionLogsWatcher)(nil)

func newActionLogsWatcher(st *State, actionId string) StringsWatcher {
	w := &actionLogsWatcher{
		commonWatcher: newCommonWatcher(st),
		coll:          collFactory(st.db(), actionsC),
		out:           make(chan []string),
		actionId:      actionId,
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns the event channel for w.
func (w *actionLogsWatcher) Changes() <-chan []string {
	return w.out
}

func (w *actionLogsWatcher) messages() ([]string, error) {
	// Get the initial logs.
	type messagesDoc struct {
		Messages []ActionMessage `bson:"messages"`
	}
	coll, closer := w.coll()
	defer closer()
	var doc messagesDoc
	err := coll.FindId(w.backend.docID(w.actionId)).Select(bson.D{{"messages", 1}}).One(&doc)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var changes []string
	for _, m := range doc.Messages {
		mjson, err := json.Marshal(actions.ActionMessage{
			Message:   m.MessageValue,
			Timestamp: m.TimestampValue.UTC(),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		changes = append(changes, string(mjson))
	}
	return changes, nil
}

func (w *actionLogsWatcher) loop() error {
	in := make(chan watcher.Change)
	filter := func(id interface{}) bool {
		k, err := w.backend.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return k == w.actionId
	}

	w.watcher.WatchCollectionWithFilter(actionsC, in, filter)
	defer w.watcher.UnwatchCollection(actionsC, in)

	changes, err := w.messages()
	if err != nil {
		return errors.Trace(err)
	}
	// Record how many messages already sent so we
	// only send new ones.
	var reportedCount int
	out := w.out

	for {
		select {
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-in:
			messages, err := w.messages()
			if err != nil {
				return errors.Trace(err)
			}
			if len(messages) > reportedCount {
				out = w.out
				changes = messages[reportedCount:]
			}
		case out <- changes:
			reportedCount += len(changes)
			out = nil
		}
	}
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

	// Only return documents with a revno greater than revnoThreshold. The
	// default zero value ensures that only modified (i.e revno > 0) rather
	// than just created documents are returned.
	revnoThreshold int64
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
			updates, ok := collectWhereRevnoGreaterThan(ch, in, w.tomb.Dying(), w.colWCfg.revnoThreshold)
			if !ok {
				return tomb.ErrDying
			}
			if err := w.mergeIds(&changes, updates); err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.sink
			} else {
				out = nil
			}
		case out <- changes:
			changes = []string{}
			out = nil
		}
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
	return mergeIds(changes, updates, w.convertId)
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

func mergeIds(changes *[]string, updates map[interface{}]bool, idconv func(string) (string, error)) error {
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
