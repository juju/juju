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

// MeterStatus represents the metering status of a unit.
type MeterStatus struct {
	Code MeterStatusCode
	Info string
}

// Severity returns relative severity of the meter status.
func (m *MeterStatus) Severity() int {
	return m.Code.Severity()
}

// MeterStatusCode represents the meter status code of a unit.
// The int value represents its relative severity when compared to
// other MeterStatusCodes.
type MeterStatusCode int

// Severity returns the relative severity.
func (m MeterStatusCode) Severity() int {
	return int(m)
}

// String returns a human readable string representation of the meter status.
func (m MeterStatusCode) String() (string, error) {
	s, ok := meterString[m]
	if !ok {
		return "", errors.Errorf("%q is not a valid meter status", m)
	}
	return s, nil
}

// MeterStatusFromString returns a valid MeterStatusCode given a string representation.
func MeterStatusFromString(str string) (MeterStatusCode, error) {
	for m, s := range meterString {
		if s == str {
			return m, nil
		}
	}
	return -1, errors.Errorf("%q is not a valid meter status", str)
}

// This const block defines the relative severities of the valid MeterStatusCodes in ascending order.
const (
	MeterNotSet MeterStatusCode = iota
	MeterGreen
	MeterAmber
	MeterRed

	MeterGreenString  = "GREEN"
	MeterNotSetString = "NOT SET"
	MeterAmberString  = "AMBER"
	MeterRedString    = "RED"
)

var (
	meterString = map[MeterStatusCode]string{
		MeterGreen:  MeterGreenString,
		MeterNotSet: MeterNotSetString,
		MeterAmber:  MeterAmberString,
		MeterRed:    MeterRedString,
	}
)

type meterStatusDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`
	Code    string `bson:"code"`
	Info    string `bson:"info"`
}

// SetMeterStatus sets the meter status for the unit.
func (u *Unit) SetMeterStatus(codeStr, info string) error {
	code, err := MeterStatusFromString(codeStr)
	if err != nil {
		return errors.Trace(err)
	}
	switch code {
	case MeterGreen, MeterAmber, MeterRed:
	case MeterNotSet:
		return errors.Errorf("you can only set MeterGreen, MeterRed or MeterAmber")
	default:
	}
	meterDoc, err := u.getMeterStatusDoc()
	if err != nil {
		return errors.Annotatef(err, "cannot update meter status for unit %s", u.Name())
	}
	codeStr, err = code.String()
	if err != nil {
		return errors.Trace(err)
	}
	if meterDoc.Code == codeStr && meterDoc.Info == info {
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
			if meterDoc.Code == codeStr && meterDoc.Info == info {
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
				Id:     u.st.docID(u.globalKey()),
				Assert: txn.DocExists,
				Update: bson.D{{"$set", bson.D{{"code", codeStr}, {"info", info}}}},
			}}, nil
	}
	return errors.Annotatef(u.st.run(buildTxn), "cannot set meter state for unit %s", u.Name())
}

// createMeterStatusOp returns the operation needed to create the meter status
// document associated with the given globalKey.
func createMeterStatusOp(st *State, globalKey string, doc *meterStatusDoc) txn.Op {
	doc.EnvUUID = st.EnvironUUID()
	return txn.Op{
		C:      meterStatusC,
		Id:     st.docID(globalKey),
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// removeMeterStatusOp returns the operation needed to remove the meter status
// document associated with the given globalKey.
func removeMeterStatusOp(st *State, globalKey string) txn.Op {
	return txn.Op{
		C:      meterStatusC,
		Id:     st.docID(globalKey),
		Remove: true,
	}
}

// GetMeterStatus returns the meter status for the unit.
func (u *Unit) GetMeterStatus() (*MeterStatus, error) {
	mm, err := u.st.MetricsManager()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot retrieve meter status for metrics manager")
	}

	mmStatus := mm.MeterStatus()
	if mmStatus.Code == MeterRed {
		return &mmStatus, nil
	}

	status, err := u.getMeterStatusDoc()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot retrieve meter status for unit %s", u.Name())
	}

	code, err := MeterStatusFromString(status.Code)
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitMeterStatus := MeterStatus{code, status.Info}
	ms := combineMeterStatus(mmStatus, unitMeterStatus)
	return &ms, nil
}

func combineMeterStatus(a, b MeterStatus) MeterStatus {
	if a.Severity() > b.Severity() {
		return a
	}
	return b
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
