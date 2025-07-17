// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
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
)

// RunCmdFunc allows tests to override calls to mongo db.Run.
type RunCmdFunc func(db *mgo.Database, cmd any, resp any) error

// A TxnWatcher watches a mongo change stream and publishes all change events
// to the hub.
type TxnWatcher struct {
	hub    Hub
	clock  Clock
	logger logger.Logger

	tomb       tomb.Tomb
	session    *mgo.Session
	jujuDBName string

	reportRequest chan chan map[string]interface{}

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
	Logger logger.Logger
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
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
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
	return nil
}

// Ready waits for the watcher to have started the change stream against mongo or return an error.
// Useful for testing.
func (w *TxnWatcher) Ready() error {
	return nil
}

const (
	FatalChangeStreamError     errors.ConstError = "fatal change stream error"
	ResumableChangeStreamError errors.ConstError = "resumable change stream error"
)
