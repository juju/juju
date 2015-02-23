// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var metricsManagerLogger = loggo.GetLogger("juju.state.metricsmanager")

const (
	defaultGracePeriod = 7 * 24 * time.Hour // 1 week in hours
	metricsManagerKey  = "metricsManagerKey"
)

var (
	MetricsManagerNotFoundError = errors.New("cannot create new metrics manager - only one allowed")
)

// MetricsManager stores data about the state of the metrics manager
type MetricsManager struct {
	st  *State
	doc metricsManagerDoc
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

// GracePeriodSeconds returns the current grace period.
func (m *MetricsManager) GracePeriod() time.Duration {
	return m.doc.GracePeriod
}

// NewMetricsManager returns a new MetricsManager
// As only one can ever exist an error is returned if one already exists.
func (st *State) NewMetricsManager() (*MetricsManager, error) {
	mm := &MetricsManager{
		st: st,
		doc: metricsManagerDoc{
			LastSuccessfulSend: time.Time{},
			ConsecutiveErrors:  0,
			GracePeriod:        defaultGracePeriod,
		}}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			coll, closer := st.getCollection(metricsManagerC)
			defer closer()
			n, err := coll.Find(bson.D{{"_id", metricsManagerKey}}).Count()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if n > 0 {
				return nil, MetricsManagerNotFoundError
			}
		}
		return []txn.Op{{
			C:      metricsManagerC,
			Id:     metricsManagerKey,
			Assert: txn.DocMissing,
		}, {
			C:      metricsManagerC,
			Id:     metricsManagerKey,
			Assert: txn.DocMissing,
			Insert: mm.doc,
		},
		}, nil
	}

	err := st.run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return mm, nil
}

// GetMetricsManager returns an existing metrics manager if one exists.
func (st *State) GetMetricsManager() (*MetricsManager, error) {
	coll, closer := st.getCollection(metricsManagerC)
	defer closer()
	n, err := coll.Find(bson.D{{"_id", metricsManagerKey}}).Count()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if n == 0 {
		return nil, MetricsManagerNotFoundError
	}
	var doc metricsManagerDoc
	err = coll.Find(bson.D{{"_id", metricsManagerKey}}).One(&doc)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MetricsManager{st: st, doc: doc}, nil
}

func (m *MetricsManager) updateMetricsManagerOps(update bson.M) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			coll, closer := m.st.getCollection(metricsManagerC)
			defer closer()
			n, err := coll.Find(bson.D{{"_id", metricsManagerKey}}).Count()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if n == 0 {
				return nil, errors.New("no existing metrics manager object")
			}
		}
		return []txn.Op{{
			C:      metricsManagerC,
			Id:     metricsManagerKey,
			Assert: txn.DocExists,
			Update: update,
		}}, nil
	}
	return m.st.run(buildTxn)
}

// SetMetricsManagerSuccessfulSend sets the last successful send time to the input time.
func (m *MetricsManager) SetMetricsManagerSuccessfulSend(t time.Time) error {
	err := m.updateMetricsManagerOps(
		bson.M{"$set": bson.M{"lastsuccessfulsend": t}},
	)
	if err != nil {
		return errors.Trace(err)
	}
	m.doc.LastSuccessfulSend = t
	return nil
}

// IncrementConsecutiveErrors adds 1 to the consecutive errors count.
func (m *MetricsManager) IncrementConsecutiveErrors() error {
	err := m.updateMetricsManagerOps(
		bson.M{"$inc": bson.M{"consecutiveerrors": 1}},
	)
	if err != nil {
		return errors.Trace(err)
	}
	m.doc.ConsecutiveErrors++
	return nil
}

// SetNoConsecutiveErrors resets the consecutive errors back to 0.
func (m *MetricsManager) SetNoConsecutiveErrors() error {
	err := m.updateMetricsManagerOps(
		bson.M{"$set": bson.M{"consecutiveerrors": 0}},
	)
	if err != nil {
		return errors.Trace(err)
	}
	m.doc.ConsecutiveErrors = 0
	return nil
}

func (m *MetricsManager) gracePeriodExceeded() bool {
	now := time.Now()
	t := m.LastSuccessfulSend().Add(m.GracePeriod())
	return t.Before(now) || t.Equal(now)
}

// MeterStatus returns the overall state of the MetricsManager as a meter status summary.
func (m *MetricsManager) MeterStatus() (code, info string) {
	if m.ConsecutiveErrors() < 3 {
		return "GREEN", "metrics manager state ok"
	}
	if m.gracePeriodExceeded() {
		return "RED", "failed to send metrics to collector - exceeded grace period"
	}
	return "AMBER", "failed to send metrics to collector - still in grace period"
}
