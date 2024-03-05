// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/wrench"
)

// Hub represents a pubsub hub. The TxnWatcher only ever publishes
// events to the hub.
type Hub interface {
	Publish(topic string, data interface{}) func()
}

// Clock represents the time methods used.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

const (
	// TxnWatcherSyncErr is published to the TxnWatcher's hub if there's a
	// sync error (e.g., an error iterating through the collection's rows).
	TxnWatcherSyncErr = "sync err"
	// TxnWatcherCollection is published to the TxnWatcher's hub for each
	// change (data is the Change instance).
	TxnWatcherCollection = "collection"

	// txnWatcherErrorWait is the delay after an error before trying to resume
	// the change stream by a call to aggregate changeStream.
	txnWatcherErrorWait = 5 * time.Second
)

// RunCmdFunc allows tests to override calls to mongo db.Run.
type RunCmdFunc func(db *mgo.Database, cmd any, resp any) error

// A TxnWatcher watches a mongo change stream and publishes all change events
// to the hub.
type TxnWatcher struct {
	hub    Hub
	clock  Clock
	logger Logger

	tomb       tomb.Tomb
	session    *mgo.Session
	jujuDBName string

	reportRequest chan chan map[string]interface{}

	// syncEvents contain the events to be
	// dispatched to the watcher channels. They're queued during
	// processing and flushed at the end to simplify the algorithm.
	// The two queues are separated because events from sync are
	// handled in reverse order due to the way the algorithm works.
	syncEvents []Change

	// iteratorStepCount tracks how many documents we've read from the database
	iteratorStepCount uint64

	// changesCount tracks all sync events that we have processed
	changesCount uint64

	// syncEventsLastLen was the length of syncEvents when we did our last flush()
	syncEventsLastLen int

	// averageSyncLen tracks a filtered average of how long the sync event queue gets before we flush
	averageSyncLen float64

	// resumeToken is passed back by the aggregate changeStream call so that a change stream may
	// be resumed if the cursor dies for some reason.
	resumeToken bson.Raw

	// cursorId is the id of the current cursor used for paging results from the change stream.
	cursorId int64

	// pollInterval is the duration of the long polling getMore call on the cursor.
	pollInterval time.Duration

	// readyChan is closed when the watcher has started the change stream, used for tests.
	readyChan chan any

	// runCmd can be used to override the db.Run function, used for tests.
	runCmd RunCmdFunc

	// ignoreCollections when set the watcher will ignore all changes for these collections.
	ignoreCollections []string
}

// TxnWatcherConfig contains the configuration parameters required
// for a NewTxnWatcher.
type TxnWatcherConfig struct {
	// Session is used exclusively fot this TxnWatcher.
	Session *mgo.Session
	// JujuDBName is the Juju database name, usually "juju".
	JujuDBName string
	// Hub is where the changes are published to.
	Hub Hub
	// Clock allows tests to control the advancing of time.
	Clock Clock
	// Logger is used to control where the log messages for this watcher go.
	Logger Logger
	// PollInterval is used to set how long mongo will wait before returning an
	// empty result. It defaults to 1s.
	PollInterval time.Duration
	// RunCmd is called to run commands against an mgo.Database, used for
	// testing.
	RunCmd RunCmdFunc
	// IgnoreCollections is optional, when provided the TxnWatcher will ignore changes
	// to documents in these collections.
	IgnoreCollections []string
}

// Validate ensures that all the values that have to be set are set.
func (config TxnWatcherConfig) Validate() error {
	if config.Session == nil {
		return errors.NotValidf("missing Session")
	}
	if config.JujuDBName == "" {
		return errors.NotValidf("missing JujuDBName")
	}
	if config.Hub == nil {
		return errors.NotValidf("missing Hub")
	}
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if config.PollInterval < 0 {
		return errors.NotValidf("negative PollInterval")
	}
	return nil
}

// NewTxnWatcher returns a new Watcher observing the changelog collection.
func NewTxnWatcher(config TxnWatcherConfig) (*TxnWatcher, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "new TxnWatcher invalid config")
	}

	w := &TxnWatcher{
		hub:               config.Hub,
		clock:             config.Clock,
		logger:            config.Logger,
		session:           config.Session,
		jujuDBName:        config.JujuDBName,
		reportRequest:     make(chan chan map[string]interface{}),
		readyChan:         make(chan any),
		pollInterval:      config.PollInterval,
		runCmd:            config.RunCmd,
		ignoreCollections: config.IgnoreCollections,
	}
	if w.logger == nil {
		w.logger = noOpLogger{}
	}
	if w.pollInterval == 0 {
		w.pollInterval = 1 * time.Second
	}
	if w.runCmd == nil {
		w.runCmd = w.runCmdImpl
	}
	w.tomb.Go(func() error {
		err := w.loop()
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		cause := errors.Cause(err)
		switch cause {
		case tomb.ErrDying:
			return tomb.ErrDying
		case tomb.ErrStillAlive:
			return tomb.ErrStillAlive
		}
		if err != nil {
			w.logger.Infof("watcher loop failed: %v", err)
		}
		return err
	})
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *TxnWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *TxnWatcher) Wait() error {
	return w.tomb.Wait()
}

