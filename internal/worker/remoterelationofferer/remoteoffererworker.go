// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationofferer

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
)

// RemoteOffererWorkerConfig defines the configuration for a remote offering
// worker.
type RemoteOffererWorkerConfig struct {
	OfferUUID       string
	ApplicationName string
	LocalModelUUID  model.UUID
	RemoteModelUUID string
	ConsumeVersion  int
	Clock           clock.Clock
	Logger          logger.Logger
}

// remoteOffererWorker listens for localChanges to relations
// involving a remote application, and publishes change to
// local relation units to the remote model. It also watches for
// changes originating from the offering model and consumes those
// in the local model.
type remoteOffererWorker struct {
	catacomb catacomb.Catacomb

	offerUUID      string
	consumeVersion int
}

// NewRemoteApplicationWorker creates a new remote application worker.
func NewRemoteApplicationWorker(config RemoteOffererWorkerConfig) (ReportableWorker, error) {
	w := &remoteOffererWorker{}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "remote-offerer",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill is defined on worker.Worker
func (w *remoteOffererWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *remoteOffererWorker) Wait() error {
	return w.catacomb.Wait()
}

// OfferUUID returns the offer UUID for the remote application worker.
func (w *remoteOffererWorker) OfferUUID() string {
	return w.offerUUID
}

// ConsumeVersion returns the consume version for the remote application worker.
func (w *remoteOffererWorker) ConsumeVersion() int {
	return w.consumeVersion
}

// Report provides information for the engine report.
func (w *remoteOffererWorker) Report() map[string]interface{} {
	result := make(map[string]interface{})
	return result
}

func (w *remoteOffererWorker) loop() (err error) {
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}
