// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"bytes"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/watcher"
)

var errPoolClosed = errors.New("pool closed")

// PoolHelper describes methods for working with a pool-supplied state.
type PoolHelper interface {
	Release() bool
	Annotate(string)
}

// PooledState is a wrapper for a State reference, indicating that it is
// managed by a pool.
type PooledState struct {
	*State

	pool          *StatePool
	modelUUID     string
	isSystemState bool
	released      bool
	itemKey       uint64
}

var _ PoolHelper = (*PooledState)(nil)

func newPooledState(st *State, pool *StatePool, modelUUID string, isSystemState bool) *PooledState {
	return &PooledState{
		State:         st,
		pool:          pool,
		modelUUID:     modelUUID,
		isSystemState: isSystemState,
		released:      false,
	}
}

// Release indicates that the pooled state is no longer required
// and can be removed from the pool if there are no other references
// to it.
// The return indicates whether the released state was actually removed
// from the pool - items marked for removal are only removed when released
// by all other reference holders.
func (ps *PooledState) Release() bool {
	if ps.isSystemState || ps.released {
		return false
	}

	removed, err := ps.pool.release(ps.modelUUID, ps.itemKey)
	if err != nil {
		logger.Errorf("releasing state back to pool: %s", err.Error())
	}
	ps.released = true
	return removed
}

// TODO: implement Close that hides the state.Close for a PooledState?

// Annotate writes the supplied context information back to the pool item.
// The information is stored against the unique ID for the referer,
// indicated by the itemKey member.
func (ps *PooledState) Annotate(context string) {
	// TODO...
}

// PoolItem tracks the usage of a State instance unique to a model.
// It associates context information about state usage for each reference
// holder by associating it with a unique key.
// It tracks whether the state has been marked for removal from the pool.
type PoolItem struct {
	state            *State
	modelUUID        string
	referenceSources map[uint64]string
	remove           bool
}

func (i *PoolItem) refCount() int {
	return len(i.referenceSources)
}

// StatePool is a cache of State instances for multiple
// models. Clients should call Release when they have finished with any
// state.
type StatePool struct {
	systemState *State
	// mu protects pool
	mu      sync.Mutex
	pool    map[string]*PoolItem
	closing bool
	// sourceKey is used to provide a unique number as a key for the
	// referencesSources structure in the pool.
	sourceKey uint64

	// hub is used to pass the transaction changes from the TxnWatcher
	// to the various HubWatchers that are used in each state object created
	// by the state pool.
	hub *pubsub.SimpleHub

	// watcherRunner makes sure the TxnWatcher stays running.
	watcherRunner *worker.Runner
	// txnWatcherSession is used exclusively for the TxnWatcher.
	txnWatcherSession *mgo.Session
}

// OpenStatePool returns a new StatePool instance.
func OpenStatePool(args OpenParams) (_ *StatePool, err error) {
	logger.Tracef("opening state pool")
	if err = args.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating args")
	}

	pool := &StatePool{
		pool: make(map[string]*PoolItem),
		hub:  pubsub.NewSimpleHub(nil),
	}

	session := args.MongoSession.Copy()
	st, err := open(
		args.ControllerModelTag,
		session,
		args.InitDatabaseFunc,
		nil,
		args.NewPolicy,
		args.Clock,
		args.RunTransactionObserver,
	)
	if err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			if closeErr := st.Close(); closeErr != nil {
				logger.Errorf("closing State for %s: %v", args.ControllerModelTag, closeErr)
			}
		}
	}()
	// If the InitDatabaseFunc is set, then we are initializing the
	// database, and the model won't be there. So we only look for the model
	// if we aren't initializing.
	if args.InitDatabaseFunc == nil {
		if _, err = st.Model(); err != nil {
			return nil, errors.Trace(mongo.MaybeUnauthorizedf(err, "cannot read model %s", args.ControllerModelTag.Id()))
		}
	}
	if err = st.start(args.ControllerTag, pool.hub); err != nil {
		return nil, errors.Trace(err)
	}
	pool.systemState = st
	// When creating the txn watchers and the worker to keep it running
	// we really want to use wall clocks. Otherwise the events never get
	// noticed. The clocks in the runner and the txn watcher are used to
	// control polling, and never return the actual times.
	pool.watcherRunner = worker.NewRunner(worker.RunnerParams{
		// TODO add a Logger parameter to RunnerParams:
		// Logger: loggo.GetLogger(logger.Name() + ".txnwatcher"),
		IsFatal:      func(err error) bool { return errors.Cause(err) == errPoolClosed },
		RestartDelay: time.Second,
		Clock:        args.Clock,
	})
	pool.txnWatcherSession = args.MongoSession.Copy()
	if err = pool.watcherRunner.StartWorker(txnLogWorker, func() (worker.Worker, error) {
		return watcher.NewTxnWatcher(
			watcher.TxnWatcherConfig{
				Session:        pool.txnWatcherSession,
				JujuDBName:     jujuDB,
				CollectionName: txnLogC,
				Hub:            pool.hub,
				Clock:          args.Clock,
				Logger:         loggo.GetLogger("juju.state.pool.txnwatcher"),
			})
	}); err != nil {
		pool.txnWatcherSession.Close()
		return nil, errors.Trace(err)
	}
	return pool, nil
}