// Stop stops all the watcher activities.
func (w *TxnWatcher) Stop() error {
	return worker.Stop(w)
}

// Dead returns a channel that is closed when the watcher has stopped.
func (w *TxnWatcher) Dead() <-chan struct{} {
	return w.tomb.Dead()
}

// Err returns the error with which the watcher stopped.
// It returns nil if the watcher stopped cleanly, tomb.ErrStillAlive
// if the watcher is still running properly, or the respective error
// if the watcher is terminating or has terminated with an error.
func (w *TxnWatcher) Err() error {
	return w.tomb.Err()
}

// Report is part of the watcher/runner Reporting interface, to expose runtime details of the watcher.
func (w *TxnWatcher) Report() map[string]interface{} {
	// TODO: (jam) do we need to synchronize with the loop?
	resCh := make(chan map[string]interface{})
	select {
	case <-w.tomb.Dying():
		return nil
	case w.reportRequest <- resCh:
		break
	}
	select {
	case <-w.tomb.Dying():
		return nil
	case res := <-resCh:
		return res
	}
}

// Ready waits for the watcher to have started the change stream against mongo or return an error.
// Useful for testing.
func (w *TxnWatcher) Ready() error {
	select {
	case <-w.tomb.Dying():
		return w.tomb.Err()
	case <-w.readyChan:
		return nil
	}
}

// loop implements the main watcher loop.
// period is the delay between each sync.
func (w *TxnWatcher) loop() error {
	w.logger.Tracef("loop started")
	defer w.logger.Tracef("loop finished")

	// Change Stream specification can be found here:
	// https://github.com/mongodb/specifications/blob/master/source/change-streams/change-streams.rst
	w.resumeToken = bson.Raw{}
	errorOccurred := false

	immediate := make(chan time.Time)
	close(immediate)
	var next <-chan time.Time = immediate

	first := true

	for {
		select {
		case <-w.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case <-next:
			next = immediate
		case resCh := <-w.reportRequest:
			report := map[string]interface{}{
				// How long was sync-events in our last flush
				"sync-events-last-len": w.syncEventsLastLen,
				// How long is sync-events on average
				"sync-events-avg": int(w.averageSyncLen + 0.5),
				// How long is the queue right now? (probably should always be 0 if we are at this point in the loop)
				"sync-events-len": len(w.syncEvents),
				// How big is our buffer
				"sync-events-cap": cap(w.syncEvents),
				// How many events have we actually generated
				"total-changes": w.changesCount,
				// How many database records have we read. note: because we have to iterate until we get to lastId,
				// this is often a bit bigger than total-sync-events
				"iterator-step-count": w.iteratorStepCount,
			}
			select {
			case <-w.tomb.Dying():
				return errors.Trace(tomb.ErrDying)
			case resCh <- report:
			}
			// This doesn't indicate we need to perform a sync
			continue
		}

		var err error
		if errorOccurred || first {
			if !first {
				w.killCursor()
			}
			err = w.init()
			if err != nil {
				err = errors.Wrap(FatalChangeStreamError, err)
			}
			if first {
				defer w.killCursor()
				w.logger.Infof("txn watcher started")
				close(w.readyChan)
				first = false
			}
		} else {
			err = w.sync()
		}
		if w.logger.IsTraceEnabled() && wrench.IsActive("txnwatcher", "sync-error") {
			err = errors.New("test sync watcher error")
		}
		if err == nil {
			if errorOccurred {
				w.logger.Infof("txn sync watcher resumed after failure")
			}
			errorOccurred = false
			w.flush()
		} else {
			w.logger.Warningf("txn watcher sync error: %v", err)
			// If didn't get a resumable error and either we have already had one error resuming
			// or received a fatal error, then exit the watcher and report failure.
			if !errors.Is(err, ResumableChangeStreamError) &&
				(errors.Is(err, FatalChangeStreamError) || errorOccurred) {
				_ = w.hub.Publish(TxnWatcherSyncErr, err)
				return errors.Trace(err)
			}
			w.logger.Warningf("txn watcher resume queued")
			errorOccurred = true
			next = w.clock.After(txnWatcherErrorWait)
		}
	}
}

