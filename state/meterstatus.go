// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var meterStatusLogger = loggo.GetLogger("juju.state.meterstatus")

// MeterStatusCode represents the meter status code of a unit.
type MeterStatusCode string

const (
	MeterNotSet       MeterStatusCode = "NOT SET"
	MeterNotAvailable MeterStatusCode = "NOT AVAILABLE"
	MeterGreen        MeterStatusCode = "GREEN"
	MeterAmber        MeterStatusCode = "AMBER"
	MeterRed          MeterStatusCode = "RED"
)

type meterStatusDoc struct {
	Id   string          `bson:"_id"`
	Code MeterStatusCode `bson:"code"`
	Info string          `bson:"info"`
}

// SetMeterStatus sets the meter status for the unit.
func (u *Unit) SetMeterStatus(codeRaw, info string) error {
	code := MeterStatusCode(codeRaw)
	switch code {
	case MeterGreen, MeterAmber, MeterRed:
	default:
		return errors.Errorf("invalid meter status %q", code)
	}
	meterDoc, err := u.getMeterStatusDoc()
	if err != nil {
		return errors.Annotatef(err, "cannot update meter status for unit %s", u.Name())
	}
	if meterDoc.Code == code && meterDoc.Info == info {
		return nil
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := u.Refresh()
			if err != nil {
				return nil, errors.Trace(err)
			}
			meterDoc, err = u.getMeterStatusDoc()
			if err != nil {
				return nil, errors.Annotatef(err, "cannot update meter status for unit %s", u.Name())
			}
			if meterDoc.Code == code && meterDoc.Info == info {
				return nil, jujutxn.ErrNoOperations
			}
		}
		return []txn.Op{
			{
				C:      unitsC,
				Id:     u.doc.DocID,
				Assert: isAliveDoc,
			}, {
				C:      meterStatusC,
				Id:     u.globalKey(),
				Assert: txn.DocExists,
				Update: bson.D{{"$set", bson.D{{"code", code}, {"info", info}}}},
			}}, nil
	}
	return errors.Annotatef(u.st.run(buildTxn), "cannot set meter state for unit %s", u.Name())
}

// createMeterStatusOp returns the operation needed to create the meter status
// document associated with the given globalKey.
func createMeterStatusOp(st *State, globalKey string, doc meterStatusDoc) txn.Op {
	return txn.Op{
		C:      meterStatusC,
		Id:     globalKey,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// removeMeterStatusOp returns the operation needed to remove the meter status
// document associated with the given globalKey.
func removeMeterStatusOp(st *State, globalKey string) txn.Op {
	return txn.Op{
		C:      meterStatusC,
		Id:     globalKey,
		Remove: true,
	}
}

// GetMeterStatus returns the meter status for the unit.
func (u *Unit) GetMeterStatus() (code, info string, err error) {
	status, err := u.getMeterStatusDoc()
	if err != nil {
		return string(MeterNotAvailable), "", errors.Annotatef(err, "cannot retrieve meter status for unit %s", u.Name())
	}
	return string(status.Code), status.Info, nil
}

func (u *Unit) getMeterStatusDoc() (*meterStatusDoc, error) {
	meterStatuses, closer := u.st.getCollection(meterStatusC)
	defer closer()
	var status meterStatusDoc
	err := meterStatuses.FindId(u.globalKey()).One(&status)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &status, nil
}
