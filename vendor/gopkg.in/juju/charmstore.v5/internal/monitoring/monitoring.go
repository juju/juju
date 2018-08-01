// Copyright 2016 Canonical Ltd.

package monitoring // import "gopkg.in/juju/charmstore.v5/internal/monitoring"

import (
	"github.com/cloud-green/monitoring"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	requestDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "charmstore",
		Subsystem: "handler",
		Name:      "request_duration",
		Help:      "The duration of a web request in seconds.",
	}, []string{"method", "root", "kind"})

	uploadProcessingDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace: "charmstore",
		Subsystem: "archive",
		Name:      "processing_duration",
		Help:      "The processing duration of a charm upload in seconds.",
	})

	blobstoreGCDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace: "charmstore",
		Subsystem: "archive",
		Name:      "blobstore_gc_duration",
		Help:      "The processing duration a garbage collection in seconds",
	})

	blobCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "charmstore",
		Subsystem: "archive",
		Name:      "blob_count",
		Help:      "The total number of stored blobs.",
	})

	maxBlobSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "charmstore",
		Subsystem: "archive",
		Name:      "max_blob_size",
		Help:      "The maximum stored blob size",
	})

	meanBlobSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "charmstore",
		Subsystem: "archive",
		Name:      "mean_blob_size",
		Help:      "The mean stored blob size",
	})
)

// BlobStats holds statistics about blobs in the blob store.
type BlobStats struct {
	// Count holds the total number of blobs stored.
	Count int
	// MaxSize holds the size of the largest blob.
	MaxSize int64
	// MeanSize holds the average blob size.
	MeanSize int64
	// TODO add counts/sizes for different
	// kinds of blob?
}

func SetBlobStoreStats(s BlobStats) {
	blobCount.Set(float64(s.Count))
	maxBlobSize.Set(float64(s.MaxSize))
	meanBlobSize.Set(float64(s.MeanSize))
}

func init() {
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(uploadProcessingDuration)
	prometheus.MustRegister(blobstoreGCDuration)
	prometheus.MustRegister(blobCount)
	prometheus.MustRegister(maxBlobSize)
	prometheus.MustRegister(meanBlobSize)
	prometheus.MustRegister(monitoring.NewMgoStatsCollector("charmstore"))
}
