// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretdrainworker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/api/agent/secretsdrain"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	jujusecrets "github.com/juju/juju/secrets"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// Logger represents the methods used by the worker to log information.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
}

// SecretsDrainFacade instances provide a set of API for the worker to deal with secret drain process.
type SecretsDrainFacade interface {
	WatchSecretBackendChanged() (watcher.NotifyWatcher, error)
	GetSecretsToDrain() ([]coresecrets.SecretMetadataForDrain, error)
	ChangeSecretBackend([]secretsdrain.ChangeSecretBackendArg) (secretsdrain.ChangeSecretBackendResult, error)
}

// Config defines the operation of the Worker.
type Config struct {
	SecretsDrainFacade
	Logger Logger

	SecretsBackendGetter func() (jujusecrets.BackendsClient, error)
}

// Validate returns an error if config cannot drive the Worker.
func (config Config) Validate() error {
	if config.SecretsDrainFacade == nil {
		return errors.NotValidf("nil SecretsDrainFacade")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.SecretsBackendGetter == nil {
		return errors.NotValidf("nil SecretsBackendGetter")
	}
	return nil
}

// NewWorker returns a secretdrainworker Worker backed by config, or an error.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

// Worker drains secrets to the new backend when the model's secret backend has changed.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// TODO(secrets): user created secrets should be drained on the controller because they do not have an owner unit!

// Kill is defined on worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() (err error) {
	watcher, err := w.config.SecretsDrainFacade.WatchSecretBackendChanged()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return errors.Trace(w.catacomb.ErrDying())
		case _, ok := <-watcher.Changes():
			if !ok {
				return errors.New("secret backend changed watch closed")
			}
			w.config.Logger.Debugf("got new secret backend")

			secrets, err := w.config.SecretsDrainFacade.GetSecretsToDrain()
			if err != nil {
				return errors.Trace(err)
			}
			if len(secrets) == 0 {
				w.config.Logger.Debugf("no secrets to drain")
				continue
			}
			w.config.Logger.Debugf("got %d secrets to drain", len(secrets))
			backends, err := w.config.SecretsBackendGetter()
			if err != nil {
				return errors.Trace(err)
			}
			_, activeBackendID, err := backends.GetBackend(nil)
			if err != nil {
				return errors.Trace(err)
			}
			for _, md := range secrets {
				if err := w.drainSecret(md, backends, activeBackendID); err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (w *Worker) drainSecret(
	md coresecrets.SecretMetadataForDrain,
	client jujusecrets.BackendsClient,
	activeBackendID string,
) error {
	var args []secretsdrain.ChangeSecretBackendArg
	var cleanUpInExternalBackendFuncs []func() error
	for _, revisionMeta := range md.Revisions {
		rev := revisionMeta
		if rev.ValueRef != nil && rev.ValueRef.BackendID == activeBackendID {
			// This should never happen.
			w.config.Logger.Warningf("secret %q revision %d has already been drained to the active backend %q", md.Metadata.URI, rev.Revision, activeBackendID)
			continue
		}
		w.config.Logger.Debugf("draining %s/%d", md.Metadata.URI.ID, rev.Revision)

		secretVal, err := client.GetRevisionContent(md.Metadata.URI, rev.Revision)
		if err != nil {
			return errors.Trace(err)
		}
		valueRef, err := client.SaveContent(md.Metadata.URI, rev.Revision, secretVal)
		if err != nil && !errors.Is(err, errors.NotSupported) {
			return errors.Trace(err)
		}
		var newValueRef *coresecrets.ValueRef
		data := secretVal.EncodedValues()
		if err == nil {
			// We are draining to an external backend,
			newValueRef = &valueRef
			// The content has successfully saved into the external backend.
			// So we won't save the content into the Juju database.
			data = nil
		}

		cleanUpInExternalBackend := func() error { return nil }
		if rev.ValueRef != nil {
			// The old backend is an external backend.
			// Note: we have to get the old backend before we make ChangeSecretBackend facade call.
			// Because the token policy will be changed after we changed the secret's backend.
			oldBackend, _, err := client.GetBackend(&rev.ValueRef.BackendID)
			if err != nil {
				return errors.Trace(err)
			}
			cleanUpInExternalBackend = func() error {
				err := oldBackend.DeleteContent(context.TODO(), rev.ValueRef.RevisionID)
				if errors.Is(err, errors.NotFound) {
					// This should never happen, but if it does, we can just ignore.
					return nil
				}
				return errors.Trace(err)
			}
		}
		cleanUpInExternalBackendFuncs = append(cleanUpInExternalBackendFuncs, cleanUpInExternalBackend)
		args = append(args, secretsdrain.ChangeSecretBackendArg{
			URI:      md.Metadata.URI,
			Revision: rev.Revision,
			ValueRef: newValueRef,
			Data:     data,
		})
	}
	if len(args) == 0 {
		return nil
	}

	w.config.Logger.Debugf("content moved, updating backend info")
	results, err := w.config.SecretsDrainFacade.ChangeSecretBackend(args)
	if err != nil {
		return errors.Trace(err)
	}

	for i, err := range results.Results {
		arg := args[i]
		backend := getBackend(arg.ValueRef)
		if err == nil {
			// We have already changed the secret to the active backend, so we
			// can clean up the secret content in the old backend now.
			if err := cleanUpInExternalBackendFuncs[i](); err != nil {
				w.config.Logger.Warningf(
					"failed to clean up secret %q-%d in the external backend %q: %v", arg.URI, arg.Revision, backend, err,
				)
			}
		} else {
			// If any of the ChangeSecretBackend calls failed, we will
			// bounce the agent to retry those failed tasks.
			w.config.Logger.Warningf(
				"failed to change secret backend for %q-%d to backend %q: %v", arg.URI, arg.Revision, activeBackendID, err,
			)
		}
	}
	if results.ErrorCount() > 0 {
		// We got failed tasks, so we have to bounce the agent to retry those failed tasks.
		return errors.Errorf(
			"failed to drain secret revisions for %q to the active backend %q", md.Metadata.URI, activeBackendID,
		)
	}
	return nil
}

func getBackend(ref *coresecrets.ValueRef) string {
	if ref == nil {
		return "internal"
	}
	return ref.BackendID
}
