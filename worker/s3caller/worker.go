// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/s3client"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

func newS3ClientWorker(session s3client.Session) worker.Worker {
	w := &s3ClientWorker{session: session}
	w.tomb.Go(w.loop)
	return w
}

type s3ClientWorker struct {
	tomb    tomb.Tomb
	session s3client.Session
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
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}
