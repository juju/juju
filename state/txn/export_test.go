// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package txn ....
package txn

import (
	stderrors "errors"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	gc "github.com/juju/testing/checkers"
	"labix.org/v2/mgo/txn"
)

var logger = loggo.GetLogger("juju.state.txn")

type TranactionSource func(attempt int) ([]txn.Op, error)

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

type TransactionRunner interface {
	RunTransaction(ops []txn.Op) error
	Run(transactions TranactionSource) error
}

type transactionRunner struct {
	runner           *txn.Runner
	transactionHooks chan ([]TransactionHook)
}

var _ TransactionRunner = (*transactionRunner)(nil)

func NewRunner(runner *txn.Runner) TransactionRunner {
	txnRunner := &transactionRunner{runner: runner}
	txnRunner.transactionHooks = make(chan ([]TransactionHook), 1)
	txnRunner.transactionHooks <- nil
	return txnRunner
}

// TransactionChecker values are returned from the various Set*Hooks calls,
// and should be run after the code under test has been executed to check
// that the expected number of transactions were run.
type TransactionChecker func()

func (c TransactionChecker) Check() {
	c()
}

func SetHooks(c *gc.C, runner TransactionRunner, hooks ...TransactionHook) TransactionChecker {
	tr := runner.(*transactionRunner)
	original := <-tr.transactionHooks
	tr.transactionHooks <- hooks
	c.Assert(original, gc.HasLen, 0)
	return func() {
		remaining := <-tr.transactionHooks
		tr.transactionHooks <- nil
		c.Assert(remaining, gc.HasLen, 0)
	}
}

func (tr *transactionRunner) Run(transactions TranactionSource) error {
	for i := 1; i <= nrRetries; i++ {
		ops, err := transactions(i)
		if err != nil {
			return errors.Annotate(err, "cannot create transactions to run")
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

// RunTransaction runs the supplied operations as a single mgo/txn transaction,
// and includes a mechanism whereby tests can use SetTransactionHooks to induce
// arbitrary state mutations before and after particular transactions.
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

// ResumeTransactions resumes all pending transactions.
func (tr *transactionRunner) ResumeTransactions() error {
	return tr.runner.ResumeAll()
}
