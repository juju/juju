// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongometrics

import (
	"time"

	"github.com/juju/mgo/v3/txn"
	"github.com/prometheus/client_golang/prometheus"
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

// TxnCollector is a prometheus.Collector that collects metrics about
// mgo/txn operations.
type TxnCollector struct {
	txnOpsTotalCounter *prometheus.CounterVec
	txnRetries         prometheus.Histogram
	txnDurations       prometheus.Histogram
}

// NewTxnCollector returns a new TxnCollector.
func NewTxnCollector() *TxnCollector {
	return &TxnCollector{
		txnOpsTotalCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "juju",
				Name:      "mgo_txn_ops_total",
				Help:      "Total number of mgo/txn ops executed.",
			},
			jujuMgoTxnLabelNames,
		),
		txnRetries: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "juju",
				Name:      "mgo_txn_retries",
				Help:      "Number of attempts to complete a transaction",
				Buckets:   prometheus.LinearBuckets(0, 1, 50),
			},
		),
		txnDurations: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "juju",
				Name:      "mgo_txn_durations",
				Help:      "Time (ms) taken to complete a transaction",
				Buckets:   prometheus.LinearBuckets(0, 2, 50),
			},
		),
	}
}

// AfterRunTransaction is called when a mgo/txn transaction has run.
func (c *TxnCollector) AfterRunTransaction(dbName, modelUUID string, attempt int, duration time.Duration, ops []txn.Op, err error) {
	for _, op := range ops {
		c.updateMetrics(dbName, attempt, duration, op, err)
	}
}

func (c *TxnCollector) updateMetrics(dbName string, attempt int, duration time.Duration, op txn.Op, err error) {
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
	c.txnRetries.Observe(float64(attempt))
	c.txnDurations.Observe(float64(duration / time.Millisecond))
}

// Describe is part of the prometheus.Collector interface.
func (c *TxnCollector) Describe(ch chan<- *prometheus.Desc) {
	c.txnOpsTotalCounter.Describe(ch)
	c.txnRetries.Describe(ch)
	c.txnDurations.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *TxnCollector) Collect(ch chan<- prometheus.Metric) {
	c.txnOpsTotalCounter.Collect(ch)
	c.txnRetries.Collect(ch)
	c.txnDurations.Collect(ch)
}
