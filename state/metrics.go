// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"
	"time"

	"github.com/juju/loggo"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var metricsLogger = loggo.GetLogger("juju.state.metrics")

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
	EnvUUID  string    `bson:"envuuid"`
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
			EnvUUID:  st.EnvironTag().String(),
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

// MetricBatches returns all metric batches currently stored in state.
// TODO (tasdomas): this method is currently only used in the uniter worker test -
//                  it needs to be modified to restrict the scope of the values it
//                  returns if it is to be used outside of tests.
func (st *State) MetricBatches() ([]MetricBatch, error) {
	c, closer := st.getCollection(metricsC)
	defer closer()
	docs := []metricBatchDoc{}
	err := c.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]MetricBatch, len(docs))
	for i, doc := range docs {
		results[i] = MetricBatch{st: st, doc: doc}
	}
	return results, nil
}

// MetricBatch returns the metric batch with the given id.
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
	// Nothing else in the system will interact with sent metrics, and nothing needs
	// to watch them either; so in this instance it's safe to do an end run around the
	// mgo/txn package. See State.cleanupRelationSettings for a similar situation.
	err := c.Remove(bson.M{
		"sent":    true,
		"created": bson.M{"$lte": age},
	})
	if err == mgo.ErrNotFound {
		metricsLogger.Infof("no metrics found to cleanup")
		return nil
	}
	return err
}

// MetricSender defines the interface used to send metrics
// to a collection service.
type MetricSender interface {
	Send([]*MetricBatch) error
}

// SendMetrics will send any unsent metrics
// over the MetricSender interface in batches
// no larger than batchSize.
func (st *State) SendMetrics(sender MetricSender, batchSize int) error {
	for {
		var docs []metricBatchDoc
		c, closer := st.getCollection(metricsC)
		defer closer()
		err := c.Find(bson.M{
			"sent": false,
		}).Limit(batchSize).All(&docs)
		if err != nil {
			return errors.Trace(err)
		}

		if len(docs) == 0 {
			break
		}
		batch := make([]*MetricBatch, len(docs))
		for i, doc := range docs {
			batch[i] = &MetricBatch{st: st, doc: doc}

		}

		err = st.sendBatch(sender, batch)
		if err != nil {
			return errors.Trace(err)
		}
	}
	unsent, err := st.countofUnsentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	sent, err := st.countofSentMetrics()
	if err != nil {
		return errors.Trace(err)
	}
	metricsLogger.Infof("metrics collection summary: sent:%d unsent:%d", sent, unsent)

	return nil
}

// countofUnsentMetrics returns the number of metrics that
// haven't been sent to the collection service.
func (st *State) countofUnsentMetrics() (int, error) {
	c, closer := st.getCollection(metricsC)
	defer closer()
	return c.Find(bson.M{
		"sent": false,
	}).Count()
}

// countofSentMetrics returns the number of metrics that
// have been sent to the collection service and have not
// been removed by the cleanup worker.
func (st *State) countofSentMetrics() (int, error) {
	c, closer := st.getCollection(metricsC)
	defer closer()
	return c.Find(bson.M{
		"sent": true,
	}).Count()
}

// sendBatch send metrics over the MetricSender interface and
// sets the metric's sent attribute.
func (st *State) sendBatch(sender MetricSender, metrics []*MetricBatch) error {
	err := sender.Send(metrics)
	if err != nil {
		return errors.Trace(err)
	}
	err = st.setMetricBatchesSent(metrics)
	if err != nil {
		metricsLogger.Warningf("failed to set sent on metrics %v", err)
	}
	return nil
}

// MarshalJSON defines how the MetricBatch type should be
// converted to json.
func (m *MetricBatch) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.doc)
}

// Refresh refreshes the contents of the MetricBatch from the underlying state.
func (m *MetricBatch) Refresh() error {
	metrics, closer := m.st.getCollection(metricsC)
	defer closer()

	err := metrics.FindId(m.doc.UUID).One(&m.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("metric %q", m)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh metric %q", m)
	}
	return nil
}

// UUID returns to uuid of the metric.
func (m *MetricBatch) UUID() string {
	return m.doc.UUID
}

// EnvUUID returns the environment UUID this metric applies to.
func (m *MetricBatch) EnvUUID() string {
	return m.doc.EnvUUID
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

// Metrics returns the metrics in this batch.
func (m *MetricBatch) Metrics() []Metric {
	result := make([]Metric, len(m.doc.Metrics))
	copy(result, m.doc.Metrics)
	return result
}

// SetSent sets the sent flag to true
func (m *MetricBatch) SetSent() error {
	ops := setSentOps([]*MetricBatch{m})
	if err := m.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set metric sent for metric %q", m.UUID())
	}

	m.doc.Sent = true
	return nil
}

func setSentOps(metrics []*MetricBatch) []txn.Op {
	ops := make([]txn.Op, len(metrics))
	for i, m := range metrics {
		ops[i] = txn.Op{
			C:      metricsC,
			Id:     m.UUID(),
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"sent": true}},
		}
	}
	return ops
}

func (st *State) setMetricBatchesSent(metrics []*MetricBatch) error {
	ops := setSentOps(metrics)
	if err := st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set metric sent in bulk call")
	}
	for _, m := range metrics {
		m.doc.Sent = true
	}
	return nil
}
