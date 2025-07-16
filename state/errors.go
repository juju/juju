// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"

	stateerrors "github.com/juju/juju/state/errors"
)

var (
	// State package internal errors.
	applicationNotAliveErr = stateerrors.NewNotAliveError("application")
	unitNotAliveErr        = stateerrors.NewNotAliveError("unit")
)

func onAbort(txnErr, err error) error {
	if txnErr == txn.ErrAborted ||
		errors.Cause(txnErr) == txn.ErrAborted {
		return errors.Trace(err)
	}
	return errors.Trace(txnErr)
}
