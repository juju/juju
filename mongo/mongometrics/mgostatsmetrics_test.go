// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package mongometrics_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo/mongometrics"
)

type MgoStatsCollectorSuite struct {
	testing.IsolationSuite
	collector       *mongometrics.MgoStatsCollector
	getCurrentStats func() mgo.Stats
}

var _ = gc.Suite(&MgoStatsCollectorSuite{})

func (s *MgoStatsCollectorSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.getCurrentStats = func() mgo.Stats {
		return mgo.Stats{}
	}
	s.collector = mongometrics.NewMgoStatsCollector(func() mgo.Stats {
		return s.getCurrentStats()
	})
}

func (s *MgoStatsCollectorSuite) TestDescribe(c *gc.C) {
	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}
	c.Assert(descs, gc.HasLen, 9)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "mgo_clusters".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "mgo_master_conns".*`)
	c.Assert(descs[2].String(), gc.Matches, `.*fqName: "mgo_slave_conns".*`)
	c.Assert(descs[3].String(), gc.Matches, `.*fqName: "mgo_sent_ops_total".*`)
	c.Assert(descs[4].String(), gc.Matches, `.*fqName: "mgo_received_ops_total".*`)
	c.Assert(descs[5].String(), gc.Matches, `.*fqName: "mgo_received_docs_total".*`)
	c.Assert(descs[6].String(), gc.Matches, `.*fqName: "mgo_sockets_alive".*`)
	c.Assert(descs[7].String(), gc.Matches, `.*fqName: "mgo_sockets_inuse".*`)
	c.Assert(descs[8].String(), gc.Matches, `.*fqName: "mgo_socket_refs".*`)
}

func (s *MgoStatsCollectorSuite) TestCollect(c *gc.C) {
	s.getCurrentStats = func() mgo.Stats {
		return mgo.Stats{
			Clusters:     1,
			MasterConns:  2,
			SlaveConns:   3,
			SentOps:      4,
			ReceivedOps:  5,
			ReceivedDocs: 6,
			SocketsAlive: 7,
			SocketsInUse: 8,
			SocketRefs:   9,
		}
	}

	stringptr := func(v string) *string {
		return &v
	}
	float64ptr := func(v float64) *float64 {
		return &v
	}
	metricTypePtr := func(v dto.MetricType) *dto.MetricType {
		return &v
	}

	registry := prometheus.NewPedanticRegistry()
	registry.Register(s.collector)
	metricFamilies, err := registry.Gather()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricFamilies, gc.HasLen, 9)
	c.Assert(metricFamilies, jc.DeepEquals, []*dto.MetricFamily{{
		Name: stringptr("mgo_clusters"),
		Help: stringptr("Current number of clusters"),
		Type: metricTypePtr(dto.MetricType_GAUGE),
		Metric: []*dto.Metric{{
			Gauge: &dto.Gauge{Value: float64ptr(1)},
		}},
	}, {
		Name: stringptr("mgo_master_conns"),
		Help: stringptr("Current number of master conns"),
		Type: metricTypePtr(dto.MetricType_GAUGE),
		Metric: []*dto.Metric{{
			Gauge: &dto.Gauge{Value: float64ptr(2)},
		}},
	}, {
		Name: stringptr("mgo_received_docs_total"),
		Help: stringptr("Total number of received docs"),
		Type: metricTypePtr(dto.MetricType_COUNTER),
		Metric: []*dto.Metric{{
			Counter: &dto.Counter{Value: float64ptr(6)},
		}},
	}, {
		Name: stringptr("mgo_received_ops_total"),
		Help: stringptr("Total number of received ops"),
		Type: metricTypePtr(dto.MetricType_COUNTER),
		Metric: []*dto.Metric{{
			Counter: &dto.Counter{Value: float64ptr(5)},
		}},
	}, {
		Name: stringptr("mgo_sent_ops_total"),
		Help: stringptr("Total number of sent ops"),
		Type: metricTypePtr(dto.MetricType_COUNTER),
		Metric: []*dto.Metric{{
			Counter: &dto.Counter{Value: float64ptr(4)},
		}},
	}, {
		Name: stringptr("mgo_slave_conns"),
		Help: stringptr("Current number of slave conns"),
		Type: metricTypePtr(dto.MetricType_GAUGE),
		Metric: []*dto.Metric{{
			Gauge: &dto.Gauge{Value: float64ptr(3)},
		}},
	}, {
		Name: stringptr("mgo_socket_refs"),
		Help: stringptr("Current number of sockets referenced"),
		Type: metricTypePtr(dto.MetricType_GAUGE),
		Metric: []*dto.Metric{{
			Gauge: &dto.Gauge{Value: float64ptr(9)},
		}},
	}, {
		Name: stringptr("mgo_sockets_alive"),
		Help: stringptr("Current number of sockets alive"),
		Type: metricTypePtr(dto.MetricType_GAUGE),
		Metric: []*dto.Metric{{
			Gauge: &dto.Gauge{Value: float64ptr(7)},
		}},
	}, {
		Name: stringptr("mgo_sockets_inuse"),
		Help: stringptr("Current number of sockets in use"),
		Type: metricTypePtr(dto.MetricType_GAUGE),
		Metric: []*dto.Metric{{
			Gauge: &dto.Gauge{Value: float64ptr(8)},
		}},
	}})
}

func (s *MgoStatsCollectorSuite) TestCollectCounterDelta(c *gc.C) {
	var sentOps int
	s.getCurrentStats = func() mgo.Stats {
		return mgo.Stats{SentOps: sentOps}
	}
	float64ptr := func(v float64) *float64 {
		return &v
	}
	registry := prometheus.NewPedanticRegistry()
	registry.Register(s.collector)

	sentOps = 1
	metricFamilies, err := registry.Gather()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricFamilies[4].Metric[0].Counter.Value, jc.DeepEquals, float64ptr(1))

	sentOps = 3
	metricFamilies, err = registry.Gather()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metricFamilies[4].Metric[0].Counter.Value, jc.DeepEquals, float64ptr(3))
}
