// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

const (
	// ErrRetryable is returned when a retryable error occurs.
	ErrRetryable = errors.ConstError("retryable")
)

// If we only receive one validation change request, then we need to keep
// retrying until we successfully connect to the target controller, or we fail.
// Constantly, retrying at a shorter interval is not a good idea, as it can
// cause the target controller to be flooded with requests (DDoS). The default
// time to wait for the migration validation to occur is 15 minutes. This time
// can be changed by the user. The retry time should be increased exponentially,
// along with additional jitter, to avoid flooding the target all at the same
// time.
//
// The strategy below without the jitter is:
//
//   - 100ms
//   - 160ms
//   - 256ms
//   - 410ms
//   - 655ms
//   - 1.049s
//   - 1.678s
//   - 2.684s
//   - 4.295s
//   - 6.872s
//   - 10.995s
//   - 17.592s
//   - 25s
//   - 25s
//   - 25s
//   - 25s
//   - 25s
//   - 25s
//   - 25s
//   - 25s
//
// With the total being: 4m6.746s. If we factor in jitter swing, that will give
// us roughly the 5m max duration. Thus giving us a good balance between
// retrying and not flooding the target controller.
//
// If the migration master does illicit another retry, even after the max
// duration has been reached, this should give us at least 1 more retry before
// the migration master gives up.

const (
	// maxRetries is the number of times we'll attempt validation
	// before giving up.
	maxRetries = 20

	// initialRetryDelay is the starting delay - this will be
	// increased exponentially up maxRetries.
	initialRetryDelay = 100 * time.Millisecond

	// retryMaxDelay is the maximum delay we'll wait between retries.
	retryMaxDelay = 25 * time.Second

	// retryMaxDuration is the maximum time we'll spend retrying the validation
	// before giving up.
	retryMaxDuration = 5 * time.Minute

	// retryExpBackoff is the exponential backoff factor for retrying the
	// validation.
	retryExpBackoff = 1.6
)

// Facade exposes controller functionality to a Worker.
type Facade interface {
	Watch() (watcher.MigrationStatusWatcher, error)
	Report(migrationId string, phase migration.Phase, success bool) error
}

// Config defines the operation of a Worker.
type Config struct {
	Agent             agent.Agent
	Facade            Facade
	Guard             fortress.Guard
	Clock             clock.Clock
	APIOpen           func(*api.Info, api.DialOpts) (api.Connection, error)
	ValidateMigration func(base.APICaller) error
	NewFacade         func(base.APICaller) (Facade, error)
	Logger            Logger

	// ApplyJitter indicates whether to apply jitter to the retry
	// backoff. This is useful when retrying validation requests to
	// avoid flooding the target controller.
	ApplyJitter bool
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Guard == nil {
		return errors.NotValidf("nil Guard")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.APIOpen == nil {
		return errors.NotValidf("nil APIOpen")
	}
	if config.ValidateMigration == nil {
		return errors.NotValidf("nil ValidateMigration")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// New returns a Worker backed by config, or an error.
func New(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &Worker{
		config:    config,
		processed: make(map[string]migration.Phase),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker waits for a model migration to be active, then locks down the
// configured fortress and implements the migration.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	processed map[string]migration.Phase
}

// Kill implements worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	watch, err := w.config.Facade.Watch()
	if err != nil {
		return errors.Annotate(err, "setting up watcher")
	}
	if err := w.catacomb.Add(watch); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case status, ok := <-watch.Changes():
			if !ok {
				return errors.New("watcher channel closed")
			}

			if err := w.handle(status); err != nil {
				w.config.Logger.Errorf("handling migration phase %s failed: %v", status.Phase, err)
				return errors.Trace(err)
			}
		}
	}
}

func (w *Worker) handle(status watcher.MigrationStatus) error {
	w.config.Logger.Infof("migration phase is now: %s", status.Phase)

	if !status.Phase.IsRunning() {
		// If the phase is not running, we can unlock the fortress, but remove
		// the migration from the processed map first.
		delete(w.processed, status.MigrationId)

		return w.config.Guard.Unlock()
	}

	// We've already processed this phase, so we can ignore it.
	// It's important to do this before we lockdown the fortress, as we want
	// to pretend that we've never seen this message.
	if p, ok := w.processed[status.MigrationId]; ok && p == status.Phase {
		return nil
	}

	// Ensure that all workers related to migration fortress have
	// stopped and aren't allowed to restart.
	err := w.config.Guard.Lockdown(w.catacomb.Dying())
	if errors.Cause(err) == fortress.ErrAborted {
		return w.catacomb.ErrDying()
	} else if err != nil {
		return errors.Trace(err)
	}

	switch status.Phase {
	case migration.QUIESCE:
		err = w.doQUIESCE(status)
	case migration.VALIDATION:
		err = w.doVALIDATION(status)
	case migration.SUCCESS:
		err = w.doSUCCESS(status)
	default:
		// The minion doesn't need to do anything for other
		// migration phases.
	}

	// If the error is ErrRetryable, then don't record the phase as processed.
	// This will allow the worker to retry the phase again.
	if errors.Is(err, ErrRetryable) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// Prevent unbounded growth of the processed map, and remove the phase
	// from the map if it's terminal.
	if status.Phase.IsTerminal() {
		delete(w.processed, status.MigrationId)
	} else {
		w.processed[status.MigrationId] = status.Phase
	}
	return nil
}

func (w *Worker) doQUIESCE(status watcher.MigrationStatus) error {
	// Report that the minion is ready and that all workers that
	// should be shut down have done so.
	return w.report(status, true)
}

func (w *Worker) doVALIDATION(status watcher.MigrationStatus) error {
	// Attempt the validation multiple times, with exponential backoff.
	// If this fails, that's it, we can't proceed. There isn't a guarantee
	// that we'll get another change event to retry.
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			return w.validate(status)
		},
		NotifyFunc: func(lastError error, attempt int) {
			w.config.Logger.Warningf("validation failed (attempt %d): %v", attempt, lastError)
		},
		Clock:       w.config.Clock,
		Attempts:    maxRetries,
		Delay:       initialRetryDelay,
		MaxDuration: retryMaxDuration,
		BackoffFunc: retry.ExpBackoff(initialRetryDelay, retryMaxDelay, retryExpBackoff, w.config.ApplyJitter),
		Stop:        w.catacomb.Dying(),
	})
	if errors.Is(err, apiservererrors.ErrTryAgain) || params.IsCodeTryAgain(err) {
		// Provide additional context about why the error occurred in the logs.
		// Then report the error to the migrationmaster.
		w.config.Logger.Warningf(`validation failed: try changing "agent-ratelimit-max" and "agent-ratelimit-rate", before trying again: %v`, err)
		return ErrRetryable
	} else if err != nil {
		// Don't return this error just log it and report to the
		// migrationmaster that things didn't work out.
		w.config.Logger.Errorf("validation failed: %v", err)
	}

	// Report the result of the validation.
	return w.report(status, err == nil)
}

