// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5/internal/charmstore"

import (
	"time"

	"gopkg.in/errgo.v1"
	tomb "gopkg.in/tomb.v2"

	"gopkg.in/juju/charmstore.v5/internal/monitoring"
)

var gcInterval = time.Hour

// blobstoreGC implements the worker that runs the blobstore
// garbage collector.
type blobstoreGC struct {
	tomb tomb.Tomb
	pool *Pool
}

// newBlobstoreGC returns a new running blobstore garbage
// collector worker.
func newBlobstoreGC(pool *Pool) *blobstoreGC {
	gc := &blobstoreGC{
		pool: pool,
	}
	gc.tomb.Go(gc.run)
	return gc
}

// Kill implements worker.Worker.Kill.
func (gc *blobstoreGC) Kill() {
	gc.tomb.Kill(nil)
}

// Kill implements worker.Worker.Wait.
func (gc *blobstoreGC) Wait() error {
	return gc.tomb.Wait()
}

func (gc *blobstoreGC) run() error {
	for {
		gcDuration := monitoring.NewBlobstoreGCDuration()
		logger.Infof("starting blobstore garbage collection")
		if err := gc.doGC(); err != nil {
			// Note: don't log the duration when there's an error.
			logger.Errorf("%v", err)
		} else {
			logger.Infof("completed blobstore garbage collection")
			gcDuration.Done()
		}
		select {
		case <-gc.tomb.Dying():
			return tomb.ErrDying
		case <-time.After(gcInterval):
		}
	}
}

func (gc *blobstoreGC) doGC() error {
	store := gc.pool.Store()
	defer store.Close()
	err := store.BlobStore.RemoveExpiredUploads()
	if err != nil {
		return errgo.Notef(err, "expired-upload garbage collection failed")
	}
	err = store.BlobStoreGC(time.Now().Add(-30 * time.Minute))
	if err != nil {
		return errgo.Notef(err, "blob garbage collection failed")
	}
	return nil
}
