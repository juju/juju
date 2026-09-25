// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// RemoteModelRelationsFacadeCloser implements RemoteModelRelationsFacade
// and add a Close() method.
type RemoteModelRelationsFacadeCloser interface {
	io.Closer
	RemoteModelRelationsFacade
}

// RemoteModelRelationsFacade instances publish local relation changes to the
// model hosting the remote application involved in the relation, and also watches
// for remote relation changes which are then pushed to the local model.
type RemoteModelRelationsFacade interface {
	// RegisterRemoteRelations sets up the remote model to participate
	// in the specified relations.
	RegisterRemoteRelations(relations ...params.RegisterRemoteRelationArg) ([]params.RegisterRemoteRelationResult, error)

	// PublishRelationChange publishes relation changes to the
	// model hosting the remote application involved in the relation.
	PublishRelationChange(params.RemoteRelationChangeEvent) error

	// WatchRelationChanges returns a watcher that notifies of changes
	// to the units in the remote model for the relation with the
	// given remote token. We need to pass the application token for
	// the case where we're talking to a v1 API and the client needs
	// to convert RelationUnitsChanges into RemoteRelationChangeEvents
	// as they come in.
	WatchRelationChanges(relationToken, applicationToken string, macs macaroon.Slice) (apiwatcher.RemoteRelationWatcher, error)

	// WatchRelationSuspendedStatus starts a RelationStatusWatcher for watching the
	// relations of each specified application in the remote model.
	WatchRelationSuspendedStatus(arg params.RemoteEntityArg) (watcher.RelationStatusWatcher, error)

	// WatchOfferStatus starts an OfferStatusWatcher for watching the status
	// of the specified offer in the remote model.
	WatchOfferStatus(arg params.OfferArg) (watcher.OfferStatusWatcher, error)

	// WatchConsumedSecretsChanges starts a watcher for any changes to secrets
	// consumed by the specified application.
	WatchConsumedSecretsChanges(applicationToken, relationToken string, mac *macaroon.Macaroon) (watcher.SecretsRevisionWatcher, error)
}

// RemoteRelationsFacade exposes remote relation functionality to a worker.
type RemoteRelationsFacade interface {
	// ImportRemoteEntity adds an entity to the remote entities collection
	// with the specified opaque token.
	ImportRemoteEntity(entity names.Tag, token string) error

	// SaveMacaroon saves the macaroon for the entity.
	SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error

	// ExportEntities allocates unique, remote entity IDs for the
	// given entities in the local model.
	ExportEntities([]names.Tag) ([]params.TokenResult, error)

	// GetToken returns the token associated with the entity with the given tag.
	GetToken(names.Tag) (string, error)

	// Relations returns information about the relations
	// with the specified keys in the local model.
	Relations(keys []string) ([]params.RemoteRelationResult, error)

	// RemoteApplications returns the current state of the remote applications with
	// the specified names in the local model.
	RemoteApplications(names []string) ([]params.RemoteApplicationResult, error)

	// WatchLocalRelationChanges returns a watcher that notifies of changes to the
	// local units in the relation with the given key.
	WatchLocalRelationChanges(relationKey string) (apiwatcher.RemoteRelationWatcher, error)

	// WatchRemoteApplications watches for addition, removal and lifecycle
	// changes to remote applications known to the local model.
	WatchRemoteApplications() (watcher.StringsWatcher, error)

	// WatchRemoteApplicationRelations starts a StringsWatcher for watching the relations of
	// each specified application in the local model, and returns the watcher IDs
	// and initial values, or an error if the application's relations could not be
	// watched.
	WatchRemoteApplicationRelations(application string) (watcher.StringsWatcher, error)

	// ConsumeRemoteRelationChange consumes a change to settings originating
	// from the remote/offering side of a relation.
	ConsumeRemoteRelationChange(change params.RemoteRelationChangeEvent) error

	// ControllerAPIInfoForModel returns the controller api info for a model.
	ControllerAPIInfoForModel(modelUUID string) (*api.Info, error)

	// SetRemoteApplicationStatus sets the status for the specified remote application.
	SetRemoteApplicationStatus(applicationName string, status status.Status, message string) error

	// UpdateControllerForModel ensures that there is an external controller record
	// for the input info, associated with the input model ID.
	UpdateControllerForModel(controller crossmodel.ControllerInfo, modelUUID string) error

	// ConsumeRemoteSecretChanges updates the local model with secret revision  changes
	// originating from the remote/offering model.
	ConsumeRemoteSecretChanges(changes []watcher.SecretRevisionChange) error
}

