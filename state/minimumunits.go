// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/utils"
)

// minimumUnitsDoc allows for keeping track of relevant changes on the
// service MinimumUnits field and on the number of alive units for the service.
// A new document is created when MinimumUnits is set to a non zero value.
// A document is deleted when either the associated service is destroyed
// or MinimumUnits is restored to zero. The MinimumUnitsWatcher reacts to
// changes sending events, each one describing one or more services. A worker
// reacts to those events ensuring the number of units for the service is
// never less than the actual alive units: new units are added if required
// (see EnsureMinimumUnits below).
type minimumUnitsDoc struct {
	// Since the referred entity type is always the Service, it is safe here
	// to use the service name as id in place of its globalKey.
	ServiceName string `bson:"_id"`
	// Revno is increased whenever a service unit is destroyed or a service
	// MinimumUnits is set.
	Revno    int
	TxnRevno int64 `bson:"txn-revno"`
}

// SetMinimumUnits changes the minimum units count for the service.
func (s *Service) SetMinimumUnits(minimumUnits int) (err error) {
	defer utils.ErrorContextf(&err, "cannot set minimum units for service %q", s)
	serviceName := s.doc.Name
	serviceOp := txn.Op{
		C:      s.st.services.Name,
		Id:     serviceName,
		Assert: isAliveDoc,
		Update: D{{"$set", D{{"minimumunits", minimumUnits}}}},
	}
	// Removing the document never fails. Racing clients trying to create the
	// document generate one failure, but the second attempt should succeed.
	// If the referred-to service advanced his life cycle to a not alive state,
	// the second attempt fails and an error is returned.
	for i := 0; i < 2; i++ {
		ops := []txn.Op{serviceOp}
		if count, err := s.st.minimumUnits.FindId(serviceName).Count(); err != nil {
			return err
		} else if count == 0 {
			if minimumUnits == 0 {
				return nil
			}
			if i != 0 {
				return fmt.Errorf("service %s is no longer alive", s.doc.Name)
			}
			ops = append(ops, minimumUnitsInsertOp(s.st, s.doc.Name))
		} else {
			if minimumUnits == 0 {
				ops = append(ops, minimumUnitsRemoveOp(s.st, s.doc.Name))
			} else if minimumUnits > s.doc.MinimumUnits {
				ops = append(ops, minimumUnitsUpdateOp(s.st, s.doc.Name))
			}
		}
		if err := s.st.runTransaction(ops); err == nil {
			log.Debugf("- DEBUG-REMOVEME ---> minimumunits/SetMinimumUnits: SET!")
			s.doc.MinimumUnits = minimumUnits
			return nil
		} else if err != txn.ErrAborted {
			return err
		}
	}
	return ErrExcessiveContention
}

// minimumUnitsInsertOp returns the operation required to insert a minimum
// units document for the service in MongoDB.
func minimumUnitsInsertOp(st *State, serviceName string) txn.Op {
	log.Debugf("- DEBUG-REMOVEME ---> minimumunits/insert")
	return txn.Op{
		C:      st.minimumUnits.Name,
		Id:     serviceName,
		Assert: txn.DocMissing,
		Insert: &minimumUnitsDoc{ServiceName: serviceName},
	}
}

// minimumUnitsIncreaseOp returns the operation required to increase the
// minimum units revno for the service in MongoDB, ignoring the case of
// document not existing. This is included in the operations performed when
// a unit is destroyed: if the document exists, then we need to update the
// Revno. If the service does not require a minimum amount of units, then
// the operation is a noop.
func minimumUnitsIncreaseOp(st *State, serviceName string) txn.Op {
	log.Debugf("- DEBUG-REMOVEME ---> minimumunits/increase")
	return txn.Op{
		C:      st.minimumUnits.Name,
		Id:     serviceName,
		Update: D{{"$inc", D{{"revno", 1}}}},
	}
}

// minimumUnitsUpdateOp returns the operation required to increase the
// minimum units revno for the service in MongoDB. The document must exist.
func minimumUnitsUpdateOp(st *State, serviceName string) txn.Op {
	log.Debugf("- DEBUG-REMOVEME ---> minimumunits/update")
	op := minimumUnitsIncreaseOp(st, serviceName)
	op.Assert = txn.DocExists
	return op
}

// minimumUnitsRemoveOp returns the operation required to remove the minimum
// units document from MongoDB.
func minimumUnitsRemoveOp(st *State, serviceName string) txn.Op {
	log.Debugf("- DEBUG-REMOVEME ---> minimumunits/remove")
	return txn.Op{
		C:      st.minimumUnits.Name,
		Id:     serviceName,
		Remove: true,
	}
}

// MinimumUnits returns the minimum units count for the service.
func (s *Service) MinimumUnits() int {
	return s.doc.MinimumUnits
}

// AliveUnitsCount returns the amount of alive units of the service.
func (s *Service) AliveUnitsCount() (int, error) {
	query := D{{"service", s.doc.Name}, {"life", Alive}}
	alive, err := s.st.units.Find(query).Count()
	if err != nil {
		return 0, fmt.Errorf(
			"cannot get alive units count from service %q: %v", s, err)
	}
	return alive, nil
}

// EnsureMinimumUnits adds new units if the service MinimumUnits value is
// greater than the number of alive units.
func (s *Service) EnsureMinimumUnits() error {
	aliveUnits, err := s.AliveUnitsCount()
	if err != nil {
		return err
	}
	missing := s.MinimumUnits() - aliveUnits
	log.Debugf("- DEBUG-REMOVEME ---> minimumunits/EnsureMinimumUnits: minimum: %v, alive: %v, missing: %v\n", s.MinimumUnits(), aliveUnits, missing)
	if missing <= 0 {
		return nil
	}
	log.Debugf("- DEBUG-REMOVEME ---> minimumunits/EnsureMinimumUnits: %v ADD UNITS!", s.Name())
	for i := 0; i < missing; i++ {
		unit, err := s.AddUnit()
		if err != nil {
			return fmt.Errorf("cannot add unit %d/%d to service %q: %v",
				i+1, missing, s.Name(), err)
		}
		if err := s.st.AssignUnit(unit, AssignNew); err != nil {
			return err
		}
	}
	return nil
}
