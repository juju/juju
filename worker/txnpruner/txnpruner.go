// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txnpruner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/worker"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.worker.txnpruner")

// TransactionPruner defines the interface for types capable of
// pruning transactions.
type TransactionPruner interface {
	PruneTransactions() error
}

// New returns a worker which periodically prunes the data for
// completed transactions.
func New(tp TransactionPruner, interval time.Duration) worker.Worker {
	return worker.NewSimpleWorker(func(stopCh <-chan struct{}) error {
		// Use a timer rather than a ticker because pruning could
		// sometimes take a while and we don't want pruning attempts
		// to occur back-to-back.
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				logger.Debugf("starting transaction pruning")
				err := tp.PruneTransactions()
				if err != nil {
					return errors.Annotate(err, "pruning failed, txnpruner stopping")
				}
				logger.Debugf("transaction pruning done")
				timer.Reset(interval)
			case <-stopCh:
				return nil
			}
		}
		return nil
	})
}
