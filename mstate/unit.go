package mstate

// Unit represents the state of a service unit.
type Unit struct {
	st          *State
	name        string
	serviceName string
	principal   string
}

// type unitDoc represents the internal state of a unit in MongoDB.
type unitDoc struct {
	Name        string `bson:"_id"`
	ServiceName string
	Principal   string
}

func newUnit(st *State, udoc *unitDoc) *Unit {
	return &Unit{
		st:          st,
		name:        udoc.Name,
		serviceName: udoc.ServiceName,
		principal:   udoc.Principal,
	}
}

// ServiceName returns the service name.
func (u *Unit) ServiceName() string {
	return u.serviceName
}

// Name returns the unit name.
func (u *Unit) Name() string {
	return u.name
}

// String returns the unit as string.
func (u *Unit) String() string {
	return u.Name()
}

// IsPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate services deployed alongside it.
func (u *Unit) IsPrincipal() bool {
	return u.principal == ""
}
