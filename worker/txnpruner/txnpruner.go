// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txnpruner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"

	jworker "github.com/juju/juju/v3/worker"
)

var logger = loggo.GetLogger("juju.worker.txnpruner")

// TransactionPruner defines the interface for types capable of
// pruning transactions.
type TransactionPruner interface {
	MaybePruneTransactions() error
}

// New returns a worker which periodically prunes the data for
// completed transactions.
func New(tp TransactionPruner, interval time.Duration, clock clock.Clock) worker.Worker {
	return jworker.NewSimpleWorker(func(stopCh <-chan struct{}) error {
		for {
			select {
			case <-clock.After(interval):
				logger.Infof("starting txn pruner")
				start := time.Now()
				err := tp.MaybePruneTransactions()
				if err != nil {
					return errors.Annotate(err, "pruning failed, txnpruner stopping")
				}
				elapsed := time.Since(start)
				logger.Infof("txn pruner completed in %d seconds", elapsed/time.Second)
			case <-stopCh:
				return nil
			}
		}
	})
}
