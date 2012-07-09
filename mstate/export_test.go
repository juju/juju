package mstate

type (
	CharmDoc   struct{ charmDoc }
	MachineDoc struct{ machineDoc }
	ServiceDoc struct{ serviceDoc }
	UnitDoc    struct{ unitDoc }
)

// BUG: this is wrong
func (doc *MachineDoc) String() string {
	m := Machine{id: doc.Id}
	return m.String()
}
