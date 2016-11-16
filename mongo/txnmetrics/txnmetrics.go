// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txnmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/mgo.v2/txn"
)

const (
	databaseLabel   = "database"
	collectionLabel = "collection"
	optypeLabel     = "optype"
	failedLabel     = "failed"
)

var (
	jujuMgoTxnLabelNames = []string{
		databaseLabel,
		collectionLabel,
		optypeLabel,
		failedLabel,
	}
)

// Collector is a prometheus.Collector that collects metrics about
// mgo/txn operations.
type Collector struct {
	txnOpsTotalCounter *prometheus.CounterVec
}

// New returns a new Collector.
func New() *Collector {
	return &Collector{
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "juju",
				Name:      "mgo_txn_ops_total",
				Help:      "Total number of mgo/txn ops executed.",
			},
			jujuMgoTxnLabelNames,
		),
	}
}

// AfterRunTransaction is called when a mgo/txn transaction has run.
func (c *Collector) AfterRunTransaction(dbName, modelUUID string, ops []txn.Op, err error) {
	for _, op := range ops {
		c.updateMetrics(dbName, op, err)
	}
}

func (c *Collector) updateMetrics(dbName string, op txn.Op, err error) {
	var failed string
	if err != nil {
		failed = "failed"
	}
	var optype string
	switch {
	case op.Insert != nil:
		optype = "insert"
	case op.Update != nil:
		optype = "update"
	case op.Remove:
		optype = "remove"
	default:
		optype = "assert"
	}
	c.txnOpsTotalCounter.With(prometheus.Labels{
		databaseLabel:   dbName,
		collectionLabel: op.C,
		optypeLabel:     optype,
		failedLabel:     failed,
	}).Inc()
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.txnOpsTotalCounter.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.txnOpsTotalCounter.Collect(ch)
}
