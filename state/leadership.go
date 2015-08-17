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

// HackLeadership stops the state's internal leadership manager to prevent it
// from interfering with apiserver shutdown.
func (st *State) HackLeadership() {
	// TODO(fwereade): 2015-08-07 lp:1482634
	// obviously, this should not exist: it's a quick hack to address lp:1481368 in
	// 1.24.4, and should be quickly replaced with something that isn't so heinous.
	//
	// But.
	//
	// I *believe* that what it'll take to fix this is to extract the mongo-session-
	// opening from state.Open, so we can create a mongosessioner Manifold on which
	// state, leadership, watching, tools storage, etc etc etc can all independently
	// depend. (Each dependency would/should have a separate session so they can
	// close them all on their own schedule, without panics -- but the failure of
	// the shared component should successfully goose them all into shutting down,
	// in parallel, of their own accord.)
	st.leadershipManager.Kill()
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