func (w *TxnWatcher) killCursor() {
	if w.cursorId == 0 {
		return
	}
	db := w.session.DB(w.jujuDBName)
	var err error
	for i := 0; i < 2; i++ {
		err = w.runCmd(db, bson.D{
			{"killCursors", "$cmd.aggregate"},
			{"cursors", []int64{w.cursorId}},
		}, nil)
		if isCursorNotFoundError(err) {
			break
		} else if err == nil {
			break
		}
		w.logger.Warningf("failed to kill cursor %d: %s", w.cursorId, err.Error())
	}
	w.cursorId = 0
}

func (w *TxnWatcher) init() error {
	db := w.session.DB(w.jujuDBName)

	cs := bson.M{
		"fullDocument": "updateLookup",
	}
	if len(w.resumeToken.Data) > 0 {
		cs["resumeAfter"] = w.resumeToken
	}

	match := bson.D{
		{"$or", []bson.D{
			{{"operationType", "insert"}},
			{{"operationType", "update"}},
			{{"operationType", "replace"}},
			{{"operationType", "delete"}},
		}},
	}

	if len(w.ignoreCollections) > 0 {
		match = append(match, bson.DocElem{"ns.coll", bson.D{{"$not", bson.D{{"$in", w.ignoreCollections}}}}})
	}

	cmd := bson.D{
		{"aggregate", 1},
		{"pipeline", []bson.D{
			{{"$changeStream", cs}},
			{{"$match", match}},
		}},
		{"cursor", bson.D{{"batchSize", 10}}},
		{"readConcern", bson.D{{"level", "majority"}}},
	}

	resp := aggregateResponse{}
	err := w.runCmd(db, cmd, &resp)
	if err != nil {
		return errors.Annotate(err, "starting change stream")
	}
	w.cursorId = resp.Cursor.Id
	err = w.process(resp.Cursor.FirstBatch)
	if err != nil {
		return errors.Annotate(err, "processing first change stream batch")
	}
	w.resumeToken = resp.Cursor.PostBatchResumeToken

	return nil
}

func (w *TxnWatcher) process(changes []bson.Raw) error {
	for i := len(changes) - 1; i >= 0; i-- {
		changeRaw := changes[i]
		change := changeStreamDocument{}
		err := bson.Unmarshal(changeRaw.Data, &change)
		if err != nil {
			return errors.Trace(err)
		}
		if len(change.Id.Data) == 0 {
			return errors.Annotate(FatalChangeStreamError, "missing change id in change")
		}
		revno := int64(0)
		switch change.OperationType {
		case "insert", "replace":
			// We read the revno from the committed document as reported by the change stream.
			revno = change.FullDocument.Revno
		case "update":
			// For updates we read the revno from the updated fields so that it isn't newer as
			// FullDocument on an update is only guaranteed to be the state of the document after
			// the update, not the state of the document as a result of the update, so the reported
			// revno could be greater.
			revno = change.UpdateDescription.UpdatedFields.Revno
			if revno == 0 && change.TxnNumber == 0 {
				a := bson.M{}
				err := bson.Unmarshal(changeRaw.Data, &a)
				if err != nil {
					return errors.Trace(err)
				}
				j, err := bson.MarshalJSON(a)
				if err != nil {
					return errors.Trace(err)
				}
				// If you get this message then a document was updated outside a mongo multi-doc transaction
				// and wasn't involved in a jujutxn (incrementing txn-revno).
				// If you were trying to modify a document from the mongo cli, you need to start a transaction
				// then modify the document while also incrementing its txn-revno field. Then commit the transaction.
				// This will correctly trigger the watcher.
				// If you are seeing this otherwise, maybe the collection should not have been watched, so go update
				// the watcherIgnoreList in state/allcollections.go
				// If you want to receive watch notifications on your collection and get here, you need to make sure
				// all mutations are done inside a jujutxn otherwise you have a programming error and the change will
				// get skipped.
				w.logger.Criticalf("update %s %v without revno %s", change.Ns.Collection, change.DocumentKey.Id, j)
				w.resumeToken = change.Id
				continue
			} else if revno == 0 && change.TxnNumber != 0 {
				// Skip update because the revno "$set" was elided.
				w.resumeToken = change.Id
				continue
			}
		case "delete":
			// Revno of -1 indicates a deleted document.
			revno = -1
		default:
			// If we have a bad change, then we can just skip it, making sure to update the resumption token
			// to resume after this event.
			w.logger.Warningf("received bad change type %s", change.OperationType)
			w.resumeToken = change.Id
			continue
		}
		w.logger.Debugf("txn watcher: %s %v #%d", change.Ns.Collection, change.DocumentKey.Id, revno)
		w.syncEvents = append(w.syncEvents, Change{
			C:     change.Ns.Collection,
			Id:    change.DocumentKey.Id,
			Revno: revno,
		})
		w.changesCount++
		w.resumeToken = change.Id
	}

	return nil
}

