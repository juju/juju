// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

type RestartConfig struct {
	Factory Factory
	Logger  loggo.Logger
	Clock   clock.Clock
	Delay   time.Duration
}

func (config RestartConfig) Validate() error {
	if config.Factory == nil {
		return errors.NotValidf("nil Factory")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Delay < 0 {
		return errors.NotValidf("non-positive Delay")
	}
	return nil
}

func NewRestartWorkers(config RestartConfig) (*RestartWorkers, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	dw, err := NewDumbWorkers(DumbConfig{
		Factory: config.Factory,
		Logger:  config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	// dw.Stop not deferred, handled in single error branch below.
	// this should change if this constructor grows.

	rw := &RestartWorkers{
		config:  config,
		workers: dw,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &rw.catacomb,
		Work: rw.run,
	})
	if err != nil {
		if stopErr := dw.Stop(); stopErr != nil {
			config.Logger.Errorf("while stopping initial workers: %v", stopErr)
		}
		return nil, errors.Trace(err)
	}
	return rw, nil
}

// RestartWorkers wraps a DumbWorkers and restarts/replaces workers as
// they fail.
type RestartWorkers struct {
	config   RestartConfig
	catacomb catacomb.Catacomb

	// mu protects workers.
	mu      sync.Mutex
	workers *DumbWorkers

	// wg tracks maintainer goroutines.
	wg sync.WaitGroup
}

// TxnLogWatcher is part of the Workers interface.
func (rw *RestartWorkers) TxnLogWatcher() TxnLogWatcher {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.workers.txnLogWorker
}

// PresenceWatcher is part of the Workers interface.
func (rw *RestartWorkers) PresenceWatcher() PresenceWatcher {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.workers.presenceWorker
}

// LeadershipManager is part of the Workers interface.
func (rw *RestartWorkers) LeadershipManager() LeaseManager {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.workers.leadershipWorker
}

// SingularManager is part of the Workers interface.
func (rw *RestartWorkers) SingularManager() LeaseManager {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.workers.singularWorker
}

// Kill is part of the worker.Worker interface, and also part of the
// Workers interface, and luckily the meaning of the former is
// compatible with that of the latter.
func (rw *RestartWorkers) Kill() {
	rw.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (rw *RestartWorkers) Wait() error {
	return rw.catacomb.Wait()
}

// Stop is part of the Workers interface.
func (rw *RestartWorkers) Stop() error {
	return worker.Stop(rw)
}

func (rw *RestartWorkers) run() error {

	replacers := []replacer{
		&txnLogWorkerReplacer{
			start:   rw.config.Factory.NewTxnLogWorker,
			current: rw.workers.txnLogWorker,
			target:  &rw.workers.txnLogWorker,
		},
		&presenceWorkerReplacer{
			start:   rw.config.Factory.NewPresenceWorker,
			current: rw.workers.presenceWorker,
			target:  &rw.workers.presenceWorker,
		},
		&leaseWorkerReplacer{
			start:   rw.config.Factory.NewLeadershipWorker,
			current: rw.workers.leadershipWorker,
			target:  &rw.workers.leadershipWorker,
		},
		&leaseWorkerReplacer{
			start:   rw.config.Factory.NewSingularWorker,
			current: rw.workers.singularWorker,
			target:  &rw.workers.singularWorker,
		},
	}
	for _, replacer := range replacers {
		rw.wg.Add(1)
		go rw.maintain(replacer)
	}

	<-rw.catacomb.Dying()
	rw.wg.Wait()
	return rw.workers.Stop()
}

// maintain drives a replacer. See commentary in func, and docs on
// the replacer interface.
func (rw *RestartWorkers) maintain(replacer replacer) {

	// Signal to the worker that we've stopped trying to maintain
	// a worker once we return from this func.
	defer rw.wg.Done()

	// First, just until the worker actually needs replacement.
	select {
	case <-rw.catacomb.Dying():
		return
	case <-replacer.needed():
	}

	// Then repeatedly try to create a replacement until success.
	for {
		select {
		case <-rw.catacomb.Dying():
			return
		case <-rw.config.Clock.After(rw.config.Delay):
		}
		if replacer.prepare() {
			break
		}
	}

	// Signal to the worker that we'll be maintaining the new
	// worker, effectively undoing the deferred Done above.
	rw.wg.Add(1)

	// Actually replace the worker...
	rw.mu.Lock()
	defer rw.mu.Unlock()
	replacer.replace()

	// ...and start again from the top.
	go rw.maintain(replacer)
}

// replacer exists to satisfy the very narrow constraints of the
// RestartWorkers.maintain method. The methods will be called
// in the order defined, as annotated:
type replacer interface {

	// needed returns a channel that will be closed when the
	// original worker has failed and needs to be restarted;
	// once this has happened...
	needed() <-chan struct{}

	// prepare will then be called repeatedly until it returns
	// true, indicating that it's readied a replacement worker;
	// at which point...
	prepare() bool

	// the workers mutex will be acquired, and it's safe for the
	// replacer to write the new worker to the target pointer
	// (and update its own internal references so that the next
	// call to needed() returns a channel tied to the new worker's
	// lifetime).
	replace()

	// the actual *implementation* of the various kinds of replacer
	// should not vary -- they'd be great candidates for codegen or
	// even generics.
}

// txnLogWorkerReplacer implements replacer. Apart from the types, it
// should be identical to presenceWorkerReplacer and leaseWorkerReplacer.
type txnLogWorkerReplacer struct {
	start   func() (TxnLogWorker, error)
	current TxnLogWorker
	next    TxnLogWorker
	target  *TxnLogWorker
}

func (r *txnLogWorkerReplacer) needed() <-chan struct{} {
	return worker.Dead(r.current)
}

func (r *txnLogWorkerReplacer) prepare() bool {
	var err error
	r.next, err = r.start()
	return err == nil
}

func (r *txnLogWorkerReplacer) replace() {
	*r.target = r.next
	r.current = r.next
	r.next = nil
}

// presenceWorkerReplacer implements replacer. Apart from the types, it
// should be identical to txnLogWorkerReplacer and leaseWorkerReplacer.
type presenceWorkerReplacer struct {
	start   func() (PresenceWorker, error)
	current PresenceWorker
	next    PresenceWorker
	target  *PresenceWorker
}

func (r *presenceWorkerReplacer) needed() <-chan struct{} {
	return worker.Dead(r.current)
}

func (r *presenceWorkerReplacer) prepare() bool {
	var err error
	r.next, err = r.start()
	return err == nil
}

func (r *presenceWorkerReplacer) replace() {
	*r.target = r.next
	r.current = r.next
	r.next = nil
}

// leaseWorkerReplacer implements replacer. Apart from the types, it
// should be identical to presenceWorkerReplacer and txnLogWorkerReplacer.
type leaseWorkerReplacer struct {
	start   func() (LeaseWorker, error)
	current LeaseWorker
	next    LeaseWorker
	target  *LeaseWorker
}

func (r *leaseWorkerReplacer) needed() <-chan struct{} {
	return worker.Dead(r.current)
}

func (r *leaseWorkerReplacer) prepare() bool {
	var err error
	r.next, err = r.start()
	return err == nil
}

func (r *leaseWorkerReplacer) replace() {
	*r.target = r.next
	r.current = r.next
	r.next = nil
}
