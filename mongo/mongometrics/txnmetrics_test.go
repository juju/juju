// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package mongometrics_test

import (
	"errors"
	"reflect"

	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo/mongometrics"
)

type TxnCollectorSuite struct {
	testing.IsolationSuite
	collector *mongometrics.TxnCollector
}

var _ = gc.Suite(&TxnCollectorSuite{})

func (s *TxnCollectorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.collector = mongometrics.NewTxnCollector()
}

func (s *TxnCollectorSuite) TestDescribe(c *gc.C) {
	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}
	c.Assert(descs, gc.HasLen, 1)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_mgo_txn_ops_total".*`)
}

func (s *TxnCollectorSuite) TestCollect(c *gc.C) {
	s.collector.AfterRunTransaction("dbname", "modeluuid", []txn.Op{{
		C:      "update-coll",
		Update: bson.D{},
	}, {
		C:      "insert-coll",
		Insert: bson.D{},
	}, {
		C:      "remove-coll",
		Remove: true,
	}, {
		C: "assert-coll",
	}}, nil)

	s.collector.AfterRunTransaction("dbname", "modeluuid", []txn.Op{{
		C:      "update-coll",
		Update: bson.D{},
	}}, errors.New("bewm"))

	ch := make(chan prometheus.Metric)
	go func() {
		defer close(ch)
		s.collector.Collect(ch)
	}()

	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}
	c.Assert(metrics, gc.HasLen, 5)

	var dtoMetrics [5]dto.Metric
	for i, metric := range metrics {
		err := metric.Write(&dtoMetrics[i])
		c.Assert(err, jc.ErrorIsNil)
	}

	float64ptr := func(v float64) *float64 {
		return &v
	}
	labelpair := func(n, v string) *dto.LabelPair {
		return &dto.LabelPair{Name: &n, Value: &v}
	}
	expected := []dto.Metric{
		{
			Counter: &dto.Counter{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("collection", "update-coll"),
				labelpair("database", "dbname"),
				labelpair("failed", ""),
				labelpair("optype", "update"),
			},
		},
		{
			Counter: &dto.Counter{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("collection", "insert-coll"),
				labelpair("database", "dbname"),
				labelpair("failed", ""),
				labelpair("optype", "insert"),
			},
		},
		{
			Counter: &dto.Counter{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("collection", "remove-coll"),
				labelpair("database", "dbname"),
				labelpair("failed", ""),
				labelpair("optype", "remove"),
			},
		},
		{
			Counter: &dto.Counter{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("collection", "assert-coll"),
				labelpair("database", "dbname"),
				labelpair("failed", ""),
				labelpair("optype", "assert"),
			},
		},
		{
			Counter: &dto.Counter{Value: float64ptr(1)},
			Label: []*dto.LabelPair{
				labelpair("collection", "update-coll"),
				labelpair("database", "dbname"),
				labelpair("failed", "failed"),
				labelpair("optype", "update"),
			},
		},
	}
	for _, dm := range dtoMetrics {
		var found bool
		for i, m := range expected {
			if !reflect.DeepEqual(dm, m) {
				continue
			}
			expected = append(expected[:i], expected[i+1:]...)
			found = true
			break
		}
		if !found {
			c.Errorf("metric %+v not expected", dm)
		}
	}
}
