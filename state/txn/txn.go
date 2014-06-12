// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package txn provides a Runner, which applies operations as part
// of a transaction onto any number of collections within a database.
// The execution of the operations is delegated to a mgo/txn/Runner.
// The purpose of the Runner is to execute the operations multiple
// times in there is a TxnAborted error, in the expectation that subsequent
// attempts will be successful.
// Also included is a mechanism whereby tests can use SetTestHooks to induce
// arbitrary state mutations before and after particular transactions.

package txn

import (
	stderrors "errors"

	"github.com/juju/loggo"
	"labix.org/v2/mgo/txn"
)

var logger = loggo.GetLogger("juju.state.txn")

const (
	nrRetries = 3
)

var (
	// ErrExcessiveContention is used to signal that even after retrying, the transaction operations
	// could not be successfully applied due to database contention.
	ErrExcessiveContention = stderrors.New("state changing too quickly; try again soon")

	// ErrNoTransactions is returned by TransactionSource implementations to signal that
	// no transaction operations are available to run.
	ErrNoTransactions = stderrors.New("no transaction operations are available")

	// ErrNoTransactions is returned by TransactionSource implementations to signal that
	// the transaction list could not be built but the caller should retry.
	ErrTransientFailure = stderrors.New("transient failure")
)

// TransactionSource defines a function that can return transaction operations to run.
type TransactionSource func(attempt int) ([]txn.Op, error)

// Runner instances applies operations to collections in a database.
type Runner interface {
	// RunTransaction applies the specified transaction operations to a database.
	RunTransaction(ops []txn.Op) error

	// Run calls the nominated function to get the transaction operations to apply to a database.
	// If there is a failure due to a txn.ErrAborted error, the attempt is retried up to nrRetries times.
	Run(transactions TransactionSource) error

	// ResumeTransactions resumes all pending transactions.
	ResumeTransactions() error
}

type transactionRunner struct {
	runner    *txn.Runner
	testHooks chan ([]TestHook)
}

var _ Runner = (*transactionRunner)(nil)

// NewRunner returns a Runner which delegates to the specified txn.Runner.
func NewRunner(runner *txn.Runner) Runner {
	txnRunner := &transactionRunner{runner: runner}
	txnRunner.testHooks = make(chan ([]TestHook), 1)
	txnRunner.testHooks <- nil
	return txnRunner
}

// Run is defined on Runner.
func (tr *transactionRunner) Run(transactions TransactionSource) error {
	for i := 0; i < nrRetries; i++ {
		ops, err := transactions(i)
		if err == ErrTransientFailure {
			continue
		}
		if err == ErrNoTransactions {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tr.RunTransaction(ops); err == nil {
			return nil
		} else if err != txn.ErrAborted {
			return err
		}
	}
	return ErrExcessiveContention
}

// RunTransaction is defined on Runner.
func (tr *transactionRunner) RunTransaction(ops []txn.Op) error {
	testHooks := <-tr.testHooks
	tr.testHooks <- nil
	if len(testHooks) > 0 {
		// Note that this code should only ever be triggered
		// during tests. If we see the log messages below
		// in a production run, something is wrong.
		defer func() {
			if testHooks[0].After != nil {
				logger.Infof("transaction 'after' hook start")
				testHooks[0].After()
				logger.Infof("transaction 'after' hook end")
			}
			if <-tr.testHooks != nil {
				panic("concurrent use of transaction hooks")
			}
			tr.testHooks <- testHooks[1:]
		}()
		if testHooks[0].Before != nil {
			logger.Infof("transaction 'before' hook start")
			testHooks[0].Before()
			logger.Infof("transaction 'before' hook end")
		}
	}
	return tr.runner.Run(ops, "", nil)
}

// ResumeTransactions is defined on Runner.
func (tr *transactionRunner) ResumeTransactions() error {
	return tr.runner.ResumeAll()
}

// TestHook holds a pair of functions to be called before and after a
// mgo/txn transaction is run.
// Exported only for testing.
type TestHook struct {
	Before func()
	After  func()
}

// TestHooks returns the test hooks for a transaction runner.
// Exported only for testing.
func TestHooks(runner Runner) chan ([]TestHook) {
	return runner.(*transactionRunner).testHooks
}
