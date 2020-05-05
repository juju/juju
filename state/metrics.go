// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
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
	UUID           string    `bson:"_id"`
	ModelUUID      string    `bson:"model-uuid"`
	Unit           string    `bson:"unit"`
	CharmURL       string    `bson:"charmurl"`
	Sent           bool      `bson:"sent"`
	DeleteTime     time.Time `bson:"delete-time"`
	Created        time.Time `bson:"created"`
	Metrics        []Metric  `bson:"metrics"`
	Credentials    []byte    `bson:"credentials"`
	SLACredentials []byte    `bson:"sla-credentials,omitempty"`
}

// Metric represents a single Metric.
type Metric struct {
	Key    string            `bson:"key"`
	Value  string            `bson:"value"`
	Time   time.Time         `bson:"time"`
	Labels map[string]string `bson:"labels,omitempty"`
}

type byTime []Metric

// Len implements sort.Interface.
func (t byTime) Len() int { return len(t) }

// Swap implements sort.Interface.
func (t byTime) Swap(i, j int) { t[i], t[j] = t[j], t[i] }

// Less implements sort.Interface.
func (t byTime) Less(i, j int) bool {
	return t[i].Time.Before(t[j].Time)
}

type byKey []Metric

// Len implements sort.Interface.
func (t byKey) Len() int { return len(t) }

// Swap implements sort.Interface.
func (t byKey) Swap(i, j int) { t[i], t[j] = t[j], t[i] }

// Less implements sort.Interface.
func (t byKey) Less(i, j int) bool {
	if t[i].Key == t[j].Key {
		return labelsKey(t[i].Labels) < labelsKey(t[j].Labels)
	}
	return t[i].Key < t[j].Key
}

func labelsKey(m map[string]string) string {
	var result []string
	for k, v := range m {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(result)
	return strings.Join(result, ",")
}

// validate checks that the MetricBatch contains valid metrics.
func (m *MetricBatch) validate() error {
	charmURL, err := charm.ParseURL(m.doc.CharmURL)
	if err != nil {
		return errors.Trace(err)
	}
	chrm, err := m.st.Charm(charmURL)
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
	UUID     string
	CharmURL string
	Created  time.Time
	Metrics  []Metric
	Unit     names.UnitTag
}

// ModelBatchParam contains the properties of a metric batch for a model
// The model uuid will be attenuated in the call to AddModelMetrics.
type ModelBatchParam struct {
	UUID    string
	Created time.Time
	Metrics []Metric
}

// AddMetrics adds a new batch of metrics to the database.
func (st *State) AddMetrics(batch BatchParam) (*MetricBatch, error) {
	if len(batch.Metrics) == 0 {
		return nil, errors.New("cannot add a batch of 0 metrics")
	}
	charmURL, err := charm.ParseURL(batch.CharmURL)
	if err != nil {
		return nil, errors.NewNotValid(err, "could not parse charm URL")
	}

	unit, err := st.Unit(batch.Unit.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	application, err := unit.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}

	slaCreds, err := st.SLACredential()
	if err != nil {
		return nil, errors.Trace(err)
	}

	metric := &MetricBatch{
		st: st,
		doc: metricBatchDoc{
			UUID:           batch.UUID,
			ModelUUID:      st.ModelUUID(),
			Unit:           batch.Unit.Id(),
			CharmURL:       charmURL.String(),
			Sent:           false,
			Created:        batch.Created,
			Metrics:        batch.Metrics,
			Credentials:    application.MetricCredentials(),
			SLACredentials: slaCreds,
		},
	}
	if err := metric.validate(); err != nil {
		return nil, err
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			notDead, err := isNotDead(st, unitsC, batch.Unit.Id())
			if err != nil || !notDead {
				return nil, errors.NotFoundf(batch.Unit.Id())
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
			Id:     st.docID(batch.Unit.Id()),
			Assert: notDeadDoc,
		}, {
			C:      metricsC,
			Id:     metric.UUID(),
			Assert: txn.DocMissing,
			Insert: &metric.doc,
		}}
		return ops, nil
	}
	err = st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return metric, nil
}

// AddModelMetrics adds a new model-centric batch of metrics to the database.
func (st *State) AddModelMetrics(batch ModelBatchParam) (*MetricBatch, error) {
	if len(batch.Metrics) == 0 {
		return nil, errors.New("cannot add a batch of 0 metrics")
	}
	slaCreds, err := st.SLACredential()
	if err != nil {
		return nil, errors.Trace(err)
	}
	metric := &MetricBatch{
		st: st,
		doc: metricBatchDoc{
			UUID:           batch.UUID,
			ModelUUID:      st.ModelUUID(),
			Sent:           false,
			Created:        batch.Created,
			Metrics:        batch.Metrics,
			SLACredentials: slaCreds,
		},
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			exists, err := st.MetricBatch(batch.UUID)
			if exists != nil && err == nil {
				return nil, errors.AlreadyExistsf("metrics batch UUID %q", batch.UUID)
			}
			if !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			}
		}
		ops := []txn.Op{{
			C:      metricsC,
			Id:     metric.UUID(),
			Assert: txn.DocMissing,
			Insert: &metric.doc,
		}}
		return ops, nil
	}
	err = st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return metric, nil
}

// AllMetricBatches returns all metric batches currently stored in state.
// TODO (tasdomas): this method is currently only used in the uniter worker test -
//                  it needs to be modified to restrict the scope of the values it
//                  returns if it is to be used outside of tests.
func (st *State) AllMetricBatches() ([]MetricBatch, error) {
	c, closer := st.db().GetCollection(metricsC)
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

func (st *State) queryMetricBatches(query bson.M) ([]MetricBatch, error) {
	c, closer := st.db().GetCollection(metricsC)
	defer closer()
	docs := []metricBatchDoc{}
	err := c.Find(query).Sort("created").All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]MetricBatch, len(docs))
	for i, doc := range docs {
		results[i] = MetricBatch{st: st, doc: doc}
	}
	return results, nil
}

