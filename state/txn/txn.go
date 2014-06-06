// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package txn provides a TransactionRunner, which applies operations as part
// of a transaction onto any number of collections within a database.
// The execution of the operations is delegated to a mgo/txn/Runner.
// The purpose of the TransactionRunner is to execute the operations multiple
// times in there is a TxnAborted error, in the expectation that subsequent
// attempts will be successful.
// Also included is a mechanism whereby tests can use SetTransactionHooks to induce
// arbitrary state mutations before and after particular transactions.

package txn

import (
	stderrors "errors"

	"github.com/juju/loggo"
	"labix.org/v2/mgo/txn"
)

var logger = loggo.GetLogger("juju.state.txn")

// TransactionHook holds a pair of functions to be called before and after a
// mgo/txn transaction is run. It is only used in testing.
type TransactionHook struct {
	Before func()
	After  func()
}

const (
	nrRetries = 3
)

var ErrExcessiveContention = stderrors.New("state changing too quickly; try again soon")

type tranactionSource func(attempt int) ([]txn.Op, error)

// TransactionRunner instances applies operations to collections in a database.
type TransactionRunner interface {
	// RunTransaction applies the specified transaction operations to a database.
	RunTransaction(ops []txn.Op) error

	// Run calls the nominated function to get the transaction operations to apply to a database.
	// If there is a failure due to a txn.ErrAborted error, the attempt is retried up to nrRetries times.
	Run(transactions tranactionSource) error

	// ResumeTransactions resumes all pending transactions.
	ResumeTransactions() error
}

type transactionRunner struct {
	runner           *txn.Runner
	transactionHooks chan ([]TransactionHook)
}

var _ TransactionRunner = (*transactionRunner)(nil)

// NewRunner returns a TransactionRunner which delegates to the specified txn.Runner.
func NewRunner(runner *txn.Runner) TransactionRunner {
	txnRunner := &transactionRunner{runner: runner}
	txnRunner.transactionHooks = make(chan ([]TransactionHook), 1)
	txnRunner.transactionHooks <- nil
	return txnRunner
}

// Run is defined on TransactionRunner.
func (tr *transactionRunner) Run(transactions tranactionSource) error {
	for i := 1; i <= nrRetries; i++ {
		ops, err := transactions(i)
		if err == ErrExcessiveContention {
			continue
		}
		if err != nil {
			return err
		}
		if len(ops) == 0 {
			return nil
		}
		if err := tr.RunTransaction(ops); err == nil {
			return nil
		} else if err != txn.ErrAborted {
			return err
		}
	}
	return ErrExcessiveContention
}

// RunTransaction is defined on TransactionRunner.
func (tr *transactionRunner) RunTransaction(ops []txn.Op) error {
	transactionHooks := <-tr.transactionHooks
	tr.transactionHooks <- nil
	if len(transactionHooks) > 0 {
		// Note that this code should only ever be triggered
		// during tests. If we see the log messages below
		// in a production run, something is wrong.
		defer func() {
			if transactionHooks[0].After != nil {
				logger.Infof("transaction 'after' hook start")
				transactionHooks[0].After()
				logger.Infof("transaction 'after' hook end")
			}
			if <-tr.transactionHooks != nil {
				panic("concurrent use of transaction hooks")
			}
			tr.transactionHooks <- transactionHooks[1:]
		}()
		if transactionHooks[0].Before != nil {
			logger.Infof("transaction 'before' hook start")
			transactionHooks[0].Before()
			logger.Infof("transaction 'before' hook end")
		}
	}
	return tr.runner.Run(ops, "", nil)
}

// ResumeTransactions is defined on TransactionRunner.
func (tr *transactionRunner) ResumeTransactions() error {
	return tr.runner.ResumeAll()
}
