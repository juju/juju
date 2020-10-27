// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/retry.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/wrench"
)

// Hub represents a pubsub hub. The TxnWatcher only ever publishes
// events to the hub.
type Hub interface {
	Publish(topic string, data interface{}) <-chan struct{}
}

// Clock represents the time methods used.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

const (
	txnWatcherStarting   = "starting"
	txnWatcherSyncErr    = "sync err"
	txnWatcherCollection = "collection"

	txnWatcherShortWait = 10 * time.Millisecond

	maxSyncRetries           = 10
	txnWatcherErrorShortWait = 500 * time.Millisecond
)

var (

	// PollStrategy is used to determine how long
	// to delay between poll intervals. A new timer
	// is created each time some watcher event is
	// fired or if the old timer completes.
	//
	// It must not be changed when any watchers are active.
	PollStrategy retry.Strategy = retry.Exponential{
		Initial:  txnWatcherShortWait,
		Factor:   1.5,
		MaxDelay: 5 * time.Second,
	}

	// ErrorStrategy is used to determine how long
	// to delay between poll intervals when attempting
	// to recover from a mongo error.
	//
	// It must not be changed when any watchers are active.
	ErrorStrategy retry.Strategy = retry.Exponential{
		Initial:  txnWatcherErrorShortWait,
		Factor:   2.0,
		MaxDelay: 30 * time.Second,
	}

	// TxnPollNotifyFunc allows tests to be able to specify
	// callbacks each time the database has been polled and processed.
	TxnPollNotifyFunc func()
)

type txnChange struct {
	collection string
	docID      interface{}
	revID      int64
}

// A TxnWatcher watches the txns.log collection and publishes all change events
// to the hub.
type TxnWatcher struct {
	hub    Hub
	clock  Clock
	logger Logger

	tomb           tomb.Tomb
	iteratorFunc   func(*mgo.Collection) mongo.Iterator
	session        *mgo.Session
	jujuDBName     string
	collectionName string

	// notifySync is copied from the package variable when the watcher
	// is created.
	notifySync func()

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

	// lastId is the most recent transaction id observed by a sync.
	lastId interface{}
}

// TxnWatcherConfig contains the configuration parameters required
// for a NewTxnWatcher.
type TxnWatcherConfig struct {
	// Session is used exclusively fot this TxnWatcher.
	Session *mgo.Session
	// JujuDBName is the Juju database name, usually "juju".
	JujuDBName string
	// CollectionName is txn logs collection name, usually "txns.log".
	CollectionName string
	// Hub is where the changes are published to.
	Hub Hub
	// Clock allows tests to control the advancing of time.
	Clock Clock
	// Logger is used to control where the log messages for this watcher go.
	Logger Logger
	// IteratorFunc can be overridden in tests to control what values the
	// watcher sees.
	IteratorFunc func(*mgo.Collection) mongo.Iterator
}

