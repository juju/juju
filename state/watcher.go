// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v6"
	"github.com/kr/pretty"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	corenetwork "github.com/juju/juju/core/network"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/charm"
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

// RelationUnitsWatcher generates signals when units enter or leave
// the scope of a RelationUnit, and changes to the settings of those
// units known to have entered.
type RelationUnitsWatcher interface {
	Watcher

	Changes() corewatcher.RelationUnitsChannel
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

// WatchApplicationCharms notifies when application charm URLs change.
// TODO(wallyworld) - use a filter to only trigger on charm URL changes.
func (st *State) WatchApplicationCharms() StringsWatcher {
	return newCollectionWatcher(st, colWCfg{col: applicationsC})
}

// WatchUnits notifies when units change.
func (st *State) WatchUnits() StringsWatcher {
	return newCollectionWatcher(st, colWCfg{col: unitsC})
}

// WatchMachines notifies when machines change.
func (st *State) WatchMachines() StringsWatcher {
	return newLifecycleWatcher(st, machinesC, nil, isLocalID(st), nil)
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

// WatchRelations returns a StringsWatcher that notifies of changes to the
// lifecycles of relations involving a.
func (a *Application) WatchRelations() StringsWatcher {
	return watchApplicationRelations(a.st, a.doc.Name)
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
	logger.Tracef(context.TODO(), "relationScopeWatcher prefix %q initializing with %# v",
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
			logger.Warningf(context.TODO(), "ignoring bad relation scope id: %#v", id)
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
	logger.Tracef(context.TODO(), "RelationScopeWatcher prefix %q merge scope to %# v from ids: %# v",
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
	sw              *RelationScopeWatcher
	watching        set.Strings
	updates         chan watcher.Change
	appSettingsKeys []string
	appUpdates      chan watcher.Change
	out             chan corewatcher.RelationUnitsChange
	logger          corelogger.Logger
}

// Watch returns a watcher that notifies of changes to counterpart units in
// the relation.
func (ru *RelationUnit) Watch() RelationUnitsWatcher {
	// TODO(jam): 2019-10-21 passing in ru.counterpartApplicationSettingsKeys() feels wrong here.
	//  we need *some* way to give the relation units watcher an idea of what actual
	//  relation it is watching, not just the Scope of what units have/haven't entered.
	//  However, we need the relation ID (which isn't currently passed), and
	//  the application names to be passed. We could
	//  a) pass in 'RelationUnit'
	//  b) pass just the relation id and app names separately
	//  c) filter on what enters scope to determine what 'apps' are connected,
	//     but I was hoping to decouple app settings from scope.
	return newRelationUnitsWatcher(ru.st, ru.WatchScope(), ru.counterpartApplicationSettingsKeys())
}

// WatchUnits returns a watcher that notifies of changes to the units of the
// specified application endpoint in the relation. This method will return an error
// if the endpoint is not globally scoped.
func (r *Relation) WatchUnits(appName string) (RelationUnitsWatcher, error) {
	ep, err := r.Endpoint(appName)
	if err != nil {
		return nil, err
	}
	if ep.Scope != charm.ScopeGlobal {
		return nil, errors.Errorf("%q endpoint is not globally scoped", ep.Name)
	}
	rsw := watchRelationScope(r.st, r.globalScope(), ep.Role, "")
	appSettingsKey := relationApplicationSettingsKey(r.Id(), appName)
	logger.Child("relationunits").Tracef(context.TODO(), "Relation.WatchUnits(%q) watching: %q", appName, appSettingsKey)
	return newRelationUnitsWatcher(r.st, rsw, []string{appSettingsKey}), nil
}

func newRelationUnitsWatcher(backend modelBackend, sw *RelationScopeWatcher, appSettingsKeys []string) RelationUnitsWatcher {
	w := &relationUnitsWatcher{
		commonWatcher:   newCommonWatcher(backend),
		sw:              sw,
		appSettingsKeys: appSettingsKeys,
		watching:        make(set.Strings),
		updates:         make(chan watcher.Change),
		appUpdates:      make(chan watcher.Change),
		out:             make(chan corewatcher.RelationUnitsChange),
		logger:          logger.Child("relationunits"),
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
func (w *relationUnitsWatcher) Changes() corewatcher.RelationUnitsChannel {
	return w.out
}

func emptyRelationUnitsChanges(changes *corewatcher.RelationUnitsChange) bool {
	return len(changes.Changed)+len(changes.AppChanged)+len(changes.Departed) == 0
}

func setRelationUnitChangeVersion(changes *corewatcher.RelationUnitsChange, key string, version int64) {
	name := unitNameFromScopeKey(key)
	settings := corewatcher.UnitSettings{Version: version}
	if changes.Changed == nil {
		changes.Changed = map[string]corewatcher.UnitSettings{}
	}
	changes.Changed[name] = settings
}

func (w *relationUnitsWatcher) watchRelatedAppSettings(changes *corewatcher.RelationUnitsChange) error {
	idsAsInterface := make([]interface{}, len(w.appSettingsKeys))
	for i, key := range w.appSettingsKeys {
		idsAsInterface[i] = w.backend.docID(key)
	}
	w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q watching app keys: %v", w.sw.prefix, w.appSettingsKeys)
	if err := w.watcher.WatchMulti(settingsC, idsAsInterface, w.appUpdates); err != nil {
		return errors.Trace(err)
	}
	// WatchMulti (as a raw DB watcher) does *not* fire an initial event, it just starts the watch, which
	// you then use to know you can read the database without missing updates.
	for _, key := range w.appSettingsKeys {
		if err := w.mergeAppSettings(changes, key); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// mergeSettings reads the relation settings node for the unit with the
// supplied key, and sets a value in the Changed field keyed on the unit's
// name. It returns the mgo/txn revision number of the settings node.
func (w *relationUnitsWatcher) mergeSettings(changes *corewatcher.RelationUnitsChange, key string) error {
	version, err := readSettingsDocVersion(w.backend.db(), settingsC, key)
	if err != nil {
		w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q merging key %q (not found)", w.sw.prefix, key)
		return errors.Trace(err)
	}
	w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q merging key %q version: %d", w.sw.prefix, key, version)
	setRelationUnitChangeVersion(changes, key, version)
	return nil
}

func (w *relationUnitsWatcher) mergeAppSettings(changes *corewatcher.RelationUnitsChange, key string) error {
	version, err := readSettingsDocVersion(w.backend.db(), settingsC, key)
	if err != nil {
		w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q merging app key %q (not found)", w.sw.prefix, key)
		return errors.Trace(err)
	}
	w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q merging app key %q version: %d", w.sw.prefix, key, version)
	if changes.AppChanged == nil {
		changes.AppChanged = make(map[string]int64)
	}
	// This also works for appName
	name := unitNameFromScopeKey(key)
	changes.AppChanged[name] = version
	return nil
}

// mergeScope starts and stops settings watches on the units entering and
// leaving the scope in the supplied RelationScopeChange event, and applies
// the expressed changes to the supplied RelationUnitsChange event.
func (w *relationUnitsWatcher) mergeScope(changes *corewatcher.RelationUnitsChange, c *RelationScopeChange) error {
	docIds := make([]interface{}, len(c.Entered))
	for i, name := range c.Entered {
		key := w.sw.prefix + name
		docID := w.backend.docID(key)
		docIds[i] = docID
	}
	w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q watching newly entered: %v, and unwatching left %v", w.sw.prefix, c.Entered, c.Left)
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
	w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q Change updated to: %# v", w.sw.prefix, changes)
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
	for _, appKey := range w.appSettingsKeys {
		docID := w.backend.docID(appKey)
		w.watcher.Unwatch(settingsC, docID, w.appUpdates)
	}
	close(w.appUpdates)
	close(w.updates)
	close(w.out)
}

func (w *relationUnitsWatcher) loop() (err error) {
	var (
		gotInitialScopeWatcher bool
		sentInitial            bool
		changes                corewatcher.RelationUnitsChange
		out                    chan<- corewatcher.RelationUnitsChange
	)
	// Note that w.ScopeWatcher *does* trigger an initial event, while
	// WatchMulti from raw document watchers does *not*. (raw database watchers
	// don't send initial events, logical watchers built on top of them do.)
	if err := w.watchRelatedAppSettings(&changes); err != nil {
		return errors.Trace(err)
	}
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
			gotInitialScopeWatcher = true
			if w.logger.IsLevelEnabled(corelogger.TRACE) {
				w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q scope Changes(): %# v", w.sw.prefix, pretty.Formatter(c))
			}
			if err = w.mergeScope(&changes, c); err != nil {
				return err
			}
			if !sentInitial || !emptyRelationUnitsChanges(&changes) {
				out = w.out
			} else {
				// If we get a change that negates a previous change, cancel the event
				out = nil
			}
		case c := <-w.updates:
			id, ok := c.Id.(string)
			if !ok {
				w.logger.Warningf(context.TODO(), "relationUnitsWatcher %q ignoring bad relation scope id: %#v", w.sw.prefix, c.Id)
				continue
			}
			w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q relation update %q", w.sw.prefix, id)
			if err := w.mergeSettings(&changes, id); err != nil {
				return errors.Annotatef(err, "relation scope id %q", id)
			}
			if gotInitialScopeWatcher && !emptyRelationUnitsChanges(&changes) {
				out = w.out
			}
		case c := <-w.appUpdates:
			id, ok := c.Id.(string)
			if !ok {
				w.logger.Warningf(context.TODO(), "relationUnitsWatcher %q ignoring bad application settings id: %#v", w.sw.prefix, c.Id)
				continue
			}
			w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q app settings update %q", w.sw.prefix, id)
			if err := w.mergeAppSettings(&changes, id); err != nil {
				return errors.Annotatef(err, "relation scope id %q", id)
			}
			if gotInitialScopeWatcher && (!sentInitial || !emptyRelationUnitsChanges(&changes)) {
				out = w.out
			}
		case out <- changes:
			if w.logger.IsLevelEnabled(corelogger.TRACE) {
				w.logger.Tracef(context.TODO(), "relationUnitsWatcher %q sent changes %# v", w.sw.prefix, pretty.Formatter(changes))
			}
			sentInitial = true
			changes = corewatcher.RelationUnitsChange{}
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
		logger.Tracef(context.TODO(), "error watching %q in %q: %v", ids, unitsC, err)
		return nil, errors.Trace(err)
	}
	logger.Tracef(context.TODO(), "watching %q ids: %q", unitsC, ids)
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
			logger.Tracef(context.TODO(), "marking change for %q", localId)
			changes = append(changes, localId)
		}
		if doc.Life != Dead {
			logger.Tracef(context.TODO(), "setting life of %q to %q", localId, doc.Life)
			w.life[localId] = doc.Life
		} else {
			// Note(jam): 2019-01-31 This was done to match existing behavior, it is not guaranteed
			// to be the behavior we want. Specifically, if we see a Dead unit we will report that
			// it exists in the initial event. However, we stop watching because
			// the object is dead, so you don't get an event when the doc is
			// removed from the database. It seems better if we either/
			// a) don't tell you about Dead documents
			// b) give you an event if a Dead document goes away.
			logger.Tracef(context.TODO(), "unwatching Dead unit: %q", localId)
			w.watcher.Unwatch(unitsC, doc.Id, w.in)
			delete(w.life, localId)
		}
	}
	// See if there are any entries that we wanted to watch but are actually gone
	for _, name := range names {
		if !found.Contains(name) {
			logger.Tracef(context.TODO(), "looking for unit %q, found it gone, Unwatching", name)
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
	logger.Tracef(context.TODO(), "changes: %q", changes)
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
		logger.Tracef(context.TODO(), "unit %q %q no longer in latest, removing watch", unitsC, name)
		delete(w.life, name)
		w.watcher.Unwatch(unitsC, w.backend.docID(name), w.in)
	}
	logger.Tracef(context.TODO(), "update reports changes: %q", changes)
	return changes, nil
}

// merge adds to and returns changes, such that it contains the supplied unit
// name if that unit is unknown and non-Dead, or has changed lifecycle status.
func (w *unitsWatcher) merge(changes []string, name string) ([]string, error) {
	logger.Tracef(context.TODO(), "merging change for %q %q", unitsC, name)
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
		logger.Tracef(context.TODO(), "document gone, unwatching %q %q", unitsC, name)
		w.watcher.Unwatch(unitsC, w.backend.docID(name), w.in)
	case life != doc.Life:
		logger.Tracef(context.TODO(), "updating doc life %q %q to %q", unitsC, name, doc.Life)
		w.life[name] = doc.Life
	default:
		return changes, nil
	}
	if !hasString(changes, name) {
		changes = append(changes, name)
	}
	logger.Tracef(context.TODO(), "merge reporting changes: %q", changes)
	return changes, nil
}

func (w *unitsWatcher) loop(coll, id string) error {
	logger.Tracef(context.TODO(), "watching root channel %q %q", coll, id)
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
			if err != nil {
				return err
			}
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
			logger.Tracef(context.TODO(), "watcher reported changes: %q", changes)
			out = nil
			changes = nil
		}
	}
}

// WatchControllerInfo returns a StringsWatcher for the controllers collection
func (st *State) WatchControllerInfo() StringsWatcher {
	return newCollectionWatcher(st, colWCfg{col: controllerNodesC})
}

// Watch returns a watcher for observing changes to a controller service.
func (c *CloudService) Watch() NotifyWatcher {
	return newEntityWatcher(c.st, cloudServicesC, c.doc.DocID)
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
		return nil, fmt.Errorf("unit's charm URL must be set before watching config")
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

// WatchLXDProfileUpgradeNotifications returns a watcher that observes the status
// of a lxd profile upgrade by monitoring changes on the unit machine's lxd profile
// upgrade completed field that is specific to an application name.  Used by
// UniterAPI v9.
func (m *Machine) WatchLXDProfileUpgradeNotifications(applicationName string) (StringsWatcher, error) {
	app, err := m.st.Application(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	watchDocId := app.doc.DocID
	return watchInstanceCharmProfileCompatibilityData(m.st, watchDocId), nil
}

// WatchLXDProfileUpgradeNotifications returns a watcher that observes the status
// of a lxd profile upgrade by monitoring changes on the unit machine's lxd profile
// upgrade completed field that is specific to itself.
func (u *Unit) WatchLXDProfileUpgradeNotifications() (StringsWatcher, error) {
	app, err := u.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}
	watchDocId := app.doc.DocID
	return watchInstanceCharmProfileCompatibilityData(u.st, watchDocId), nil
}

func watchInstanceCharmProfileCompatibilityData(backend modelBackend, watchDocId string) StringsWatcher {
	initial := ""
	members := bson.D{{"_id", watchDocId}}
	collection := applicationsC
	filter := func(id interface{}) bool {
		return id.(string) == watchDocId
	}
	extract := func(query documentFieldWatcherQuery) (string, error) {
		var doc applicationDoc
		if err := query.One(&doc); err != nil {
			return "", err
		}
		return *doc.CharmURL, nil
	}
	transform := func(value string) string {
		return lxdprofile.NotRequiredStatus
	}
	return newDocumentFieldWatcher(backend, collection, members, initial, filter, extract, transform)
}

// *Deprecated* Although this watcher seems fairly admirable in terms of what
// it does, it unfortunately does things at the wrong level. With the
// consequence of wiring up complex structures on something that wasn't intended
// from the outset for it to do.
//
// documentFieldWatcher notifies about any changes to a document field
// specifically, the watcher looks for changes to a document field, and records
// the current document field (known value). If the document doesn't exist an
// initialKnown value can be set for the default.
// Events are generated when there are changes to a document field that is
// different from the known value. So setting field multiple times won't
// dispatch an event, on changes that differ will be dispatched.
type documentFieldWatcher struct {
	commonWatcher
	// docId is used to select the initial interesting entities.
	collection   string
	members      bson.D
	known        *string
	initialKnown string
	filter       func(interface{}) bool
	extract      func(documentFieldWatcherQuery) (string, error)
	transform    func(string) string
	out          chan []string
}

// documentFieldWatcherQuery is a point of use interface, to prevent the leaking
// of query interface out of the core watcher.
type documentFieldWatcherQuery interface {
	One(result interface{}) (err error)
}

var _ Watcher = (*documentFieldWatcher)(nil)

func newDocumentFieldWatcher(
	backend modelBackend,
	collection string,
	members bson.D,
	initialKnown string,
	filter func(interface{}) bool,
	extract func(documentFieldWatcherQuery) (string, error),
	transform func(string) string,
) StringsWatcher {
	w := &documentFieldWatcher{
		commonWatcher: newCommonWatcher(backend),
		collection:    collection,
		members:       members,
		initialKnown:  initialKnown,
		filter:        filter,
		extract:       extract,
		transform:     transform,
		out:           make(chan []string),
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

func (w *documentFieldWatcher) initial() error {
	col, closer := w.db.GetCollection(w.collection)
	defer closer()

	field := w.initialKnown

	if newField, err := w.extract(col.Find(w.members)); err == nil {
		field = newField
	}
	w.known = &field

	logger.Tracef(context.TODO(), "Started watching %s for %v: %q", w.collection, w.members, field)
	return nil
}

func (w *documentFieldWatcher) merge(change watcher.Change) (bool, error) {
	// we care about change.Revno equalling -1 as we want to know about
	// documents being deleted.
	if change.Revno == -1 {
		// treat this as the document being deleted
		if w.known != nil {
			w.known = nil
			return true, nil
		}
		return false, nil
	}
	col, closer := w.db.GetCollection(w.collection)
	defer closer()

	// check the field before adding it to the known value
	currentField, err := w.extract(col.Find(w.members))
	if err != nil {
		if err != mgo.ErrNotFound {
			logger.Debugf(context.TODO(), "%s NOT mgo err not found", w.collection)
			return false, err
		}
		// treat this as the document being deleted
		if w.known != nil {
			w.known = nil
			return true, nil
		}
		return false, nil
	}
	if w.known == nil || *w.known != currentField {
		w.known = &currentField

		logger.Tracef(context.TODO(), "Changes in watching %s for %v: %q", w.collection, w.members, currentField)
		return true, nil
	}
	return false, nil
}

func (w *documentFieldWatcher) loop() error {
	err := w.initial()
	if err != nil {
		return err
	}

	ch := make(chan watcher.Change)
	w.watcher.WatchCollectionWithFilter(w.collection, ch, w.filter)
	defer w.watcher.UnwatchCollection(w.collection, ch)

	out := w.out
	for {
		var value string
		if w.known != nil {
			value = *w.known
		}
		if w.transform != nil {
			value = w.transform(value)
		}
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
		case out <- []string{value}:
			out = nil
		}
	}
}

func (w *documentFieldWatcher) Changes() <-chan []string {
	return w.out
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
	logger.Tracef(context.TODO(), "for machine %q watching new units %q", w.machine.doc.DocID, unitNames)
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
		logger.Debugf(context.TODO(), "unit %q referenced but not found", name)
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
	logger.Tracef(context.TODO(), "machineUnitsWatcher removing unit %q for life %q", doc.Name, doc.Life)
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
		logger.Tracef(context.TODO(), "machineUnitsWatcher found life changed to %q => %q for %q", life, doc.Life, doc.Name)
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
			watchLogger.Errorf(context.TODO(), "key is not type string, got %T", key)
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

// watchActionNotificationsFilteredBy starts and returns a StringsWatcher
// that notifies on new Actions being enqueued on the ActionRecevers
// being watched as well as changes to non-completed Actions.
func (st *State) watchActionNotificationsFilteredBy(receivers ...ActionReceiver) StringsWatcher {
	return newActionNotificationWatcher(st, false, receivers...)
}

// watchEnqueuedActionsFilteredBy starts and returns a StringsWatcher
// that notifies on new Actions being enqueued on the ActionRecevers
// being watched.
func (st *State) watchEnqueuedActionsFilteredBy(receivers ...ActionReceiver) StringsWatcher {
	return newActionNotificationWatcher(st, true, receivers...)
}

// actionNotificationWatcher is a StringsWatcher that watches for changes on the
// action notification collection, but only triggers events once per action.
type actionNotificationWatcher struct {
	commonWatcher
	source chan watcher.Change
	sink   chan []string
	filter func(interface{}) bool
	// notifyPending when true will notify all pending and running actions as
	// initial events, but thereafter only notify on pending actions.
	notifyPending bool
}

// newActionNotificationWatcher starts and returns a new StringsWatcher configured
// with the given collection and filter function. notifyPending when true will notify all pending and running actions as
// initial events, but thereafter only notify on pending actions.
func newActionNotificationWatcher(backend modelBackend, notifyPending bool, receivers ...ActionReceiver) StringsWatcher {
	w := &actionNotificationWatcher{
		commonWatcher: newCommonWatcher(backend),
		source:        make(chan watcher.Change),
		sink:          make(chan []string),
		filter:        makeIdFilter(backend, actionMarker, receivers...),
		notifyPending: notifyPending,
	}

	w.tomb.Go(func() error {
		defer close(w.sink)
		defer close(w.source)
		return w.loop()
	})

	return w
}

// Changes returns the event channel for this watcher
func (w *actionNotificationWatcher) Changes() <-chan []string {
	return w.sink
}

func (w *actionNotificationWatcher) loop() error {
	var (
		changes []string
		in      = (<-chan watcher.Change)(w.source)
		out     = (chan<- []string)(w.sink)
	)

	w.watcher.WatchCollectionWithFilter(actionNotificationsC, w.source, w.filter)
	defer w.watcher.UnwatchCollection(actionNotificationsC, w.source)

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
			if w.notifyPending {
				if err := w.filterPendingAndMergeIds(&changes, updates); err != nil {
					return err
				}
			} else {
				if err := w.mergeIds(&changes, updates); err != nil {
					return err
				}
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

func (w *actionNotificationWatcher) initial() ([]string, error) {
	var ids []string
	var doc actionNotificationDoc
	coll, closer := w.db.GetCollection(actionNotificationsC)
	defer closer()
	iter := coll.Find(nil).Iter()
	for iter.Next(&doc) {
		if w.filter(doc.DocId) {
			ids = append(ids, actionNotificationIdToActionId(doc.DocId))
		}
	}
	return ids, iter.Close()
}

// filterPendingAndMergeIds reduces the keys published to the first action notification (pending actions).
func (w *actionNotificationWatcher) filterPendingAndMergeIds(changes *[]string, updates map[interface{}]bool) error {
	var newIDs []string
	for val, idExists := range updates {
		docID, ok := val.(string)
		if !ok {
			return errors.Errorf("id is not of type string, got %T", val)
		}

		id := actionNotificationIdToActionId(docID)
		chIx, idAlreadyInChangeset := indexOf(id, *changes)
		if idExists {
			if !idAlreadyInChangeset {
				// add id to fetch from mongo
				newIDs = append(newIDs, w.backend.localID(docID))
			}
		} else {
			if idAlreadyInChangeset {
				// remove id from changes
				*changes = append((*changes)[:chIx], (*changes)[chIx+1:]...)
			}
		}
	}

	coll, closer := w.db.GetCollection(actionNotificationsC)
	defer closer()

	// query for all documents that match the ids who
	// don't have a changed field. These are new pending actions.
	query := bson.D{{"_id", bson.D{{"$in", newIDs}}}}
	var doc actionNotificationDoc
	iter := coll.Find(query).Iter()
	for iter.Next(&doc) {
		if doc.Changed.IsZero() {
			*changes = append(*changes, actionNotificationIdToActionId(doc.DocId))
		}
	}
	return iter.Close()
}

func (w *actionNotificationWatcher) mergeIds(changes *[]string, updates map[interface{}]bool) error {
	return mergeIds(changes, updates, func(id string) (string, error) {
		return actionNotificationIdToActionId(id), nil
	})
}

// WatchControllerStatusChanges starts and returns a StringsWatcher that
// notifies when the status of a controller node changes.
// TODO(cherylj) Add unit tests for this, as per bug 1543408.
func (st *State) WatchControllerStatusChanges() StringsWatcher {
	return newCollectionWatcher(st, colWCfg{
		col:    statusesC,
		filter: makeControllerIdFilter(st),
	})
}

func makeControllerIdFilter(st *State) func(interface{}) bool {
	initialNodes, err := st.ControllerNodes()
	if err != nil {
		logger.Debugf(context.TODO(), "unable to get controller nodes: %v", err)
		return nil
	}

	filter := controllerIdFilter{
		st:        st,
		lastNodes: make([]string, len(initialNodes)),
	}
	for i, n := range initialNodes {
		filter.lastNodes[i] = n.Id()
	}
	return filter.match
}

// controllerIdFilter is a stateful watcher filter function - if it
// can't get the current controller nodes it uses the
// last nodes retrieved. Since this is called from multiple
// goroutines getting/updating lastNodes is protected by a mutex.
type controllerIdFilter struct {
	mu        sync.Mutex
	st        *State
	lastNodes []string
}

func (f *controllerIdFilter) nodeIds() []string {
	var result []string
	nodes, err := f.st.ControllerNodes()
	f.mu.Lock()
	if err != nil {
		// Most likely, things will be killed and
		// restarted if we hit this error.  Just use
		// the machine list we knew about last time.
		logger.Debugf(context.TODO(), "unable to get controller info: %v", err)
		result = f.lastNodes
	} else {
		ids := make([]string, len(nodes))
		for i, n := range nodes {
			ids[i] = n.Id()
		}
		f.lastNodes = ids
		result = ids
	}
	f.mu.Unlock()
	return result
}

func (f *controllerIdFilter) match(key interface{}) bool {
	switch key.(type) {
	case string:
		nodeIds := f.nodeIds()
		for _, id := range nodeIds {
			if strings.HasSuffix(key.(string), fmt.Sprintf("m#%s", id)) {
				return true
			}
			// TODO(HA) - add k8s controller filter when we do k8s HA
		}
	default:
		watchLogger.Errorf(context.TODO(), "key is not type string, got %T", key)
	}
	return false
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
	_, _ = hash.Write([]byte(name))
	_, _ = hash.Write(data)
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// WatchServiceAddressesHash returns a StringsWatcher that emits a
// hash of the unit's container address whenever it changes.
func (a *Application) WatchServiceAddressesHash() StringsWatcher {
	firstCall := true
	w := &hashWatcher{
		commonWatcher: newCommonWatcher(a.st),
		out:           make(chan []string),
		collection:    cloudServicesC,
		id:            a.st.docID(a.globalKey()),
		hash: func() (string, error) {
			result, err := hashServiceAddresses(a, firstCall)
			firstCall = false
			return result, err
		},
	}
	w.start()
	return w
}

// WatchConfigSettingsHash returns a watcher that yields a hash of the
// application's config settings whenever they are changed.
func (a *Application) WatchConfigSettingsHash() StringsWatcher {
	applicationConfigKey := applicationConfigKey(a.Name())
	return newSettingsHashWatcher(a.st, applicationConfigKey)
}

func hashServiceAddresses(a *Application, firstCall bool) (string, error) {
	service, err := a.ServiceInfo()
	if firstCall && errors.Is(err, errors.NotFound) {
		// To keep behaviour the same as
		// WatchServiceAddresses, we need to ignore NotFound
		// errors on the first call but propagate them after
		// that.
		return "", nil
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	addresses := service.Addresses()
	if len(addresses) == 0 {
		return "", nil
	}
	address := addresses[0]
	hash := sha256.New()
	_, _ = hash.Write([]byte(address.Value))
	_, _ = hash.Write([]byte(address.Type))
	_, _ = hash.Write([]byte(address.Scope))
	_, _ = hash.Write([]byte(address.SpaceID))
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

// WatchMachineAndEndpointAddressesHash returns a StringsWatcher that reports changes to
// the hash value of the address assignments to the unit's endpoints. The hash
// is recalculated when any of the following events occurs:
// - the machine addresses for the unit change.
// - the endpoint bindings for the unit's application change.
func (u *Unit) WatchMachineAndEndpointAddressesHash() (StringsWatcher, error) {
	app, err := u.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}
	endpointsDoc, err := readEndpointBindingsDoc(app.st, app.globalKey())
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineId, err := u.AssignedMachineId()
	if err != nil {
		return nil, errors.Trace(err)
	}
	machine, err := u.st.Machine(machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	mCopy := &Machine{st: machine.st, doc: machine.doc}

	// We need to check the hash when either the bindings change or when
	// the machine doc changes (e.g. machine has new network addresses).
	w := &hashMultiWatcher{
		commonWatcher: newCommonWatcher(app.st),
		out:           make(chan []string),
		collectionToIDMap: map[string]string{
			endpointBindingsC: endpointsDoc.DocID,
			machinesC:         machine.doc.DocID,
		},
		hash: func() (string, error) {
			bindings, err := app.EndpointBindings()
			if err != nil {
				return "", err
			}
			return hashMachineAddressesForEndpointBindings(mCopy, bindings.Map())
		},
	}
	w.start()
	return w, nil
}

func hashMachineAddressesForEndpointBindings(m *Machine, bindingsToSpaceIDs map[string]string) (string, error) {
	if err := m.Refresh(); err != nil {
		return "", errors.Trace(err)
	}
	hash := sha256.New()

	addresses := m.Addresses()
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Value < addresses[j].Value })
	for _, address := range addresses {
		hashAddr(hash, address)
	}

	// Also include binding assignments to the hash. We don't care about
	// address assignments at this point; if the machine addresses change
	// (e.g. due to a reboot), the above code block would yield a different
	// hash.
	sortedEndpoints := make([]string, 0, len(bindingsToSpaceIDs))
	for epName := range bindingsToSpaceIDs {
		sortedEndpoints = append(sortedEndpoints, epName)
	}
	sort.Strings(sortedEndpoints)

	for _, epName := range sortedEndpoints {
		if epName == "" {
			continue
		}
		_, _ = hash.Write([]byte(fmt.Sprintf("%s:%s", epName, bindingsToSpaceIDs[epName])))
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func hashAddr(h hash.Hash, addr corenetwork.SpaceAddress) {
	_, _ = h.Write([]byte(addr.Value))
	_, _ = h.Write([]byte(addr.Type))
	_, _ = h.Write([]byte(addr.Scope))
	_, _ = h.Write([]byte(addr.SpaceID))
}

// hashMultiWatcher watches a set of documents for changes, invokes a
// user-defined hash function and emits changes in the hash value.
type hashMultiWatcher struct {
	commonWatcher
	collectionToIDMap map[string]string
	hash              func() (string, error)
	out               chan []string
}

func (w *hashMultiWatcher) Changes() <-chan []string {
	return w.out
}

func (w *hashMultiWatcher) start() {
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
}

func (w *hashMultiWatcher) loop() error {
	changesCh := make(chan watcher.Change)
	for collection, id := range w.collectionToIDMap {
		w.watcher.Watch(collection, id, changesCh)
	}
	defer func() {
		for collection, id := range w.collectionToIDMap {
			w.watcher.Unwatch(collection, id, changesCh)
		}
	}()

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