// Clock returns the clock used by the system state.
func (p *StatePool) Clock() clock.Clock {
	return p.systemState.clock()
}

// Get returns a PooledState for a given model, creating a new State instance
// if required.
// If the State has been marked for removal, an error is returned.
func (p *StatePool) Get(modelUUID string) (*PooledState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pool == nil {
		return nil, errors.New("pool is closed")
	}

	if modelUUID == p.systemState.ModelUUID() {
		return newPooledState(p.systemState, p, modelUUID, true), nil
	}

	item, ok := p.pool[modelUUID]
	if ok && item.remove {
		// Disallow further usage of a pool item marked for removal.
		return nil, errors.NewNotFound(nil, fmt.Sprintf("model %v has been removed", modelUUID))
	}

	p.sourceKey++
	key := p.sourceKey

	source := string(debug.Stack())

	// Already have a state in the pool for this model; use it.
	if ok {
		item.referenceSources[key] = source
		ps := newPooledState(item.state, p, modelUUID, false)
		ps.itemKey = key
		return ps, nil
	}

	// Don't create any new pool objects if the state pool is in the
	// process of closing down.
	if p.closing {
		return nil, errors.New("pool is closing")
	}

	// We need a new state and pool item.
	st, err := p.openState(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	p.pool[modelUUID] = &PoolItem{
		modelUUID: modelUUID,
		state:     st,
		referenceSources: map[uint64]string{
			key: source,
		},
	}
	ps := newPooledState(st, p, modelUUID, false)
	ps.itemKey = key
	return ps, nil
}

func (p *StatePool) openState(modelUUID string) (*State, error) {
	modelTag := names.NewModelTag(modelUUID)
	session := p.systemState.session.Copy()
	newSt, err := newState(
		modelTag, p.systemState.controllerModelTag,
		session, p.systemState.newPolicy, p.systemState.stateClock,
		p.systemState.runTransactionObserver,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := newSt.start(p.systemState.controllerTag, p.hub); err != nil {
		return nil, errors.Trace(err)
	}
	return newSt, nil
}

// GetModel is a convenience method for getting a Model for a State.
func (p *StatePool) GetModel(modelUUID string) (*Model, PoolHelper, error) {
	ps, err := p.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	model, err := ps.Model()
	if err != nil {
		ps.Release()
		return nil, nil, errors.Trace(err)
	}

	return model, ps, nil
}

// release indicates that the client has finished using the State. If the
// state has been marked for removal, it will be closed and removed
// when the final Release is done; if there are no references, it will be
// closed and removed immediately. The boolean result reports whether or
// not the state was closed and removed.
func (p *StatePool) release(modelUUID string, key uint64) (bool, error) {
	if modelUUID == p.systemState.ModelUUID() {
		// We do not monitor usage of the controller's state.
		return false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pool == nil {
		return false, errors.New("pool is closed")
	}

	item, ok := p.pool[modelUUID]
	if !ok {
		return false, errors.Errorf("unable to return unknown model %v to the pool", modelUUID)
	}
	if item.refCount() == 0 {
		return false, errors.Errorf("state pool refcount for model %v is already 0", modelUUID)
	}
	delete(item.referenceSources, key)
	return p.maybeRemoveItem(item)
}

// Remove takes the state out of the pool and closes it, or marks it
// for removal if it's currently being used (indicated by Gets without
// corresponding Releases). The boolean result indicates whether or
// not the state was removed.
func (p *StatePool) Remove(modelUUID string) (bool, error) {
	if modelUUID == p.systemState.ModelUUID() {
		// We do not monitor usage of the controller's state.
		return false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pool == nil {
		return false, errors.New("pool is closed")
	}

	item, ok := p.pool[modelUUID]
	if !ok {
		// Don't require the client to keep track of what we've seen -
		// ignore unknown model uuids.
		return false, nil
	}
	item.remove = true
	return p.maybeRemoveItem(item)
}

func (p *StatePool) maybeRemoveItem(item *PoolItem) (bool, error) {
	if item.remove && item.refCount() == 0 {
		delete(p.pool, item.modelUUID)
		return true, item.state.Close()
	}
	return false, nil
}

// SystemState returns the State passed in to NewStatePool.
func (p *StatePool) SystemState() *State {
	p.mu.Lock()
	defer p.mu.Unlock()
	// State pool is closed, no more access to the system state.
	if p.pool == nil {
		return nil
	}
	return p.systemState
}

// Close closes all State instances in the pool.
func (p *StatePool) Close() error {
	p.mu.Lock()
	// A nil pool map indicates that the pool has already been closed.
	if p.pool == nil {
		p.mu.Unlock()
		return nil
	}
	logger.Tracef("state pool closed from:\n%s", debug.Stack())

	// Before we go through and close the state pool objects, we need to
	// stop all the workers running in the state objects. If anyone had asked
	// for a mutliwatcher, an all watcher worker is stated in the state object's
	// workers. These need to be stopped before we start closing connections
	// as those workers use the pool.
	p.closing = true
	p.mu.Unlock()

	for uuid, item := range p.pool {
		if err := item.state.stopWorkers(); err != nil {
			logger.Infof("state workers for model %s did not stop: %v", uuid, err)
		}
	}
	if err := p.systemState.stopWorkers(); err != nil {
		logger.Infof("state workers for controller model did not stop: %v", err)
	}

	// Reacquire the lock to modify the pool.
	// Hopefully by now any workers running that may have released objects
	// to the pool should be fine.
	p.mu.Lock()
	pool := p.pool
	p.pool = nil
	var lastErr error
	// We release the lock as we are closing the state objects to allow
	// other goroutines that may be attempting to Get a model from the pool
	// to continue. The Get method will fail with a closed pool.
	// We do this just in case the workers didn't stop above when we were trying.
	p.mu.Unlock()
	for _, item := range pool {
		if item.refCount() != 0 || item.remove {
			logger.Warningf(
				"state for %v leaked from pool - references: %v, removed: %v",
				item.state.ModelUUID(),
				item.refCount(),
				item.remove,
			)
		}
		err := item.state.Close()
		if err != nil {
			lastErr = err
		}
	}
	p.mu.Lock()
	if p.watcherRunner != nil {
		_ = worker.Stop(p.watcherRunner)
		p.txnWatcherSession.Close()
	}
	p.mu.Unlock()
	// As with above and the other watchers. Unlock while releas
	if err := p.systemState.Close(); err != nil {
		lastErr = err
	}
	return errors.Annotate(lastErr, "at least one error closing a state")
}

// IntrospectionReport produces the output for the introspection worker
// in order to look inside the state pool.
func (p *StatePool) IntrospectionReport() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	removeCount := 0
	buff := &bytes.Buffer{}

	for uuid, item := range p.pool {
		if item.remove {
			removeCount++
		}
		fmt.Fprintf(buff, "\nModel: %s\n", uuid)
		fmt.Fprintf(buff, "  Marked for removal: %v\n", item.remove)
		fmt.Fprintf(buff, "  Reference count: %v\n", item.refCount())
		index := 0
		for _, ref := range item.referenceSources {
			index++
			fmt.Fprintf(buff, "    [%d]\n%s\n", index, ref)
		}
		item.state.workers.Runner.Report()
	}

	return fmt.Sprintf(""+
		"Model count: %d models\n"+
		"Marked for removal: %d models\n"+
		"\n%s", len(p.pool), removeCount, buff)
}

// Report conforms to the Dependency Engine Report() interface, giving an opportunity to introspect
// what is going on at runtime.
func (p *StatePool) Report() map[string]interface{} {
	p.mu.Lock()
	report := make(map[string]interface{})
	report["txn-watcher"] = p.watcherRunner.Report()
	report["system"] = p.systemState.Report()
	report["pool-size"] = len(p.pool)
	for uuid, item := range p.pool {
		modelReport := item.state.Report()
		modelReport["ref-count"] = item.refCount()
		modelReport["to-remove"] = item.remove
		report[uuid] = modelReport
	}
	p.mu.Unlock()
	return report
}
