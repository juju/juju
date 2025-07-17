// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn/v3"
)

type multiModelRunner struct {
}

// RunTransaction is part of the jujutxn.Runner interface. Operations
// that affect multi-model collections will be modified to
// ensure correct interaction with these collections.
func (r *multiModelRunner) RunTransaction(tx *jujutxn.Transaction) error {
	return nil
}

// Run is part of the jujutxn.Runner interface. Operations returned by
// the given "transactions" function that affect multi-model
// collections will be modified to ensure correct interaction with
// these collections.
func (r *multiModelRunner) Run(transactions jujutxn.TransactionSource) error {
	return nil
}

// ResumeTransactions is part of the jujutxn.Runner interface.
func (r *multiModelRunner) ResumeTransactions() error {
	return errors.NotImplemented
}

// MaybePruneTransactions is part of the jujutxn.Runner interface.
func (r *multiModelRunner) MaybePruneTransactions(opts jujutxn.PruneOptions) error {
	return errors.NotImplemented
}
