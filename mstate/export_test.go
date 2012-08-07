package mstate

type (
	CharmDoc    charmDoc
	MachineDoc  machineDoc
	RelationDoc relationDoc
	ServiceDoc  serviceDoc
	UnitDoc     unitDoc
)

func NewRelation(st *State, doc *RelationDoc) *Relation {
	return &Relation{st: st, doc: relationDoc(*doc)}
}

func (doc *MachineDoc) String() string {
	m := &Machine{doc: machineDoc(*doc)}
	return m.String()
}

func (u *Unit) MachineId() *int {
	return u.doc.MachineId
}
