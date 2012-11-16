package state

import "launchpad.net/juju-core/testing"

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

func init() {
	logSize = logSizeTests
}

// TestingStateInfo returns information suitable for
// connecting to the testing state server.
func TestingStateInfo() *Info {
	return &Info{
		Addrs:       []string{testing.MgoAddr},
		RootCertPEM: []byte(testing.RootCertPEM),
	}
}
