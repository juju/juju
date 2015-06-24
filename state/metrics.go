// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var metricsLogger = loggo.GetLogger("juju.state.metrics")

const (
	CleanupAge = time.Hour * 24
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
	UUID        string    `bson:"_id"`
	EnvUUID     string    `bson:"env-uuid"`
	Unit        string    `bson:"unit"`
	CharmUrl    string    `bson:"charmurl"`
	Sent        bool      `bson:"sent"`
	Created     time.Time `bson:"created"`
	Metrics     []Metric  `bson:"metrics"`
	Credentials []byte    `bson:"credentials"`
}

// Metric represents a single Metric.
type Metric struct {
	Key   string    `bson:"key"`
	Value string    `bson:"value"`
	Time  time.Time `bson:"time"`
}

// validate checks that the MetricBatch contains valid metrics.
func (m *MetricBatch) validate() error {
	charmUrl, err := charm.ParseURL(m.doc.CharmUrl)
	if err != nil {
		return errors.Trace(err)
	}
	chrm, err := m.st.Charm(charmUrl)
	if err != nil {
		return errors.Trace(err)
	}
	chrmMetrics := chrm.Metrics()
	if chrmMetrics == nil {
		return errors.Errorf("charm doesn't implement metrics")
	}
	for _, m := range m.doc.Metrics {
		if err := chrmMetrics.ValidateMetric(m.Key, m.Value); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// BatchParam contains the properties of the metrics batch used when creating a metrics
// batch.
type BatchParam struct {
	UUID        string
	CharmURL    *charm.URL
	Created     time.Time
	Metrics     []Metric
	Credentials []byte
}

// addMetrics adds a new batch of metrics to the database.
func (st *State) addMetrics(unitTag names.UnitTag, batch BatchParam) (*MetricBatch, error) {
	if len(batch.Metrics) == 0 {
		return nil, errors.New("cannot add a batch of 0 metrics")
	}

	metric := &MetricBatch{
		st: st,
		doc: metricBatchDoc{
			UUID:        batch.UUID,
			EnvUUID:     st.EnvironUUID(),
			Unit:        unitTag.Id(),
			CharmUrl:    batch.CharmURL.String(),
			Sent:        false,
			Created:     batch.Created,
			Metrics:     batch.Metrics,
			Credentials: batch.Credentials,
		}}
	if err := metric.validate(); err != nil {
		return nil, err
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			notDead, err := isNotDead(st, unitsC, unitTag.Id())
			if err != nil || !notDead {
				return nil, errors.NotFoundf(unitTag.Id())
			}
			exists, err := st.MetricBatch(batch.UUID)
			if exists != nil && err == nil {
				return nil, errors.AlreadyExistsf("metrics batch UUID %q", batch.UUID)
			}
			if !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
		}
		ops := []txn.Op{{
			C:      unitsC,
			Id:     st.docID(unitTag.Id()),
			Assert: notDeadDoc,
		}, {
			C:      metricsC,
			Id:     metric.UUID(),
			Assert: txn.DocMissing,
			Insert: &metric.doc,
		}}
		return ops, nil
	}
	err := st.run(buildTxn)
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
	age := time.Now().Add(-(CleanupAge))
	metricsLogger.Tracef("cleaning up metrics created before %v", age)
	metrics, closer := st.getCollection(metricsC)
	defer closer()
	// Nothing else in the system will interact with sent metrics, and nothing needs
	// to watch them either; so in this instance it's safe to do an end run around the
	// mgo/txn package. See State.cleanupRelationSettings for a similar situation.
	metricsW := metrics.Writeable()
	info, err := metricsW.RemoveAll(bson.M{
		"sent":    true,
		"created": bson.M{"$lte": age},
	})
	metricsLogger.Tracef("cleanup removed %d metrics", info.Removed)
	return errors.Trace(err)
}

// MetricsToSend returns batchSize metrics that need to be sent
// to the collector
func (st *State) MetricsToSend(batchSize int) ([]*MetricBatch, error) {
	var docs []metricBatchDoc
	c, closer := st.getCollection(metricsC)
	defer closer()
	err := c.Find(bson.M{
		"sent": false,
	}).Limit(batchSize).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	batch := make([]*MetricBatch, len(docs))
	for i, doc := range docs {
		batch[i] = &MetricBatch{st: st, doc: doc}

	}

	return batch, nil
}

// CountOfUnsentMetrics returns the number of metrics that
// haven't been sent to the collection service.
func (st *State) CountOfUnsentMetrics() (int, error) {
	c, closer := st.getCollection(metricsC)
	defer closer()
	return c.Find(bson.M{
		"sent": false,
	}).Count()
}

// CountOfSentMetrics returns the number of metrics that
// have been sent to the collection service and have not
// been removed by the cleanup worker.
func (st *State) CountOfSentMetrics() (int, error) {
	c, closer := st.getCollection(metricsC)
	defer closer()
	return c.Find(bson.M{
		"sent": true,
	}).Count()
}

// MarshalJSON defines how the MetricBatch type should be
// converted to json.
func (m *MetricBatch) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.doc)
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

// Created returns the time this metric batch was created.
func (m *MetricBatch) Created() time.Time {
	return m.doc.Created
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
	ops := setSentOps([]string{m.UUID()})
	if err := m.st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set metric sent for metric %q", m.UUID())
	}

	m.doc.Sent = true
	return nil
}

// Credentials returns any credentials associated with the metric batch.
func (m *MetricBatch) Credentials() []byte {
	return m.doc.Credentials
}

func setSentOps(batchUUIDs []string) []txn.Op {
	ops := make([]txn.Op, len(batchUUIDs))
	for i, u := range batchUUIDs {
		ops[i] = txn.Op{
			C:      metricsC,
			Id:     u,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"sent": true}},
		}
	}
	return ops
}

// SetMetricBatchesSent sets sent on each MetricBatch corresponding to the uuids provided.
func (st *State) SetMetricBatchesSent(batchUUIDs []string) error {
	ops := setSentOps(batchUUIDs)
	if err := st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set metric sent in bulk call")
	}
	return nil
}
