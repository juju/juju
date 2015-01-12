package state

import "gopkg.in/mgo.v2/txn"

const settingsKey = "leadership-settings-"

func addLeadershipSettingsOp(serviceId string) txn.Op {
	return txn.Op{
		C:      settingsC,
		Id:     LeadershipSettingsDocId(serviceId),
		Insert: make(map[string]interface{}),
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
	return settingsKey + serviceId
}
