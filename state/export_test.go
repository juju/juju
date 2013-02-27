package state

import (
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/charm"
	"time"
)

type (
	CharmDoc    charmDoc
	MachineDoc  machineDoc
	RelationDoc relationDoc
	ServiceDoc  serviceDoc
	UnitDoc     unitDoc
)

func (doc *MachineDoc) String() string {
	m := &Machine{doc: machineDoc(*doc)}
	return m.String()
}

var defaultDialTimeout = dialTimeout

func SetDialTimeout(d time.Duration) {
	if d == 0 {
		dialTimeout = defaultDialTimeout
	} else {
		dialTimeout = d
	}
}

func ServiceSettingsRefCount(st *State, serviceName string, curl *charm.URL) (int, error) {
	key := serviceSettingsKey(serviceName, curl)
	var doc settingsRefsDoc
	if err := st.settingsrefs.FindId(key).One(&doc); err == nil {
		return doc.RefCount, nil
	}
	return 0, mgo.ErrNotFound
}

func init() {
	logSize = logSizeTests
}
