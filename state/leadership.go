// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/leadership"
)

func removeLeadershipSettingsOp(applicationId string) txn.Op {
	return removeSettingsOp(settingsC, leadershipSettingsKey(applicationId))
}

func leadershipSettingsKey(applicationId string) string {
	return fmt.Sprintf("a#%s#leader", applicationId)
}

// buildTxnWithLeadership returns a transaction source
// that reasserts application leadership continuity.
func buildTxnWithLeadership(buildTxn jujutxn.TransactionSource, token leadership.Token) jujutxn.TransactionSource {
	return func(attempt int) ([]txn.Op, error) {
		if err := token.Check(); err != nil {
			return nil, errors.Annotatef(err, "checking leadership continuity")
		}
		ops, err := buildTxn(attempt)
		if err == jujutxn.ErrNoOperations {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
}
