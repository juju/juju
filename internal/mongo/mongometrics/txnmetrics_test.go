// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package mongometrics_test

import (
	"errors"
	"reflect"
	"time"

	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/mongo/mongometrics"
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
	c.Assert(descs, gc.HasLen, 3)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_mgo_txn_ops_total".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "juju_mgo_txn_retries".*`)
	c.Assert(descs[2].String(), gc.Matches, `.*fqName: "juju_mgo_txn_durations".*`)
}

func (s *TxnCollectorSuite) TestCollect(c *gc.C) {
	s.collector.AfterRunTransaction("dbname", "modeluuid", 1, time.Millisecond, []txn.Op{{
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

	s.collector.AfterRunTransaction("dbname", "modeluuid", 1, time.Millisecond, []txn.Op{{
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
	c.Assert(metrics, gc.HasLen, 7)

	var dtoMetrics [7]dto.Metric
	for i, metric := range metrics {
		err := metric.Write(&dtoMetrics[i])
		c.Assert(err, jc.ErrorIsNil)
	}

	float64ptr := func(v float64) *float64 {
		return &v
	}
	uint64ptr := func(v uint64) *uint64 {
		return &v
	}
	labelpair := func(n, v string) *dto.LabelPair {
		return &dto.LabelPair{Name: &n, Value: &v}
	}
	var retryBuckets []*dto.Bucket
	for i := 0; i < 50; i++ {
		count := uint64(0)
		if i > 0 {
			count = 5
		}
		retryBuckets = append(retryBuckets, &dto.Bucket{
			CumulativeCount: uint64ptr(count),
			UpperBound:      float64ptr(float64(i)),
		})
	}
	var durationBuckets []*dto.Bucket
	for i := 0; i < 50; i++ {
		count := uint64(0)
		if i > 0 {
			count = 5
		}
		durationBuckets = append(durationBuckets, &dto.Bucket{
			CumulativeCount: uint64ptr(count),
			UpperBound:      float64ptr(float64(2 * i)),
		})
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
		{
			Histogram: &dto.Histogram{
				SampleCount: uint64ptr(5),
				SampleSum:   float64ptr(5),
				Bucket:      retryBuckets,
			},
		},
		{
			Histogram: &dto.Histogram{
				SampleCount: uint64ptr(5),
				SampleSum:   float64ptr(5),
				Bucket:      durationBuckets,
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