// MetricBatchesForUnit returns metric batches for the given unit.
func (st *State) MetricBatchesForUnit(unit string) ([]MetricBatch, error) {
	_, err := st.Unit(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st.queryMetricBatches(bson.M{"unit": unit})
}

// MetricBatchesForModel returns metric batches for all the units in the model.
func (st *State) MetricBatchesForModel() ([]MetricBatch, error) {
	return st.queryMetricBatches(bson.M{"model-uuid": st.ModelUUID()})
}

// MetricBatchesForApplication returns metric batches for the given application.
func (st *State) MetricBatchesForApplication(application string) ([]MetricBatch, error) {
	app, err := st.Application(application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	units, err := app.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitNames := make([]bson.M, len(units))
	for i, u := range units {
		unitNames[i] = bson.M{"unit": u.Name()}
	}
	return st.queryMetricBatches(bson.M{"$or": unitNames})
}

// MetricBatch returns the metric batch with the given id.
func (st *State) MetricBatch(id string) (*MetricBatch, error) {
	c, closer := st.db().GetCollection(metricsC)
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
	now := st.clock().Now()
	metrics, closer := st.db().GetCollection(metricsC)
	defer closer()
	// Nothing else in the system will interact with sent metrics, and nothing needs
	// to watch them either; so in this instance it's safe to do an end run around the
	// mgo/txn package. See State.cleanupRelationSettings for a similar situation.
	metricsW := metrics.Writeable()
	// TODO (mattyw) iter over this.
	info, err := metricsW.RemoveAll(bson.M{
		"model-uuid":  st.ModelUUID(),
		"sent":        true,
		"delete-time": bson.M{"$lte": now},
	})
	if err == nil {
		metricsLogger.Tracef("cleanup removed %d metrics", info.Removed)
	}
	return errors.Trace(err)
}

// MetricsToSend returns batchSize metrics that need to be sent
// to the collector
func (st *State) MetricsToSend(batchSize int) ([]*MetricBatch, error) {
	var docs []metricBatchDoc
	c, closer := st.db().GetCollection(metricsC)
	defer closer()

	q := bson.M{
		"model-uuid": st.ModelUUID(),
		"sent":       false,
	}
	err := c.Find(q).Limit(batchSize).All(&docs)
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
	c, closer := st.db().GetCollection(metricsC)
	defer closer()
	return c.Find(bson.M{
		"model-uuid": st.ModelUUID(),
		"sent":       false,
	}).Count()
}

// CountOfSentMetrics returns the number of metrics that
// have been sent to the collection service and have not
// been removed by the cleanup worker.
func (st *State) CountOfSentMetrics() (int, error) {
	c, closer := st.db().GetCollection(metricsC)
	defer closer()
	return c.Find(bson.M{
		"model-uuid": st.ModelUUID(),
		"sent":       true,
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

// ModelUUID returns the model UUID this metric applies to.
func (m *MetricBatch) ModelUUID() string {
	return m.doc.ModelUUID
}

// Unit returns the name of the unit this metric was generated in.
func (m *MetricBatch) Unit() string {
	return m.doc.Unit
}

// CharmURL returns the charm url for the charm this metric was generated in.
func (m *MetricBatch) CharmURL() string {
	return m.doc.CharmURL
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

// UniqueMetrics returns only the last value for each
// metric key in this batch.
func (m *MetricBatch) UniqueMetrics() []Metric {
	metrics := m.Metrics()
	sort.Sort(byTime(metrics))
	uniq := map[string]Metric{}
	for _, m := range metrics {
		uniq[fmt.Sprintf("%s-%s", m.Key, labelsKey(m.Labels))] = m
	}
	results := make([]Metric, len(uniq))
	i := 0
	for _, m := range uniq {
		results[i] = m
		i++
	}
	sort.Sort(byKey(results))
	return results
}

// SetSent marks the metric has having been sent at
// the specified time.
func (m *MetricBatch) SetSent(t time.Time) error {
	deleteTime := t.UTC().Add(CleanupAge)
	ops := setSentOps([]string{m.UUID()}, deleteTime)
	if err := m.st.db().RunTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set metric sent for metric %q", m.UUID())
	}

	m.doc.Sent = true
	m.doc.DeleteTime = deleteTime
	return nil
}

// Credentials returns any credentials associated with the metric batch.
func (m *MetricBatch) Credentials() []byte {
	return m.doc.Credentials
}

// SLACredentials returns any sla credentials associated with the metric batch.
func (m *MetricBatch) SLACredentials() []byte {
	return m.doc.SLACredentials
}

func setSentOps(batchUUIDs []string, deleteTime time.Time) []txn.Op {
	ops := make([]txn.Op, len(batchUUIDs))
	for i, u := range batchUUIDs {
		ops[i] = txn.Op{
			C:      metricsC,
			Id:     u,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"sent": true, "delete-time": deleteTime}},
		}
	}
	return ops
}

// SetMetricBatchesSent sets sent on each MetricBatch corresponding to the uuids provided.
func (st *State) SetMetricBatchesSent(batchUUIDs []string) error {
	deleteTime := st.clock().Now().UTC().Add(CleanupAge)
	ops := setSentOps(batchUUIDs, deleteTime)
	if err := st.db().RunTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set metric sent in bulk call")
	}
	return nil
}
