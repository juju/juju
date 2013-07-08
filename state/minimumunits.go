// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"errors"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/utils"
)

// minUnitsDoc keeps track of relevant changes on the service's MinUnits field
// and on the number of alive units for the service.
// A new document is created when MinUnits is set to a non zero value.
// A document is deleted when either the associated service is destroyed
// or MinUnits is restored to zero. The Revno is increased when either MinUnits
// for a service is increased or a unit is destroyed.
// TODO(frankban): the MinUnitsWatcher reacts to changes by sending events,
// each one describing one or more services. A worker reacts to those events
// ensuring the number of units for the service is never less than the actual
// alive units: new units are added if required.
type minUnitsDoc struct {
	// ServiceName is safe to be used here in place of its globalKey, since
	// the referred entity type is always the Service.
	ServiceName string `bson:"_id"`
	Revno       int
}

// SetMinUnits changes the number of minimum units required by the service.
func (s *Service) SetMinUnits(minUnits int) (err error) {
	defer utils.ErrorContextf(&err, "cannot set minimum units for service %q", s)
	defer func() {
		if err == nil {
			s.doc.MinUnits = minUnits
		}
	}()
	if minUnits < 0 {
		return errors.New("cannot set a negative minimum number of units")
	}
	service := &Service{st: s.st, doc: s.doc}
	// Removing the document never fails. Racing clients trying to create the
	// document generate one failure, but the second attempt should succeed.
	// If one client tries to update the document, and a racing client removes
	// it, the former should be able to re-create the document in the second
	// attempt. If the referred-to service advanced its life cycle to a not
	// alive state, an error is returned after the first failing attempt.
	for i := 0; i < 2; i++ {
		if service.doc.Life != Alive {
			return errors.New("service is no longer alive")
		}
		if minUnits == service.doc.MinUnits {
			return nil
		}
		ops := setMinUnitsOps(service, minUnits)
		if err := s.st.runTransaction(ops); err != txn.ErrAborted {
			return err
		}
		if err := service.Refresh(); err != nil {
			return err
		}
	}
	return ErrExcessiveContention
}

// setMinUnitsOps returns the operations required to set MinUnits on the
// service and to create/update/remove the minUnits document in MongoDB.
func setMinUnitsOps(service *Service, minUnits int) []txn.Op {
	state := service.st
	serviceName := service.Name()
	ops := []txn.Op{{
		C:      state.services.Name,
		Id:     serviceName,
		Assert: isAliveDoc,
		Update: D{{"$set", D{{"minunits", minUnits}}}},
	}}
	if service.doc.MinUnits == 0 {
		return append(ops, txn.Op{
			C:      state.minUnits.Name,
			Id:     serviceName,
			Assert: txn.DocMissing,
			Insert: &minUnitsDoc{ServiceName: serviceName},
		})
	}
	if minUnits == 0 {
		return append(ops, minUnitsRemoveOp(state, serviceName))
	}
	if minUnits > service.doc.MinUnits {
		op := minUnitsTriggerOp(state, serviceName)
		op.Assert = txn.DocExists
		return append(ops, op)
	}
	return ops
}

// minUnitsTriggerOp returns the operation required to increase the minimum
// units revno for the service in MongoDB, ignoring the case of document not
// existing. This is included in the operations performed when a unit is
// destroyed: if the document exists, then we need to update the Revno.
// If the service does not require a minimum number of units, then the
// operation is a noop.
func minUnitsTriggerOp(st *State, serviceName string) txn.Op {
	return txn.Op{
		C:      st.minUnits.Name,
		Id:     serviceName,
		Update: D{{"$inc", D{{"revno", 1}}}},
	}
}

// minUnitsRemoveOp returns the operation required to remove the minimum
// units document from MongoDB.
func minUnitsRemoveOp(st *State, serviceName string) txn.Op {
	return txn.Op{
		C:      st.minUnits.Name,
		Id:     serviceName,
		Remove: true,
	}
}

// MinUnits returns the minimum units count for the service.
func (s *Service) MinUnits() int {
	return s.doc.MinUnits
}

// EnsureMinUnits adds new units if the service's MinUnits value is greater
// than the number of alive units.
func (s *Service) EnsureMinUnits() (err error) {
	defer utils.ErrorContextf(&err,
		"cannot ensure minimum units for service %q", s)
	service := &Service{st: s.st, doc: s.doc}
	defer func() {
		if err == nil {
			s.doc.MinUnits = service.doc.MinUnits
			s.doc.UnitCount = service.doc.UnitCount
		}
	}()
loop:
	for {
		// Ensure the service is alive.
		if service.doc.Life != Alive {
			return errors.New("service is no longer alive")
		}
		// Exit without errors if the MinUnits for the service is not set.
		if service.doc.MinUnits == 0 {
			return nil
		}
		// Retrieve the number of alive units for the service.
		aliveUnits, err := aliveUnitsCount(service)
		if err != nil {
			return err
		}
		// Calculate the number of required units to be added.
		missing := service.doc.MinUnits - aliveUnits
		if missing <= 0 {
			return nil
		}
		// Add missing units.
		for i := 0; i < missing; i++ {
			name, ops, err := ensureMinUnitsOps(service)
			if err != nil {
				return err
			}
			err = service.st.runTransaction(ops)
			// Refresh the service in two cases: either the transaction was
			// aborted (in which case we also restart the whole loop) or the
			// transaction run correctly but we still need to add more units.
			if err == txn.ErrAborted || (err == nil && i != missing-1) {
				if err := service.Refresh(); err != nil {
					return err
				}
				if err == txn.ErrAborted {
					continue loop
				}
			}
			if err != nil {
				return err
			}
			unit, err := service.Unit(name)
			if err != nil {
				return err
			}
			if err := service.st.AssignUnit(unit, AssignNew); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

// aliveUnitsCount returns the number a alive units for the service.
func aliveUnitsCount(service *Service) (int, error) {
	query := D{{"service", service.doc.Name}, {"life", Alive}}
	aliveUnits, err := service.st.units.Find(query).Count()
	if err != nil {
		return 0, errors.New("cannot get alive units count")
	}
	return aliveUnits, nil
}

// ensureMinUnitsOps returns the operations required to add a unit for the
// service in MongoDB. The operation is aborted if the service document changes
// when running the transaction.
func ensureMinUnitsOps(service *Service) (string, []txn.Op, error) {
	name, ops, err := service.addUnitOps("")
	if err != nil {
		return "", nil, err
	}
	ops = append(ops, txn.Op{
		C:      service.st.services.Name,
		Id:     service.doc.Name,
		Assert: D{{"txn-revno", service.doc.TxnRevno}},
	})
	return name, ops, nil
}