// flush sends all pending events to their respective channels.
func (w *TxnWatcher) flush() {
	// refreshEvents are stored newest first.
	for i := len(w.syncEvents) - 1; i >= 0; i-- {
		e := w.syncEvents[i]
		_ = w.hub.Publish(TxnWatcherCollection, e)
	}
	w.averageSyncLen = (filterFactor * float64(len(w.syncEvents))) + ((1.0 - filterFactor) * w.averageSyncLen)
	w.syncEventsLastLen = len(w.syncEvents)
	w.syncEvents = w.syncEvents[:0]
	// TODO(jam): 2018-11-07 Consider if averageSyncLen << cap(syncEvents) we should reallocate the buffer, so that it
	// doesn't grow to the size of the largest-ever change and never shrink
}

// sync updates the watcher knowledge from the database, and
// queues events to observing channels.
func (w *TxnWatcher) sync() error {
	w.logger.Tracef("txn watcher %p starting sync", w)

	db := w.session.DB(w.jujuDBName)
	resp := aggregateResponse{}
	var err error
	for i := 0; i < 2; i++ {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		default:
		}
		err = w.runCmd(db, bson.D{
			{"getMore", w.cursorId},
			{"collection", "$cmd.aggregate"},
			{"batchSize", 10},
			{"maxTimeMS", w.pollInterval.Milliseconds()},
			{"readConcern", bson.D{{"level", "majority"}}},
		}, &resp)
		if isCursorNotFoundError(err) {
			return errors.Wrap(ResumableChangeStreamError, err)
		} else if isFatalGetMoreError(err) {
			return errors.Wrap(FatalChangeStreamError, err)
		} else if err == nil {
			break
		}
	}
	if err != nil {
		return errors.Wrap(ResumableChangeStreamError, err)
	}

	err = w.process(resp.Cursor.NextBatch)
	if err != nil {
		return errors.Trace(err)
	}
	w.resumeToken = resp.Cursor.PostBatchResumeToken

	return nil
}

func (w *TxnWatcher) runCmdImpl(db *mgo.Database, cmd any, resp any) error {
	return db.Run(cmd, resp)
}

type changeStreamDocument struct {
	Id                bson.Raw            `bson:"_id"`
	OperationType     string              `bson:"operationType"`
	Ns                nsRef               `bson:"ns"`
	DocumentKey       *docKey             `bson:"documentKey"`
	ClusterTime       bson.MongoTimestamp `bson:"clusterTime"`
	FullDocument      *txnDoc             `bson:"fullDocument"`
	UpdateDescription *updateDescription  `bson:"updateDescription"`
	TxnNumber         int64               `bson:"txnNumber"`
}

type updateDescription struct {
	UpdatedFields txnDoc `bson:"updatedFields"`
}

type nsRef struct {
	Database   string `bson:"db"`
	Collection string `bson:"coll"`
}

type docKey struct {
	Id any `bson:"_id"`
}

type txnDoc struct {
	Revno int64 `bson:"txn-revno"`
}

type cursorData struct {
	FirstBatch           []bson.Raw `bson:"firstBatch"`
	NextBatch            []bson.Raw `bson:"nextBatch"`
	Ns                   string     `bson:"ns"`
	Id                   int64      `bson:"id"`
	PostBatchResumeToken bson.Raw   `bson:"postBatchResumeToken"`
}

type aggregateResponse struct {
	Ok     bool
	Cursor cursorData
}

func isCursorNotFoundError(err error) bool {
	code := 0
	switch e := err.(type) {
	case *mgo.LastError:
		code = e.Code
	case *mgo.QueryError:
		code = e.Code
	}
	switch code {
	case 43: // CursorNotFound
		return true
	}
	return false
}

func isFatalGetMoreError(err error) bool {
	code := 0
	switch e := err.(type) {
	case *mgo.LastError:
		code = e.Code
	case *mgo.QueryError:
		code = e.Code
	}
	switch code {
	case
		6,     // HostUnreachable
		7,     // HostNotFound
		89,    // NetworkTimeout
		91,    // ShutdownInProgress
		189,   // PrimarySteppedDown
		262,   // ExceededTimeLimit
		9001,  // SocketException
		10107, // NotWritablePrimary
		11600, // InterruptedAtShutdown
		11602, // InterruptedDueToReplStateChange
		13435, // NotPrimaryNoSecondaryOk
		13436, // NotPrimaryOrSecondary
		63,    // StaleShardVersion
		150,   // StaleEpoch
		13388, // StaleConfig
		234,   // RetryChangeStream
		133:   // FailedToSatisfyReadPreference
		return true
	}
	return false
}

const (
	FatalChangeStreamError     errors.ConstError = "fatal change stream error"
	ResumableChangeStreamError errors.ConstError = "resumable change stream error"
)