type newRemoteRelationsFacadeFunc func(*api.Info) (RemoteModelRelationsFacadeCloser, error)

// Config defines the operation of a Worker.
type Config struct {
	ModelUUID                string
	RelationsFacade          RemoteRelationsFacade
	NewRemoteModelFacadeFunc newRemoteRelationsFacadeFunc
	Clock                    clock.Clock
	Logger                   Logger

	// Used for testing.
	Runner *worker.Runner
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.ModelUUID == "" {
		return errors.NotValidf("empty model uuid")
	}
	if config.RelationsFacade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.NewRemoteModelFacadeFunc == nil {
		return errors.NotValidf("nil Remote Model Facade func")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// New returns a Worker backed by config, or an error.
func New(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner := config.Runner
	if runner == nil {
		runner = worker.NewRunner(worker.RunnerParams{
			Clock:  config.Clock,
			Logger: config.Logger,

			// One of the remote application workers failing should not
			// prevent the others from running.
			IsFatal: func(error) bool { return false },

			// For any failures, try again in 15 seconds.
			RestartDelay: 15 * time.Second,
		})
	}
	w := &Worker{
		config:                config,
		applicationIdentities: make(map[string]remoteApplicationIdentity),
		runner:                runner,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	})
	return w, errors.Trace(err)
}

// Worker manages relations and associated settings where
// one end of the relation is a remote application.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	logger   loggo.Logger

	runner *worker.Runner
	// mu protects applicationIdentities, not worker lifecycle or API calls.
	mu sync.Mutex

	// applicationIdentities records the identity observed for each SAAS name.
	applicationIdentities map[string]remoteApplicationIdentity
}

// Kill is defined on worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker.
func (w *Worker) Wait() error {
	err := w.catacomb.Wait()
	if err != nil {
		w.logger.Errorf("error in top level remote relations worker: %v", err)
	}
	return err
}

func (w *Worker) loop() (err error) {
	changes, err := w.config.RelationsFacade.WatchRemoteApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(changes); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case applicationIds, ok := <-changes.Changes():
			if !ok {
				return errors.New("change channel closed")
			}
			err = w.handleApplicationChanges(applicationIds)
			if err != nil {
				return err
			}
		}
	}
}

