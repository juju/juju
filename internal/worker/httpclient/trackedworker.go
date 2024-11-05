// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"net/http"

	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	internalhttp "github.com/juju/juju/internal/http"
)

type trackedWorker struct {
	tomb tomb.Tomb

	client *internalhttp.Client
}

// NewTrackedWorker creates a new tracked worker for a http client.
func NewTrackedWorker(client *internalhttp.Client) (worker.Worker, error) {

	w := &trackedWorker{
		client: client,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *trackedWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *trackedWorker) Wait() error {
	return w.tomb.Wait()
}

// Do sends an HTTP request and returns an HTTP response, following
// policy (such as redirects, cookies, auth) as configured on the client.
func (w *trackedWorker) Do(req *http.Request) (*http.Response, error) {
	return w.client.Do(req)
}

func (w *trackedWorker) loop() error {
	select {
	case <-w.tomb.Dying():
		return w.tomb.Err()
	}
}
