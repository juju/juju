package state

import (
	"fmt"

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
