// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package monitoring

import (
	"fmt"
	"sync"

	"github.com/juju/loggo"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var (
	log = loggo.GetLogger("monitoring.collectors")
)

type stats struct {
	Count uint64 `bson:"count"`
	Size  uint64 `bson:"size"`
}

// NewDBCollectionsSizeCollector returns a new collector that monitors sizes and object counts
// of all collections in the specified database.
func NewDBCollectionsSizeCollector(namespace string, session *mgo.Session, dbName string) *dbCollectionsSizeCollector {
	return &dbCollectionsSizeCollector{
		sizeDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "database", fmt.Sprintf("%s_collection_size_bytes", dbName)),
			"collection size in bytes",
			[]string{"collection"},
			nil,
		),
		countDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "database", fmt.Sprintf("%s_collection_doc_count", dbName)),
			"collection document count",
			[]string{"collection"},
			nil),
		session: session,
		dbName:  dbName,
	}
}

type dbCollectionsSizeCollector struct {
	sizeDesc  *prometheus.Desc
	countDesc *prometheus.Desc

	session *mgo.Session
	dbName  string
}

var _ prometheus.Collector = (*dbCollectionsSizeCollector)(nil)

// Describe implements the prometheus.Collector interface.
func (u *dbCollectionsSizeCollector) Describe(c chan<- *prometheus.Desc) {
	c <- u.sizeDesc
	c <- u.countDesc
}

// Collect implements the prometheus.Collector interface.
func (u *dbCollectionsSizeCollector) Collect(ch chan<- prometheus.Metric) {
	session := u.session.Copy()
	defer session.Close()

	collectionNames, err := session.DB(u.dbName).CollectionNames()
	if err != nil {
		log.Errorf("failed to get database %v collection names", u.dbName)
		return
	}

	for _, collectionName := range collectionNames {
		var stats stats
		if err := session.DB(u.dbName).Run(bson.M{"collStats": collectionName}, &stats); err != nil {
			log.Errorf("failed to report %v collection stats: %v", collectionName, err)
			return
		}

		mSize, err := prometheus.NewConstMetric(u.sizeDesc, prometheus.GaugeValue, float64(stats.Size), collectionName)
		if err != nil {
			log.Errorf("failed to report %v collection stats: %v", collectionName, err)
			return
		}
		mCount, err := prometheus.NewConstMetric(u.countDesc, prometheus.GaugeValue, float64(stats.Count), collectionName)
		if err != nil {
			log.Errorf("failed to report %v collection stats: %v", collectionName, err)
			return
		}
		ch <- mSize
		ch <- mCount
	}
}

// CollectionSizeCollector implements the prometheus.Collector interface and
// reports the size of the specified mongo collection.
type CollectionSizeCollector struct {
	sizeDesc  *prometheus.Desc
	countDesc *prometheus.Desc

	mu         sync.Mutex
	collection *mgo.Collection
	closed     bool
}

// Check implementation of prometheus.Collector interface.
var _ prometheus.Collector = (*CollectionSizeCollector)(nil)

// NewCollectionSizeCollector returns a new collector with specified properties
// that monitors the size of the given collection.
// It should be closed after use.
func NewCollectionSizeCollector(namespace, subsystem, namePrefix string, collection *mgo.Collection) *CollectionSizeCollector {
	return &CollectionSizeCollector{
		sizeDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, fmt.Sprintf("%s_collection_size_bytes", namePrefix)),
			fmt.Sprintf("%v collection size in bytes", collection.Name),
			nil,
			nil),
		countDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, subsystem, fmt.Sprintf("%s_collection_count", namePrefix)),
			fmt.Sprintf("%v collection object count", collection.Name),
			nil,
			nil),
		collection: collection.With(collection.Database.Session.Copy()),
	}
}

// Close releases all resources used by the collector.
func (u *CollectionSizeCollector) Close() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.closed {
		u.collection.Database.Session.Close()
		u.closed = true
	}
}

// Describe implements the prometheus.Collector interface.
func (u *CollectionSizeCollector) Describe(c chan<- *prometheus.Desc) {
	c <- u.sizeDesc
	c <- u.countDesc
}

// Collect implements the prometheus.Collector interface.
func (u *CollectionSizeCollector) Collect(ch chan<- prometheus.Metric) {
	var collection *mgo.Collection
	u.mu.Lock()
	if !u.closed {
		collection = u.collection.With(u.collection.Database.Session.Copy())
	}
	u.mu.Unlock()

	if collection == nil {
		log.Errorf("failed to report %v collection stats: collector is closed", u.collection.Name)
		return
	}
	defer collection.Database.Session.Close()
	var stats stats
	if err := collection.Database.Run(bson.M{"collStats": collection.Name}, &stats); err != nil {
		log.Errorf("failed to report %v collection stats: %v", u.collection.Name, err)
		return
	}

	mSize, err := prometheus.NewConstMetric(u.sizeDesc, prometheus.GaugeValue, float64(stats.Size))
	if err != nil {
		log.Errorf("failed to report %v collection stats: %v", u.collection.Name, err)
		return
	}
	mCount, err := prometheus.NewConstMetric(u.countDesc, prometheus.GaugeValue, float64(stats.Count))
	if err != nil {
		log.Errorf("failed to report %v collection stats: %v", u.collection.Name, err)
		return
	}
	ch <- mSize
	ch <- mCount
}
