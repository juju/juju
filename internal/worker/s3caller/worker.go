// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

func newS3ClientWorker(session objectstore.Session) worker.Worker {
	w := &s3ClientWorker{session: session}
	w.tomb.Go(w.loop)
	return w
}

type s3ClientWorker struct {
	tomb    tomb.Tomb
	session objectstore.Session
}

// Kill is part of the worker.Worker interface.
func (w *s3ClientWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *s3ClientWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *s3ClientWorker) loop() (err error) {
	<-w.tomb.Dying()
	return tomb.ErrDying
}
