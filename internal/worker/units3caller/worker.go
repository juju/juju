// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package units3caller

import (
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
)

type s3Worker struct {
	tomb    tomb.Tomb
	session objectstore.Session
}

func newS3Worker(session objectstore.Session) worker.Worker {
	w := &s3Worker{session: session}
	w.tomb.Go(w.loop)
	return w
}

// Kill is part of the worker.Worker interface.
func (w *s3Worker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *s3Worker) Wait() error {
	return w.tomb.Wait()
}

func (w *s3Worker) loop() (err error) {
	<-w.tomb.Dying()
	return tomb.ErrDying
}
