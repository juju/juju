// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"
)

const (
	defaultGracePeriod                      = 7 * 24 * time.Hour // 1 week in hours
	metricsManagerConsecutiveErrorThreshold = 3
)

func metricsManagerKey(st *State) string {
	return st.docID("metricsManager")
}

// MetricsManager stores data about the state of the metrics manager
type MetricsManager struct {
	st     *State
	doc    metricsManagerDoc
	status meterStatusDoc
}

type metricsManagerDoc struct {
	LastSuccessfulSend time.Time     `bson:"lastsuccessfulsend"`
	ConsecutiveErrors  int           `bson:"consecutiveerrors"`
	GracePeriod        time.Duration `bson:"graceperiod"`
}

// LastSuccessfulSend returns the time of the last successful send.
func (m *MetricsManager) LastSuccessfulSend() time.Time {
	return m.doc.LastSuccessfulSend
}

// ConsecutiveErrors returns the number of consecutive failures.
func (m *MetricsManager) ConsecutiveErrors() int {
	return m.doc.ConsecutiveErrors
}

// GracePeriod returns the current grace period.
func (m *MetricsManager) GracePeriod() time.Duration {
	return m.doc.GracePeriod
}

// MetricsManager returns an existing metricsmanager, or a new one if non exists.
func (st *State) MetricsManager() (*MetricsManager, error) {
	mm, err := st.getMetricsManager()
	if errors.Is(err, errors.NotFound) {
		return st.newMetricsManager()
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return mm, nil
}

func (st *State) newMetricsManager() (*MetricsManager, error) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 1 {
			if _, err := st.getMetricsManager(); err == nil {
				return nil, jujutxn.ErrNoOperations
			}
		}
		id := metricsManagerKey(st)
		mm := &MetricsManager{
			st: st,
			doc: metricsManagerDoc{
				LastSuccessfulSend: time.Time{},
				ConsecutiveErrors:  0,
				GracePeriod:        defaultGracePeriod,
			},
			status: meterStatusDoc{
				Code:      meterString[MeterNotSet],
				ModelUUID: st.ModelUUID(),
			},
		}
		return []txn.Op{{
			C:      metricsManagerC,
			Id:     id,
			Assert: txn.DocMissing,
			Insert: mm.doc,
		}, {
			C:      meterStatusC,
			Id:     id,
			Assert: txn.DocMissing,
			Insert: mm.status,
		}}, nil
	}
	err := st.db().Run(buildTxn)
	if err != nil {
		return nil, onAbort(err, errors.NotFoundf("metrics manager"))
	}
	return st.getMetricsManager()
}

func (st *State) getMetricsManager() (*MetricsManager, error) {
	coll, closer := st.db().GetCollection(metricsManagerC)
	defer closer()
	var doc metricsManagerDoc
	err := coll.FindId(metricsManagerKey(st)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("metrics manager")
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	collS, closerS := st.db().GetCollection(meterStatusC)
	defer closerS()
	status := meterStatusDoc{
		Code:      meterString[MeterNotSet],
		ModelUUID: st.ModelUUID(),
	}
	err = collS.FindId(metricsManagerKey(st)).One(&status)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}
	return &MetricsManager{
		st:     st,
		doc:    doc,
		status: status,
	}, nil
}

func (m *MetricsManager) updateMetricsManager(update bson.M, status *bson.M) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if _, err := m.st.getMetricsManager(); errors.Is(err, errors.NotFound) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}
		ops := []txn.Op{{
			C:      metricsManagerC,
			Id:     metricsManagerKey(m.st),
			Assert: txn.DocExists,
			Update: update,
		}}
		if status != nil {
			ops = append(ops, txn.Op{
				C:      meterStatusC,
				Id:     metricsManagerKey(m.st),
				Assert: txn.DocExists,
				Update: *status,
			})
		}
		return ops, nil
	}

	if err := m.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetLastSuccessfulSend sets the last successful send time to the input time.
func (m *MetricsManager) SetLastSuccessfulSend(t time.Time) error {
	var status *bson.M
	if m.status.Code != meterString[MeterGreen] {
		status = &bson.M{
			"$set": bson.M{
				"code": meterString[MeterGreen],
				"info": "",
			},
		}
	}
	err := m.updateMetricsManager(
		bson.M{
			"$set": bson.M{
				"lastsuccessfulsend": t.UTC(),
				"consecutiveerrors":  0,
			},
		},
		status,
	)
	if err != nil {
		return errors.Trace(err)
	}

	m.doc.LastSuccessfulSend = t.UTC()
	m.doc.ConsecutiveErrors = 0
	return nil
}

func (m *MetricsManager) SetGracePeriod(t time.Duration) error {
	if t < 0 {
		return errors.New("grace period can't be negative")
	}
	m1 := MetricsManager{
		st:  m.st,
		doc: m.doc,
	}
	m1.doc.GracePeriod = t
	newStatus := m1.MeterStatus()

	var statusUpdate *bson.M
	if newStatus != m.MeterStatus() {
		statusUpdate = &bson.M{
			"$set": bson.M{
				"code":       meterString[newStatus.Code],
				"info":       newStatus.Info,
				"model-uuid": m.st.ModelUUID(),
			},
		}
	}

	err := m.updateMetricsManager(
		bson.M{"$set": bson.M{
			"graceperiod": t,
		}},
		statusUpdate,
	)
	if err != nil {
		return errors.Trace(err)
	}
	m.doc.GracePeriod = t
	return nil
}

// IncrementConsecutiveErrors adds 1 to the consecutive errors count.
func (m *MetricsManager) IncrementConsecutiveErrors() error {
	m1 := MetricsManager{
		st:  m.st,
		doc: m.doc,
	}
	m1.doc.ConsecutiveErrors++
	newStatus := m1.MeterStatus()

	var statusUpdate *bson.M
	if newStatus != m.MeterStatus() {
		statusUpdate = &bson.M{
			"$set": bson.M{
				"code":       meterString[newStatus.Code],
				"info":       newStatus.Info,
				"model-uuid": m.st.ModelUUID(),
			},
		}
	}
	err := m.updateMetricsManager(
		bson.M{"$inc": bson.M{"consecutiveerrors": 1}},
		statusUpdate,
	)
	if err != nil {
		return errors.Trace(err)
	}
	m.doc.ConsecutiveErrors++
	return nil
}

func (m *MetricsManager) gracePeriodExceeded() bool {
	now := m.st.clock().Now()
	t := m.LastSuccessfulSend().Add(m.GracePeriod())
	return t.Before(now) || t.Equal(now)
}

// MeterStatus returns the overall state of the MetricsManager as a meter status summary.
func (m *MetricsManager) MeterStatus() MeterStatus {
	if m.ConsecutiveErrors() < metricsManagerConsecutiveErrorThreshold {
		return MeterStatus{Code: MeterGreen, Info: "ok"}
	}
	if m.gracePeriodExceeded() {
		return MeterStatus{Code: MeterRed, Info: "failed to send metrics, exceeded grace period"}
	}
	return MeterStatus{Code: MeterAmber, Info: "failed to send metrics"}
}

func (m *MetricsManager) ModelStatus() MeterStatus {
	return MeterStatus{
		Code: MeterStatusFromString(m.status.Code),
		Info: m.status.Info,
	}
}
