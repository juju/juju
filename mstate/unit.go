package mstate

// unitDoc represents the internal state of a unit in MongoDB.
type unitDoc struct {
	Name    string `bson:"_id"`
	Service string
	UnitSet string
}

// ServiceName returns the service name.
func (u *unitDoc) ServiceName() string {
	return u.Service
}

// String returns the unit as string.
func (u *unitDoc) String() string {
	return u.Name
}

// unitSet represents the internal MongoDB state of a principal unit and
// its subsidiaries.
type unitSet struct {
	Principal string `bson:"_id"`
}

// Unit represents the state of a service unit.
type Unit struct {
	st *State
	*unitDoc
	*unitSet
}

func newUnit(st *State, udoc *unitDoc) *Unit {
	uset := &unitSet{Principal: udoc.UnitSet}
	return &Unit{
		st:      st,
		unitDoc: udoc,
		unitSet: uset,
	}
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return u.unitDoc.Name
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.unitDoc.Name == u.unitSet.Principal
}
