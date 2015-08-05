package state

import (
	"fmt"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/leadership"
)

const settingsKey = "s#%s#leader"

func addLeadershipSettingsOp(serviceId string) txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     leadershipSettingsDocId(serviceId),
		Insert: bson.D{},
		Assert: txn.DocMissing,
	}
}

func removeLeadershipSettingsOp(serviceId string) txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     leadershipSettingsDocId(serviceId),
		Remove: true,
	}
}

func leadershipSettingsDocId(serviceId string) string {
	return fmt.Sprintf(settingsKey, serviceId)
}

// LeadershipClaimer returns a leadership.Claimer for units and services in the
// state's environment.
func (st *State) LeadershipClaimer() leadership.Claimer {
	return st.leadershipManager
}

// LeadershipChecker returns a leadership.Checker for units and services in the
// state's environment.
func (st *State) LeadershipChecker() leadership.Checker {
	return st.leadershipManager
}

// buildTxnWithLeadership returns a transaction source that combines the supplied source
// with checks and asserts on the supplied token.
func buildTxnWithLeadership(buildTxn jujutxn.TransactionSource, token leadership.Token) jujutxn.TransactionSource {
	return func(attempt int) ([]txn.Op, error) {
		var prereqs []txn.Op
		if err := token.Check(&prereqs); err != nil {
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
