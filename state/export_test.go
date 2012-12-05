package state

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

// WatchPrincipalUnits2 returns a UnitsWatcher tracking the machine's principal
// units. The public API still uses a MachinePrincipalUnitsWatcher, which is due
// for retirement.
func (m *Machine) WatchPrincipalUnits2() *UnitsWatcher {
	m = &Machine{m.st, m.doc}
	coll := m.st.machines.Name
	getUnits := func() ([]string, error) {
		if err := m.Refresh(); err != nil {
			return nil, err
		}
		return m.doc.Principals, nil
	}
	return newUnitsWatcher(m.st, getUnits, coll, m.doc.Id, m.doc.TxnRevno)
}
