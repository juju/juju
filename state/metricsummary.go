// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// TODO how does the above affect migration?

type metricSummaryDoc struct {
	Id            string    `bson:"_id"`
	ModelUUID     string    `bson:"model-uuid"`
	Unit          string    `bson:"unit"`
	CharmURL      string    `bson:"charmurl"`
	Key           string    `bson:"key"`
	Value         string    `bson:"value"`
	Time          time.Time `bson:"time"`
	AbsoluteTotal float64   `bson:"absolutetotal"`
	TxnRevno      int64     `bson:"txn-revno"`
}

// MetricSummary contains potentially aggregated metric data
// to be used when presenting metric data to a user.
// In the case of guage metrics the Value will always be the
// last value recevied from the Unit.
// For absolute metrics the Value will represent the sum of
// Values received over the life of the Unit.
type MetricSummary struct {
	doc metricSummaryDoc
}

func (m *MetricSummary) Id() string        { return m.doc.Id }
func (m *MetricSummary) ModelUUID() string { return m.doc.ModelUUID }
func (m *MetricSummary) Unit() string      { return m.doc.Unit }
func (m *MetricSummary) CharmURL() string  { return m.doc.CharmURL }
func (m *MetricSummary) Key() string       { return m.doc.Key }
func (m *MetricSummary) Value() string     { return m.doc.Value }
func (m *MetricSummary) Time() time.Time   { return m.doc.Time }

func metricSummaryDocId(unit, metricKey string) string {
	return fmt.Sprintf("%s-%s", unit, metricKey)
}

func newMetricSummaryDocs(batch *MetricBatch) ([]metricSummaryDoc, error) {
	docs := make([]metricSummaryDoc, len(batch.UniqueMetrics()))
	for i, m := range batch.UniqueMetrics() {
		fValue, err := strconv.ParseFloat(m.Value, 64)
		if err != nil {
			return nil, errors.Trace(err)
		}
		docs[i] = metricSummaryDoc{
			Id:            metricSummaryDocId(batch.Unit(), m.Key),
			ModelUUID:     batch.ModelUUID(),
			Unit:          batch.Unit(),
			CharmURL:      batch.CharmURL(),
			Key:           m.Key,
			Value:         m.Value,
			Time:          m.Time,
			AbsoluteTotal: fValue,
		}
	}
	return docs, nil
}

func (st *State) removeMetricSummariesForUnit(unit string) error {
	c, closer := st.getCollection(metricSummaryC)
	defer closer()
	// Nothing else in the system will interact with metricsummaries, and nothing needs
	// to watch them either; so in this instance it's safe to do an end run around the
	// mgo/txn package. See State.CleanupOldMetrics for a similar situation.
	_, err := c.Writeable().RemoveAll(bson.M{
		"unit": unit,
	})
	return errors.Trace(err)
}

func (st *State) getId(id string) (*MetricSummary, error) {
	c, closer := st.getCollection(metricSummaryC)
	defer closer()
	doc := metricSummaryDoc{}
	err := c.FindId(id).One(&doc)
	if err != nil {
		return nil, err
	}
	return &MetricSummary{doc: doc}, nil
}

func (st *State) metricSummaryOps(batch *MetricBatch) ([]txn.Op, error) {
	fmt.Println("ops")
	docs, err := newMetricSummaryDocs(batch)
	if err != nil {
		return nil, errors.Trace(err)
	}
	txns := make([]txn.Op, len(docs))
	for i, d := range docs {
		fmt.Println(d)
		existingDoc, err := st.getId(d.Id)
		if err == mgo.ErrNotFound {
			fmt.Println("insert")
			txns[i] = insertMetricSummaryOps(d)
			continue
		}
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		isAbsolute, err := st.isAbsoluteMetric(existingDoc.CharmURL(), existingDoc.Key())
		if err != nil {
			return nil, errors.Trace(err)
		}
		fmt.Println(isAbsolute)
		if isAbsolute {
			ops, err := updateAbsoluteMetricSummaryOps(existingDoc.doc, d)
			if err != nil {
				return nil, errors.Trace(err)
			}
			txns[i] = ops
		} else {
			txns[i] = updateGuageMetricSummaryOps(d)
		}
	}
	fmt.Println(txns)
	return txns, nil
}

func (st *State) isAbsoluteMetric(charmURL, metricKey string) (bool, error) {
	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return false, errors.Trace(err)
	}
	ch, err := st.Charm(curl)
	if err != nil {
		return false, errors.Trace(err)
	}
	metric, ok := ch.Metrics().Metrics[metricKey]
	if !ok {
		return false, errors.NotFoundf("metric %q for charm %q", metricKey, charmURL)
	}
	return metric.Type == charm.MetricTypeAbsolute, nil
}

func updateGuageMetricSummaryOps(s metricSummaryDoc) txn.Op {
	return txn.Op{
		C:      metricSummaryC,
		Id:     s.Id,
		Assert: txn.DocExists,
		Update: bson.M{"$set": bson.M{
			"key":           s.Key,
			"value":         s.Value,
			"time":          s.Time,
			"absolutetotal": s.Value,
		}},
	}
}

func updateAbsoluteMetricSummaryOps(existingDoc, newDoc metricSummaryDoc) (txn.Op, error) {
	newValue, err := strconv.ParseFloat(newDoc.Value, 64)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	sum := existingDoc.AbsoluteTotal + newValue
	return txn.Op{
		C:      metricSummaryC,
		Id:     existingDoc.Id,
		Assert: bson.D{{"txn-revno", existingDoc.TxnRevno}},
		Update: bson.M{"$set": bson.M{
			"time":          newDoc.Time,
			"value":         strconv.FormatFloat(sum, 'f', 2, 64),
			"absolutetotal": sum,
		}},
	}, nil
}

func insertMetricSummaryOps(s metricSummaryDoc) txn.Op {
	return txn.Op{
		C:      metricSummaryC,
		Id:     s.Id,
		Assert: txn.DocMissing,
		Insert: s,
	}
}

func (st *State) queryMetricSummaries(query bson.M) ([]MetricSummary, error) {
	c, closer := st.getCollection(metricSummaryC)
	defer closer()
	docs := []metricSummaryDoc{}
	err := c.Find(query).Sort("time").All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]MetricSummary, len(docs))
	for i, doc := range docs {
		results[i] = MetricSummary{doc: doc}
	}
	return results, nil
}
