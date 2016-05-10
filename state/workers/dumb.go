// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"
	"github.com/juju/juju/worker"
	"github.com/juju/loggo"
)

type DumbConfig struct {
	Factory Factory
	Logger  loggo.Logger
}

func (config DumbConfig) Validate() error {
	if config.Factory == nil {
		return errors.NotValidf("nil Factory")
	}
	return nil
}

func NewDumbWorkers(config DumbConfig) (_ *DumbWorkers, err error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	logger := config.Logger

	result := &DumbWorkers{config: config}
	defer func() {
		if err != nil {
			if stopErr := result.Stop(); stopErr != nil {
				logger.Errorf("while aborting DumbWorkers creation: %v", err)
			}
		}
	}()

	logger.Debugf("starting leadership lease manager")
	result.leadershipWorker, err = config.Factory.NewLeadershipWorker()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create leadership lease manager")
	}

	logger.Debugf("starting singular lease manager")
	result.singularWorker, err = config.Factory.NewSingularWorker()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create singular lease manager")
	}

	logger.Debugf("starting transaction log watcher")
	result.txnLogWorker, err = config.Factory.NewTxnLogWorker()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create transaction log watcher")
	}

	logger.Debugf("starting presence watcher")
	result.presenceWorker, err = config.Factory.NewPresenceWorker()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create presence watcher")
	}

	return result, nil
}

// DumbWorkers holds references to standard state workers.
type DumbWorkers struct {
	config           DumbConfig
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

// Stop is part of the Workers interface.
func (dw *DumbWorkers) Stop() error {
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

// Kill is part of the Workers interface.
func (dw *DumbWorkers) Kill() {
	dw.leadershipWorker.Kill()
	dw.singularWorker.Kill()
}
