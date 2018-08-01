// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/txn"
)

// TransactionChecker values are returned from the various Set*Hooks calls,
// and should be run after the code under test has been executed to check
// that the expected number of transactions were run.
type TransactionChecker func()

func (c TransactionChecker) Check() {
	c()
}

// SetBeforeHooks uses Settxn.TestHooks to queue N functions to be run
// immediately before the next N transactions. The first function is executed
// before the first transaction, the second function before the second
// transaction and so on. Nil values are accepted, and useful, in that they can
// be used to ensure that a transaction is run at the expected time, without
// having to make any changes or assert any state.
func SetBeforeHooks(c *gc.C, runner txn.Runner, fs ...func()) TransactionChecker {
	transactionHooks := make([]txn.TestHook, len(fs))
	for i, f := range fs {
		transactionHooks[i] = txn.TestHook{Before: f}
	}
	return SetTestHooks(c, runner, transactionHooks...)
}

// SetAfterHooks uses Settxn.TestHooks to queue N functions to be run
// immediately after the next N transactions. The first function is executed
// after the first transaction, the second function after the second
// transaction and so on.
func SetAfterHooks(c *gc.C, runner txn.Runner, fs ...func()) TransactionChecker {
	transactionHooks := make([]txn.TestHook, len(fs))
	for i, f := range fs {
		transactionHooks[i] = txn.TestHook{After: f}
	}
	return SetTestHooks(c, runner, transactionHooks...)
}

// SetRetryHooks uses txn.TestHooks to inject a block function designed
// to disrupt a transaction built against recent state, and a check function
// designed to verify that the replacement transaction against the new state
// has been applied as expected.
func SetRetryHooks(c *gc.C, runner txn.Runner, block, check func()) TransactionChecker {
	return SetTestHooks(c, runner, txn.TestHook{
		Before: block,
	}, txn.TestHook{
		After: check,
	})
}

// SetTestHooks queues up hooks to be applied to the next transactions,
// and returns a function that asserts all hooks have been run (and removes any
// that have not). Each hook function can freely execute its own transactions
// without causing other hooks to be triggered.
// It returns a function that asserts that all hooks have been run, and removes
// any that have not. It is an error to set transaction hooks when any are
// already queued; and setting transaction hooks renders the *State goroutine-
// unsafe.
func SetTestHooks(c *gc.C, runner txn.Runner, hooks ...txn.TestHook) TransactionChecker {
	transactionHooks := txn.TestHooks(runner)
	original := <-transactionHooks
	transactionHooks <- hooks
	c.Assert(original, gc.HasLen, 0)
	return func() {
		remaining := <-transactionHooks
		transactionHooks <- nil
		c.Assert(remaining, gc.HasLen, 0)
	}
}
