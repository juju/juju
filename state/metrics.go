// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// MetricBatch represents a batch of metrics reported from a unit.
// These will be received from the unit in batches.
// The main contents of the metric (key, value) is defined
// by the charm author and sent from the unit via a call to
// add-metric
type MetricBatch struct {
	st  *State
	doc metricBatchDoc
}

type metricBatchDoc struct {
	UUID     string    `bson:"_id"`
	Unit     string    `bson:"unit"`
	CharmUrl string    `bson:"charmurl"`
	Sent     bool      `bson:"sent"`
	Created  time.Time `bson:"created"`
	Metrics  []Metric  `bson:"metrics"`
}

// Metric represents a single Metric.
type Metric struct {
	Key         string    `bson:"key"`
	Value       string    `bson:"value"`
	Time        time.Time `bson:"time"`
	Credentials []byte    `bson:"credentials"`
}

// AddMetric adds a new batch of metrics to the database.
// A UUID for the metric will be generated and the new MetricBatch will be returned
func (st *State) addMetrics(unitTag names.UnitTag, charmUrl *charm.URL, created time.Time, metrics []Metric) (*MetricBatch, error) {
	if len(metrics) == 0 {
		return nil, errors.New("cannot add a batch of 0 metrics")
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}

	metric := &MetricBatch{
		st: st,
		doc: metricBatchDoc{
			UUID:     uuid.String(),
			Unit:     unitTag.Id(),
			CharmUrl: charmUrl.String(),
			Sent:     false,
			Created:  created,
			Metrics:  metrics,
		}}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			notDead, err := isNotDead(st.db, unitsC, unitTag.Id())
			if err != nil || !notDead {
				return nil, errors.NotFoundf(unitTag.Id())
			}
		}
		ops := []txn.Op{{
			C:      unitsC,
			Id:     unitTag.Id(),
			Assert: notDeadDoc,
		}, {
			C:      metricsC,
			Id:     metric.UUID(),
			Assert: txn.DocMissing,
			Insert: &metric.doc,
		}}
		return ops, nil
	}
	err = st.run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return metric, nil
}

// MetricBatch returns the metric batch with the given id
func (st *State) MetricBatch(id string) (*MetricBatch, error) {
	c, closer := st.getCollection(metricsC)
	defer closer()
	doc := metricBatchDoc{}
	err := c.Find(bson.M{"_id": id}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("metric %v", id)
	}
	if err != nil {
		return nil, err
	}
	return &MetricBatch{st: st, doc: doc}, nil
}

// CleanupOldMetrics looks for metrics that are 24 hours old (or older)
// and have been sent. Any metrics it finds are deleted.
func (st *State) CleanupOldMetrics() error {
	age := time.Now().Add(-(time.Hour * 24))
	c, closer := st.getCollection(metricsC)
	defer closer()
	return c.Remove(bson.M{
		"sent":    true,
		"created": bson.M{"$lte": age},
	})
}

// UUID returns to uuid of the metric.
func (m *MetricBatch) UUID() string {
	return m.doc.UUID
}

// Unit returns the name of the unit this metric was generated in.
func (m *MetricBatch) Unit() string {
	return m.doc.Unit
}

// CharmURL returns the charm url for the charm this metric was generated in.
func (m *MetricBatch) CharmURL() string {
	return m.doc.CharmUrl
}

// Sent returns a flag to tell us if this metric has been sent to the metric
// collection service
func (m *MetricBatch) Sent() bool {
	return m.doc.Sent
}

// SetSent sets the sent flag to true
func (m *MetricBatch) SetSent() error {
	ops := []txn.Op{{
		C:      metricsC,
		Id:     m.UUID(),
		Assert: txn.DocExists,
		Update: bson.M{"$set": bson.M{"sent": true}},
	}}
	if err := m.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set metric sent for metric %q", m.UUID())
	}

	m.doc.Sent = true
	return nil
}

// Metrics returns the metrics in this batch.
func (m *MetricBatch) Metrics() []Metric {
	result := make([]Metric, len(m.doc.Metrics))
	copy(result, m.doc.Metrics)
	return result
}
