// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2/txn"
	jujutxn "github.com/juju/txn"

	"github.com/juju/juju/core/leadership"
)

func removeLeadershipSettingsOp(applicationId string) txn.Op {
	return removeSettingsOp(settingsC, leadershipSettingsKey(applicationId))
}

func leadershipSettingsKey(applicationId string) string {
	return fmt.Sprintf("a#%s#leader", applicationId)
}

// buildTxnWithLeadership returns a transaction source that combines the supplied source
// with checks and asserts on the supplied token.
func buildTxnWithLeadership(buildTxn jujutxn.TransactionSource, token leadership.Token) jujutxn.TransactionSource {
	return func(attempt int) ([]txn.Op, error) {
		var prereqs []txn.Op
		if err := token.Check(attempt, &prereqs); err != nil {
			return nil, errors.Annotatef(err, "prerequisites failed")
		}
		ops, err := buildTxn(attempt)
		if err == jujutxn.ErrNoOperations {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return append(prereqs, ops...), nil
	}
}
