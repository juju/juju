// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txnpruner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/worker"
)

// TransactionPruner defines the interface for types capable of
// pruning transactions.
type TransactionPruner interface {
	MaybePruneTransactions() error
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
				err := tp.MaybePruneTransactions()
				if err != nil {
					return errors.Annotate(err, "pruning failed, txnpruner stopping")
				}
				timer.Reset(interval)
			case <-stopCh:
				return nil
			}
		}
	})
}