// Validate ensures that all the values that have to be set are set.
func (config TxnWatcherConfig) Validate() error {
	if config.Session == nil {
		return errors.NotValidf("missing Session")
	}
	if config.CollectionName == "" {
		return errors.NotValidf("missing CollectionName")
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
	return nil
}

// New returns a new Watcher observing the changelog collection,
// which must be a capped collection maintained by mgo/txn.
func NewTxnWatcher(config TxnWatcherConfig) (*TxnWatcher, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "new TxnWatcher invalid config")
	}

	w := &TxnWatcher{
		hub:            config.Hub,
		clock:          config.Clock,
		logger:         config.Logger,
		session:        config.Session,
		jujuDBName:     config.JujuDBName,
		collectionName: config.CollectionName,
		iteratorFunc:   config.IteratorFunc,
		notifySync:     TxnPollNotifyFunc,
		reportRequest:  make(chan chan map[string]interface{}),
	}
	if w.iteratorFunc == nil {
		w.iteratorFunc = w.iter
	}
	if w.logger == nil {
		w.logger = noOpLogger{}
	}
	w.tomb.Go(func() error {
		err := w.loop()
		cause := errors.Cause(err)
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		if err != nil && cause != tomb.ErrDying {
			w.logger.Infof("watcher loop failed: %v", err)
		}
		return cause
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

// getTxnLogCollection returns the raw mongodb txns collection.
func (w *TxnWatcher) getTxnLogCollection() *mgo.Collection {
	if w.session.Ping() != nil {
		w.session.Refresh()
	}
	return w.session.DB(w.jujuDBName).C(w.collectionName)
}

// loop implements the main watcher loop.
// period is the delay between each sync.
func (w *TxnWatcher) loop() error {
	w.logger.Tracef("loop started")
	defer w.logger.Tracef("loop finished")
	// Make sure we have read the last ID before telling people
	// we have started.
	logCollection := w.getTxnLogCollection()
	if err := w.initLastId(logCollection); err != nil {
		return errors.Trace(err)
	}

	// Initially we have no retries and will use the
	// polling backoff strategy.
	syncRetryCount := 0
	backoffStrategy := PollStrategy

	// Also make sure we have prepared the timer before
	// we tell people we've started.
	now := w.clock.Now()
	backoff := backoffStrategy.NewTimer(now)
	d, _ := backoff.NextSleep(now)
	next := w.clock.After(d)
	w.hub.Publish(txnWatcherStarting, nil)
	for {
		select {
		case <-w.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case <-next:
			d, ok := backoff.NextSleep(w.clock.Now())
			if !ok {
				// This shouldn't happen, but be defensive.
				backoff = backoffStrategy.NewTimer(w.clock.Now())
			}
			next = w.clock.After(d)
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

		if logCollection == nil {
			// On error the log collection will be set to nil so get it again.
			// This will refresh the mongo session if needed.
			logCollection = w.getTxnLogCollection()
		}
		added, err := w.sync(logCollection)
		if wrench.IsActive("txnwatcher", "sync-error") {
			added = false
			err = errors.New("test sync watcher error")
		}

		if err == nil {
			if syncRetryCount > 0 {
				w.logger.Tracef("txn sync watcher recovered after %d retries", syncRetryCount)
			}
			// Something's happened, so reset the exponential backoff
			// so we'll retry again quickly.
			if syncRetryCount > 0 || added {
				backoff = PollStrategy.NewTimer(w.clock.Now())
				next = w.clock.After(txnWatcherShortWait)
			}
			syncRetryCount = 0
			w.flush()
			if !added && w.notifySync != nil {
				w.notifySync()
			}
		} else {
			w.logger.Warningf("txn watcher sync error: %v\ncurrent retry count %d", err, syncRetryCount)
			if syncRetryCount > maxSyncRetries {
				w.hub.Publish(txnWatcherSyncErr, err)
				return errors.Trace(err)
			}
			logCollection = nil
			syncRetryCount++
			if syncRetryCount == 1 {
				// An error occurred so set up the error retry strategy.
				backoff = ErrorStrategy.NewTimer(w.clock.Now())
				next = w.clock.After(txnWatcherErrorShortWait)
			}
		}
	}
}

// flush sends all pending events to their respective channels.
func (w *TxnWatcher) flush() {
	// refreshEvents are stored newest first.
	for i := len(w.syncEvents) - 1; i >= 0; i-- {
		e := w.syncEvents[i]
		w.hub.Publish(txnWatcherCollection, e)
	}
	w.averageSyncLen = (filterFactor * float64(len(w.syncEvents))) + ((1.0 - filterFactor) * w.averageSyncLen)
	w.syncEventsLastLen = len(w.syncEvents)
	w.syncEvents = w.syncEvents[:0]
	// TODO(jam): 2018-11-07 Consider if averageSyncLen << cap(syncEvents) we should reallocate the buffer, so that it
	// doesn't grow to the size of the largest-ever change and never shrink
}

// initLastId reads the most recent changelog document and initializes
// lastId with it. This causes all history that precedes the creation
// of the watcher to be ignored.
func (w *TxnWatcher) initLastId(log *mgo.Collection) error {
	var entry struct {
		Id interface{} `bson:"_id"`
	}
	err := log.Find(nil).Sort("-$natural").One(&entry)
	if err != nil && err != mgo.ErrNotFound {
		return errors.Trace(err)
	}
	w.lastId = entry.Id
	return nil
}

func (w *TxnWatcher) iter(log *mgo.Collection) mongo.Iterator {
	return log.Find(nil).Batch(10).Sort("-$natural").Iter()
}

// sync updates the watcher knowledge from the database, and
// queues events to observing channels.
func (w *TxnWatcher) sync(log *mgo.Collection) (bool, error) {
	w.logger.Tracef("txn watcher %p starting sync", w)
	added := false
	// Iterate through log events in reverse insertion order (newest first).
	iter := w.iteratorFunc(log)
	seen := make(map[watchKey]bool)
	first := true
	lastId := w.lastId
	var entry bson.D
	for iter.Next(&entry) {
		w.iteratorStepCount++
		if len(entry) == 0 {
			w.logger.Tracef("got empty changelog document")
		}
		id := entry[0]
		if id.Name != "_id" {
			w.logger.Warningf("watcher: _id field isn't first entry")
			continue
		}
		if first {
			w.lastId = id.Value
			first = false
		}
		if id.Value == lastId {
			break
		}
		w.logger.Tracef("%p step %d got changelog document: %#v", w, w.iteratorStepCount, entry)
		for _, c := range entry[1:] {
			// See txn's Runner.ChangeLog for the structure of log entries.
			var d, r []interface{}
			dr, _ := c.Value.(bson.D)
			for _, item := range dr {
				switch item.Name {
				case "d":
					d, _ = item.Value.([]interface{})
				case "r":
					r, _ = item.Value.([]interface{})
				}
			}
			if len(d) == 0 || len(d) != len(r) {
				w.logger.Warningf("changelog has invalid collection document: %#v", c)
				continue
			}
			for i := len(d) - 1; i >= 0; i-- {
				key := watchKey{c.Name, d[i]}
				if seen[key] {
					continue
				}
				seen[key] = true
				revno, ok := r[i].(int64)
				if !ok {
					w.logger.Warningf("changelog has revno with type %T: %#v", r[i], r[i])
					continue
				}
				if revno < 0 {
					revno = -1
				}
				w.syncEvents = append(w.syncEvents, Change{
					C:     c.Name,
					Id:    d[i],
					Revno: revno,
				})
				w.changesCount++
				added = true
			}
		}
	}
	if err := iter.Close(); err != nil {
		return false, errors.Annotate(err, "watcher iteration error")
	}
	return added, nil
}