func (w *Worker) handleApplicationChanges(applicationIds []string) error {
	// TODO(wallyworld) - watcher should not give empty events
	if len(applicationIds) == 0 {
		return nil
	}
	logger := w.config.Logger
	logger.Debugf("processing remote application changes for: %s", applicationIds)

	// Fetch the current state of each of the remote applications that have changed.
	results, err := w.config.RelationsFacade.RemoteApplications(applicationIds)
	if err != nil {
		return errors.Annotate(err, "querying remote applications")
	}

	for i, result := range results {
		name := applicationIds[i]

		// A name can be reused for a different offer or consume generation.
		// Stop the old worker before starting one for the replacement.
		appGone := result.Error != nil && params.IsCodeNotFound(result.Error)
		if result.Error != nil && !appGone {
			return errors.Annotatef(result.Error, "querying remote application %q", name)
		}

		var remoteApp *params.RemoteApplication
		identityChanged := false
		if !appGone {
			remoteApp = result.Result
			w.mu.Lock()
			existingIdentity, ok := w.applicationIdentities[name]
			w.mu.Unlock()
			appGone = remoteApp.Life == life.Dead
			identityChanged = ok && existingIdentity != applicationIdentity(remoteApp)
		}
		if appGone || identityChanged {
			// The remote application has been removed, stop its worker.
			logger.Debugf("saas application %q gone from offering model", name)
			err := w.runner.StopAndRemoveWorker(name, w.catacomb.Dying())
			if err != nil && !errors.IsNotFound(err) {
				w.logger.Warningf("error stopping saas worker for %q: %v", name, err)
			}
			w.mu.Lock()
			delete(w.applicationIdentities, name)
			w.mu.Unlock()
			if appGone {
				continue
			}
		}

		startFunc := func() (worker.Worker, error) {
			// The runner retains this function across restarts. Read the current
			// application instead of capturing the previous offer's credentials.
			remoteApp, err := getRemoteApplication(w.config.RelationsFacade, name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if remoteApp.Life == life.Dead {
				return nil, errors.NotFoundf("live remote application %q", name)
			}
			appWorker := &remoteApplicationWorker{
				offerUUID:                         remoteApp.OfferUUID,
				applicationName:                   remoteApp.Name,
				localModelUUID:                    w.config.ModelUUID,
				remoteModelUUID:                   remoteApp.ModelUUID,
				isConsumerProxy:                   remoteApp.IsConsumerProxy,
				consumeVersion:                    remoteApp.ConsumeVersion,
				offerMacaroon:                     remoteApp.Macaroon,
				localRelationUnitChanges:          make(chan RelationUnitChangeEvent),
				remoteRelationUnitChanges:         make(chan RelationUnitChangeEvent),
				localModelFacade:                  w.config.RelationsFacade,
				newRemoteModelRelationsFacadeFunc: w.config.NewRemoteModelFacadeFunc,
				logger:                            logger,
				clock:                             w.config.Clock,
			}
			if err := catacomb.Invoke(catacomb.Plan{
				Site: &appWorker.catacomb,
				Work: appWorker.loop,
			}); err != nil {
				return nil, errors.Trace(err)
			}
			return appWorker, nil
		}

		logger.Debugf("starting watcher for remote application %q", name)
		// Start the application worker to watch for things like new relations.
		if err := w.runner.StartWorker(name, startFunc); err != nil {
			if errors.IsAlreadyExists(err) {
				w.logger.Debugf("already running remote application worker for %q", name)
			} else if err != nil {
				return errors.Annotate(err, "error starting remote application worker")
			}
		}
		w.mu.Lock()
		w.applicationIdentities[name] = applicationIdentity(remoteApp)
		w.mu.Unlock()
	}
	return nil
}

type remoteApplicationIdentity struct {
	offerUUID      string
	consumeVersion int
	modelUUID      string
	consumerProxy  bool
}

func applicationIdentity(app *params.RemoteApplication) remoteApplicationIdentity {
	return remoteApplicationIdentity{
		offerUUID: app.OfferUUID, consumeVersion: app.ConsumeVersion,
		modelUUID: app.ModelUUID, consumerProxy: app.IsConsumerProxy,
	}
}

func getRemoteApplication(facade RemoteRelationsFacade, name string) (*params.RemoteApplication, error) {
	results, err := facade.RemoteApplications([]string{name})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results) != 1 {
		return nil, errors.Errorf("expected one remote application, got %d", len(results))
	}
	if results[0].Error != nil {
		return nil, results[0].Error
	}
	if results[0].Result == nil {
		return nil, errors.New("missing remote application result")
	}
	return results[0].Result, nil
}

// Report provides information for the engine report.
func (w *Worker) Report() map[string]interface{} {
	result := make(map[string]interface{})
	w.mu.Lock()
	names := make([]string, 0, len(w.applicationIdentities))
	for name := range w.applicationIdentities {
		names = append(names, name)
	}
	w.mu.Unlock()

	// Reporting must not wait for a worker's startup API call or restart delay.
	abort := make(chan struct{})
	close(abort)
	saasWorkers := make(map[string]interface{})
	for _, name := range names {
		appWorker, err := w.runner.Worker(name, abort)
		if err == worker.ErrAborted {
			saasWorkers[name] = map[string]interface{}{"state": "starting"}
			continue
		}
		if err != nil {
			saasWorkers[name] = fmt.Sprintf("ERROR: %v", err)
			continue
		}
		saasWorkers[name] = appWorker.(worker.Reporter).Report()
	}
	result["workers"] = saasWorkers
	return result
}
