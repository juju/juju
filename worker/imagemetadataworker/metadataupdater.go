// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadataworker

import (
	"time"

	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/worker"
)

// updatePublicImageMetadataPeriod is how frequently we check for
// public image metadata updates.
const updatePublicImageMetadataPeriod = time.Hour * 24

// NewWorker returns a worker that lists published cloud
// images metadata, and records them in state.
func NewWorker(cl *imagemetadata.Client) worker.Worker {
	// TODO (anastasiamac 2015-09-02) Bug#1491353 - don't ignore stop channel.
	f := func(stop <-chan struct{}) error {
		return cl.UpdateFromPublishedImages()
	}
	return worker.NewPeriodicWorker(f, updatePublicImageMetadataPeriod, worker.NewTimer)
}
