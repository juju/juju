package state

import (
	"fmt"

	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/trivial"
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

func init() {
	logSize = logSizeTests
}

// AddUnitSubordinateTo adds a new subordinate unit to the service, subordinate
// to principal. It does not verify relation state sanity or pre-existence of
// other subordinates of the same service; is deprecated; and only continues
// to exist for the convenience of certain tests, which are themselves due for
// overhaul.
func (s *Service) AddUnitSubordinateTo(principal *Unit) (unit *Unit, err error) {
	log.Printf("state: Service.AddUnitSubordinateTo is DEPRECATED; subordinate units should be created only as a side-effect of a principal entering relation scope")
	defer trivial.ErrorContextf(&err, "cannot add unit to service %q as a subordinate of %q", s, principal)
	ch, _, err := s.Charm()
	if err != nil {
		return nil, err
	}
	if !ch.Meta().Subordinate {
		return nil, fmt.Errorf("service is not a subordinate")
	}
	if !principal.IsPrincipal() {
		return nil, fmt.Errorf("unit is not a principal")
	}
	name, ops, err := s.addUnitOps(principal.doc.Name, false)
	if err != nil {
		return nil, err
	}
	if err = s.st.runner.Run(ops, "", nil); err == nil {
		return s.Unit(name)
	} else if err != txn.ErrAborted {
		return nil, err
	}
	if alive, err := isAlive(s.st.services, s.doc.Name); err != nil {
		return nil, err
	} else if !alive {
		return nil, fmt.Errorf("service is not alive")
	}
	if alive, err := isAlive(s.st.units, principal.doc.Name); err != nil {
		return nil, err
	} else if !alive {
		return nil, fmt.Errorf("principal unit is not alive")
	}
	return nil, fmt.Errorf("inconsistent state")
}