func (w *Worker) validate(status watcher.MigrationStatus) error {
	agentConf := w.config.Agent.CurrentConfig()
	apiInfo, ok := agentConf.APIInfo()
	if !ok {
		return errors.New("no API connection details")
	}
	apiInfo.Addrs = status.TargetAPIAddrs
	apiInfo.CACert = status.TargetCACert
	// Application agents (k8s) use old password.
	if apiInfo.Password == "" {
		apiInfo.Password = agentConf.OldPassword()
	}

	// Use zero DialOpts (no retries) because the worker must stay
	// responsive to Kill requests. We don't want it to be blocked by
	// a long set of retry attempts.
	conn, err := w.config.APIOpen(apiInfo, api.DialOpts{})
	if err != nil {
		return errors.Annotate(err, "failed to open API to target controller")
	}
	defer func() { _ = conn.Close() }()

	// Ask the agent to confirm that things look ok.
	err = w.config.ValidateMigration(conn)
	return errors.Trace(err)
}

func (w *Worker) doSUCCESS(status watcher.MigrationStatus) (err error) {
	defer func() {
		if err != nil {
			cfg := w.config.Agent.CurrentConfig()
			w.config.Logger.Criticalf("migration failed for %v: %s/agent.conf left unchanged and pointing to source controller: %v",
				cfg.Tag(), cfg.Dir(), err,
			)
		}
	}()
	hps, err := network.ParseProviderHostPorts(status.TargetAPIAddrs...)
	if err != nil {
		return errors.Annotate(err, "converting API addresses")
	}

	// Report first because the config update that's about to happen
	// will cause the API connection to drop. The SUCCESS phase is the
	// point of no return anyway, so we must retry this step even if
	// the api connection dies.
	if err := w.robustReport(status, true); err != nil {
		return errors.Trace(err)
	}

	err = w.config.Agent.ChangeConfig(func(conf agent.ConfigSetter) error {
		err := conf.SetAPIHostPorts([]network.HostPorts{hps.HostPorts()})
		if err != nil {
			return errors.Trace(err)
		}
		conf.SetCACert(status.TargetCACert)
		return nil
	})
	return errors.Annotate(err, "setting agent config")
}

func (w *Worker) report(status watcher.MigrationStatus, success bool) error {
	w.config.Logger.Infof("reporting back for phase %s: %v", status.Phase, success)
	err := w.config.Facade.Report(status.MigrationId, status.Phase, success)
	return errors.Annotate(err, "failed to report phase progress")
}

func (w *Worker) robustReport(status watcher.MigrationStatus, success bool) error {
	err := w.report(status, success)
	if err != nil && !rpc.IsShutdownErr(err) {
		return fmt.Errorf("cannot report migration status %v success=%v: %w", status, success, err)
	} else if err == nil {
		return nil
	}
	w.config.Logger.Warningf("report migration status failed: %v", err)

	apiInfo, ok := w.config.Agent.CurrentConfig().APIInfo()
	if !ok {
		return fmt.Errorf("cannot report migration status %v success=%v: no API connection details", status, success)
	}
	apiInfo.Addrs = status.SourceAPIAddrs
	apiInfo.CACert = status.SourceCACert

	err = retry.Call(retry.CallArgs{
		Func: func() error {
			w.config.Logger.Infof("reporting back for phase %s: %v", status.Phase, success)

			conn, err := w.config.APIOpen(apiInfo, api.DialOpts{})
			if err != nil {
				return fmt.Errorf("cannot dial source controller: %w", err)
			}
			defer func() { _ = conn.Close() }()

			facade, err := w.config.NewFacade(conn)
			if err != nil {
				return err
			}

			return facade.Report(status.MigrationId, status.Phase, success)
		},
		IsFatalError: func(err error) bool {
			return false
		},
		NotifyFunc: func(lastError error, attempt int) {
			w.config.Logger.Warningf("report migration status failed (attempt %d): %v", attempt, lastError)
		},
		Clock:       w.config.Clock,
		Delay:       initialRetryDelay,
		Attempts:    maxRetries,
		BackoffFunc: retry.DoubleDelay,
		Stop:        w.catacomb.Dying(),
	})
	if err != nil {
		return fmt.Errorf("cannot report migration status %v success=%v: %w", status, success, err)
	}
	return nil
}
