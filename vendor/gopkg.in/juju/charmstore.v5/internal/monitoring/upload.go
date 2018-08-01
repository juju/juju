// Copyright 2016 Canonical Ltd.

package monitoring // import "gopkg.in/juju/charmstore.v5/internal/monitoring"

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// NewUploadProcessingDuration returns a new
// Duration to be used for measuring the time taken
// to process an upload.
func NewUploadProcessingDuration() *Duration {
	return newDuration(uploadProcessingDuration)
}

// NewBlobstoreGCDuration returns a new
// Duration to be used for measuring the time taken
// to run the blobstore garbage collector.
func NewBlobstoreGCDuration() *Duration {
	return newDuration(blobstoreGCDuration)
}

// Duration represents a time duration to be montored.
// The duration starts when the Duration is created
// and finishes when Done is called.
type Duration struct {
	metric    prometheus.Summary
	startTime time.Time
}

// Done observes the duration as a metric.
// It should only be called once.
func (d *Duration) Done() {
	d.metric.Observe(float64(time.Since(d.startTime)) / float64(time.Microsecond))
}

func newDuration(metric prometheus.Summary) *Duration {
	return &Duration{
		metric:    metric,
		startTime: time.Now(),
	}
}
