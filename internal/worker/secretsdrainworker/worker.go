// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/api/common/secretsdrain"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	jujusecrets "github.com/juju/juju/internal/secrets"
)

// SecretsDrainFacade instances provide a set of API for the worker to deal with secret drain process.
type SecretsDrainFacade interface {
	WatchSecretBackendChanged(context.Context) (watcher.NotifyWatcher, error)
	GetSecretsToDrain(context.Context) ([]coresecrets.SecretMetadataForDrain, error)
	ChangeSecretBackend(context.Context, []secretsdrain.ChangeSecretBackendArg) (secretsdrain.ChangeSecretBackendResult, error)
}

// Config defines the operation of the Worker.
type Config struct {
	SecretsDrainFacade
	Logger logger.Logger

	SecretsBackendGetter  func() (jujusecrets.BackendsClient, error)
	LeadershipTrackerFunc func() leadership.ChangeTracker
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
	if config.LeadershipTrackerFunc == nil {
		return errors.NotValidf("nil LeadershipTrackerFunc")
	}
	return nil
}

// NewWorker returns a secretsdrainworker Worker backed by config, or an error.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "secrets-drain",
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

func (w *Worker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	watcher, err := w.config.SecretsDrainFacade.WatchSecretBackendChanged(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

waitforchanges:
	for {
		select {
		case <-w.catacomb.Dying():
			return errors.Trace(w.catacomb.ErrDying())
		case _, ok := <-watcher.Changes():
			if !ok {
				return errors.New("secret backend changed watch closed")
			}
			w.config.Logger.Debugf(ctx, "got new secret backend")
			for {
				switch err := w.drainSecrets(); {
				case err == nil:
					continue waitforchanges
				case errors.Is(err, leadership.ErrLeadershipChanged):
					// If leadership changes during the drain operation,
					// we need to finish up and start again.
					w.config.Logger.Warningf(ctx, "leadership changed, restarting drain operation")
					continue
				default:
					return errors.Trace(err)
				}
			}
		}
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

// drainSecrets queries the secrets owned by the unit and if the unit is
// the leader, this will include any application owned secrets.
// If leadership changes during the draining operation, an error satisfying
// [leadershipworker.ErrLeadershipChanged] is returned.
func (w *Worker) drainSecrets() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	leadershipTracker := w.config.LeadershipTrackerFunc()
	drainErr := leadershipTracker.WithStableLeadership(ctx, func(ctx context.Context) error {
		secrets, err := w.config.SecretsDrainFacade.GetSecretsToDrain(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		if len(secrets) == 0 {
			w.config.Logger.Debugf(ctx, "no secrets to drain")
			return nil
		}
		w.config.Logger.Debugf(ctx, "got %d secrets to drain", len(secrets))
		backends, err := w.config.SecretsBackendGetter()
		if err != nil {
			return errors.Trace(err)
		}
		for _, md := range secrets {
			if err := w.drainSecret(ctx, md, backends); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
	if errors.Is(drainErr, context.Canceled) ||
		errors.Is(drainErr, context.DeadlineExceeded) {
		select {
		case <-w.catacomb.Dying():
			drainErr = w.catacomb.ErrDying()
		default:
		}
	}
	return drainErr
}

func (w *Worker) drainSecret(
	ctx context.Context, md coresecrets.SecretMetadataForDrain, client jujusecrets.BackendsClient,
) error {
	// Exit early if we need to abort.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	// Otherwise completely process the specified secret
	// and its revisions.
	var args []secretsdrain.ChangeSecretBackendArg
	var cleanUpInExternalBackendFuncs []func() error
	for _, revisionMeta := range md.Revisions {
		rev := revisionMeta
		// We have to get the active backend for each drain operation because the active backend
		// could be changed during the draining process.
		activeBackend, activeBackendID, err := client.GetBackend(ctx, nil, true)
		if err != nil {
			return errors.Trace(err)
		}
		if rev.ValueRef != nil && rev.ValueRef.BackendID == activeBackendID {
			w.config.Logger.Debugf(ctx, "secret %q revision %d is already on the active backend %q", md.URI, rev.Revision, activeBackendID)
			continue
		}
		w.config.Logger.Debugf(ctx, "draining %s/%d", md.URI.ID, rev.Revision)

		secretVal, err := client.GetRevisionContent(ctx, md.URI, rev.Revision)
		if err != nil {
			return errors.Trace(err)
		}
		newRevId, err := activeBackend.SaveContent(ctx, md.URI, rev.Revision, secretVal)
		if err != nil && !errors.Is(err, errors.NotSupported) {
			return errors.Trace(err)
		}
		w.config.Logger.Debugf(ctx, "saved secret %s/%d to the new backend %q, %#v", md.URI.ID, rev.Revision, activeBackendID, err)
		var newValueRef *coresecrets.ValueRef
		data := secretVal.EncodedValues()
		if err == nil {
			// We are draining to an external backend,
			newValueRef = &coresecrets.ValueRef{
				BackendID:  activeBackendID,
				RevisionID: newRevId,
			}
			// The content has successfully saved into the external backend.
			// So we won't save the content into the Juju database.
			data = nil
		}

		cleanUpInExternalBackend := func() error { return nil }
		if rev.ValueRef != nil {
			// The old backend is an external backend.
			// Note: we have to get the old backend before we make ChangeSecretBackend facade call.
			// Because the token policy(for the vault backend especially) will be changed after we changed the secret's backend.
			oldBackend, _, err := client.GetBackend(ctx, &rev.ValueRef.BackendID, true)
			if err != nil {
				return errors.Trace(err)
			}
			cleanUpInExternalBackend = func() error {
				w.config.Logger.Debugf(ctx, "cleanup secret %s/%d from old backend %q", md.URI.ID, rev.Revision, rev.ValueRef.BackendID)
				if activeBackendID == rev.ValueRef.BackendID {
					// Ideally, We should have done all these drain steps in the controller via transaction, but by design, we only allow
					// uniters to be able to access secret content. So we have to do these extra checks to avoid
					// secret gets deleted wrongly when the model's secret backend is changed back to
					// the old backend while the secret is being drained.
					return nil
				}
				err := oldBackend.DeleteContent(ctx, rev.ValueRef.RevisionID)
				if errors.Is(err, errors.NotFound) {
					// This should never happen, but if it does, we can just ignore.
					return nil
				}
				return errors.Trace(err)
			}
		}
		cleanUpInExternalBackendFuncs = append(cleanUpInExternalBackendFuncs, cleanUpInExternalBackend)
		args = append(args, secretsdrain.ChangeSecretBackendArg{
			URI:      md.URI,
			Revision: rev.Revision,
			ValueRef: newValueRef,
			Data:     data,
		})
	}
	if len(args) == 0 {
		return nil
	}

	w.config.Logger.Debugf(ctx, "content moved, updating backend info")
	results, err := w.config.SecretsDrainFacade.ChangeSecretBackend(ctx, args)
	if err != nil {
		return errors.Trace(err)
	}

	for i, err := range results.Results {
		arg := args[i]
		if err == nil {
			// We have already changed the secret to the active backend, so we
			// can clean up the secret content in the old backend now.
			if err := cleanUpInExternalBackendFuncs[i](); err != nil {
				w.config.Logger.Warningf(ctx, "failed to clean up secret %q-%d in the external backend: %v", arg.URI, arg.Revision, err)
			}
		} else {
			// If any of the ChangeSecretBackend calls failed, we will
			// bounce the agent to retry those failed tasks.
			w.config.Logger.Warningf(ctx, "failed to change secret backend for %q-%d: %v", arg.URI, arg.Revision, err)
		}
	}
	if results.ErrorCount() > 0 {
		// We got failed tasks, so we have to bounce the agent to retry those failed tasks.
		return errors.Errorf("failed to drain secret revisions for %q to the active backend", md.URI)
	}
	return nil
}
