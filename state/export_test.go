package state

import (
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/charm"
	"time"
)

// TestingDialTimeout controls how long calls to state.Open
// will wait during testing.
const TestingDialTimeout = 100 * time.Millisecond

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

func ServiceSettingsRefCount(st *State, serviceName string, curl *charm.URL) (int, error) {
	key := serviceSettingsKey(serviceName, curl)
	var doc settingsRefsDoc
	if err := st.settingsrefs.FindId(key).One(&doc); err == nil {
		return doc.RefCount, nil
	}
	return 0, mgo.ErrNotFound
}

func ServiceRelationCount(svc *Service) int {
	return svc.doc.RelationCount
}

func init() {
	logSize = logSizeTests
}
