package state

import (
	"fmt"

	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

const settingsKey = "s#%s#leader"

func addLeadershipSettingsOp(serviceId string) txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     LeadershipSettingsDocId(serviceId),
		Insert: bson.D{},
		Assert: txn.DocMissing,
	}
}

func removeLeadershipSettingsOp(serviceId string) txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     LeadershipSettingsDocId(serviceId),
		Remove: true,
	}
}

func LeadershipSettingsDocId(serviceId string) string {
	return fmt.Sprintf(settingsKey, serviceId)
}
