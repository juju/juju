// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

// DumbConfig holds a DumbWorkers' dependencies.
type DumbConfig struct {
	Factory Factory
	Logger  loggo.Logger
}

// Validate returns an error if config cannot drive a DumbWorkers.
func (config DumbConfig) Validate() error {
	if config.Factory == nil {
		return errors.NotValidf("nil Factory")
	}
	if config.Logger == (loggo.Logger{}) {
		return errors.NotValidf("uninitialized Logger")
	}
	return nil
}

// NewDumbWorkers returns a worker that will live until Kill()ed,
// giving access to a set of sub-workers needed by the state package.
//
// These workers may die of their own accord at any time, and will
// not be replaced; they will also all be stopped before Wait returns.
func NewDumbWorkers(config DumbConfig) (_ *DumbWorkers, err error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	logger := config.Logger

	w := &DumbWorkers{config: config}
	defer func() {
		if err == nil {
			return
		}
		// this is ok because cleanup can handle nil fields
		if cleanupErr := w.cleanup(); cleanupErr != nil {
			logger.Errorf("while aborting DumbWorkers creation: %v", cleanupErr)
		}
	}()

	logger.Debugf("starting leadership lease manager")
	w.leadershipWorker, err = config.Factory.NewLeadershipWorker()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create leadership lease manager")
	}

	logger.Debugf("starting singular lease manager")
	w.singularWorker, err = config.Factory.NewSingularWorker()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create singular lease manager")
	}

	logger.Debugf("starting transaction log watcher")
	w.txnLogWorker, err = config.Factory.NewTxnLogWorker()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create transaction log watcher")
	}

	logger.Debugf("starting presence watcher")
	w.presenceWorker, err = config.Factory.NewPresenceWorker()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create presence watcher")
	}

	// note that we specifically *don't* want to use catacomb's
	// worker-tracking features like Add and Init, because we want
	// this type to live until externally killed, regardless of the
	// state of the inner workers. We're just using catacomb because
	// it's slightly safer than tomb.
	err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.run,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// DumbWorkers holds references to standard state workers. The workers
// are not guaranteed to be running; but they are guaranteed to be
// stopped when the DumbWorkers is stopped.
type DumbWorkers struct {
	config           DumbConfig
	catacomb         catacomb.Catacomb
	txnLogWorker     TxnLogWorker
	presenceWorker   PresenceWorker
	leadershipWorker LeaseWorker
	singularWorker   LeaseWorker
}

// TxnLogWatcher is part of the Workers interface.
func (dw *DumbWorkers) TxnLogWatcher() TxnLogWatcher {
	return dw.txnLogWorker
}

// PresenceWatcher is part of the Workers interface.
func (dw *DumbWorkers) PresenceWatcher() PresenceWatcher {
	return dw.presenceWorker
}

// LeadershipManager is part of the Workers interface.
func (dw *DumbWorkers) LeadershipManager() LeaseManager {
	return dw.leadershipWorker
}

// SingularManager is part of the Workers interface.
func (dw *DumbWorkers) SingularManager() LeaseManager {
	return dw.singularWorker
}

// Kill is part of the worker.Worker interface.
func (dw *DumbWorkers) Kill() {
	dw.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (dw *DumbWorkers) Wait() error {
	return dw.catacomb.Wait()
}

func (dw *DumbWorkers) run() error {
	<-dw.catacomb.Dying()
	if err := dw.cleanup(); err != nil {
		return errors.Trace(err)
	}
	return dw.catacomb.ErrDying()
}

func (dw *DumbWorkers) cleanup() error {
	var errs []error
	handle := func(name string, w worker.Worker) {
		if w == nil {
			return
		}
		if err := worker.Stop(w); err != nil {
			errs = append(errs, errors.Annotatef(err, "error stopping %s", name))
		}
	}

	handle("transaction log watcher", dw.txnLogWorker)
	handle("presence watcher", dw.presenceWorker)
	handle("leadership lease manager", dw.leadershipWorker)
	handle("singular lease manager", dw.singularWorker)
	if len(errs) > 0 {
		for _, err := range errs[1:] {
			dw.config.Logger.Errorf("while stopping state workers: %v", err)
		}
		return errs[0]
	}

	dw.config.Logger.Debugf("stopped state workers without error")
	return nil
}
