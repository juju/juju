// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jujuretry "github.com/juju/retry"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/retry.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

const (
	// maxRetries is the number of times we'll attempt validation
	// before giving up.
	maxRetries = 10

	// initialRetryDelay is the starting delay - this will be
	// increased exponentially up maxRetries.
	initialRetryDelay = 100 * time.Millisecond

	// retryBackoffFactor is how much longer we wait after a failing
	// retry. Retrying 10 times starting at 100ms and backing off 1.6x
	// gives us a total delay time of about 45s.
	retryBackoffFactor = 1.6
)

// Facade exposes controller functionality to a Worker.
type Facade interface {
	Watch(context.Context) (watcher.MigrationStatusWatcher, error)
	Report(ctx context.Context, migrationId string, phase migration.Phase, success bool) error
}

// Config defines the operation of a Worker.
type Config struct {
	Agent             agent.Agent
	Facade            Facade
	Guard             fortress.Guard
	Clock             clock.Clock
	APIOpen           func(context.Context, *api.Info, api.DialOpts) (api.Connection, error)
	ValidateMigration func(context.Context, base.APICaller) error
	NewFacade         func(base.APICaller) (Facade, error)
	Logger            logger.Logger
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
	w := &Worker{config: config}
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
	ctx, cancel := w.scopedContext()
	defer cancel()

	watch, err := w.config.Facade.Watch(ctx)
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
			if err := w.handle(ctx, status); err != nil {
				w.config.Logger.Errorf(ctx, "handling migration phase %s failed: %v", status.Phase, err)
				return errors.Trace(err)
			}
		}
	}
}

func (w *Worker) handle(ctx context.Context, status watcher.MigrationStatus) error {
	w.config.Logger.Infof(ctx, "migration phase is now: %s", status.Phase)

	if !status.Phase.IsRunning() {
		return w.config.Guard.Unlock()
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
		err = w.doQUIESCE(ctx, status)
	case migration.VALIDATION:
		err = w.doVALIDATION(ctx, status)
	case migration.SUCCESS:
		err = w.doSUCCESS(ctx, status)
	default:
		// The minion doesn't need to do anything for other
		// migration phases.
	}
	return errors.Trace(err)
}

func (w *Worker) doQUIESCE(ctx context.Context, status watcher.MigrationStatus) error {
	// Report that the minion is ready and that all workers that
	// should be shut down have done so.
	return w.report(ctx, status, true)
}

func (w *Worker) doVALIDATION(ctx context.Context, status watcher.MigrationStatus) error {
	attempt := retry.StartWithCancel(
		retry.LimitCount(maxRetries, retry.Exponential{
			Initial: initialRetryDelay,
			Factor:  retryBackoffFactor,
			Jitter:  true,
		}),
		w.config.Clock,
		w.catacomb.Dying(),
	)
	var err error
	for attempt.Next() {
		err = w.validate(ctx, status)
		if err == nil {
			break
		}
		if attempt.More() {
			w.config.Logger.Warningf(ctx, "validation failed (retrying): %v", err)
		}
	}
	if errors.Is(err, apiservererrors.ErrTryAgain) || params.IsCodeTryAgain(err) {
		// We treat TryAgainError as a retriable error,
		// so ingore it and don't report to the migration master.
		w.config.Logger.Errorf(ctx, "validation failed due to rate limit reached: %v", err)
		return nil
	}
	if err != nil {
		// Don't return this error just log it and report to the
		// migrationmaster that things didn't work out.
		w.config.Logger.Errorf(ctx, "validation failed: %v", err)
	}
	return w.report(ctx, status, err == nil)
}

func (w *Worker) validate(ctx context.Context, status watcher.MigrationStatus) error {
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
	conn, err := w.config.APIOpen(ctx, apiInfo, api.DialOpts{})
	if err != nil {
		return errors.Annotate(err, "failed to open API to target controller")
	}
	defer func() { _ = conn.Close() }()

	// Ask the agent to confirm that things look ok.
	err = w.config.ValidateMigration(ctx, conn)

	return errors.Trace(err)
}

func (w *Worker) doSUCCESS(ctx context.Context, status watcher.MigrationStatus) (err error) {
	defer func() {
		if err != nil {
			cfg := w.config.Agent.CurrentConfig()
			w.config.Logger.Criticalf(ctx, "migration failed for %v: %s/agent.conf left unchanged and pointing to source controller: %v",
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
	if err := w.robustReport(ctx, status, true); err != nil {
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

func (w *Worker) report(ctx context.Context, status watcher.MigrationStatus, success bool) error {
	w.config.Logger.Infof(ctx, "reporting back for phase %s: %v", status.Phase, success)
	err := w.config.Facade.Report(ctx, status.MigrationId, status.Phase, success)
	return errors.Annotate(err, "failed to report phase progress")
}

func (w *Worker) robustReport(ctx context.Context, status watcher.MigrationStatus, success bool) error {
	err := w.report(ctx, status, success)
	if err != nil && !rpc.IsShutdownErr(err) {
		return fmt.Errorf("cannot report migration status %v success=%v: %w", status, success, err)
	} else if err == nil {
		return nil
	}
	w.config.Logger.Warningf(ctx, "report migration status failed: %v", err)

	apiInfo, ok := w.config.Agent.CurrentConfig().APIInfo()
	if !ok {
		return fmt.Errorf("cannot report migration status %v success=%v: no API connection details", status, success)
	}
	apiInfo.Addrs = status.SourceAPIAddrs
	apiInfo.CACert = status.SourceCACert

	err = jujuretry.Call(jujuretry.CallArgs{
		Func: func() error {
			w.config.Logger.Infof(ctx, "reporting back for phase %s: %v", status.Phase, success)

			conn, err := w.config.APIOpen(ctx, apiInfo, api.DialOpts{})
			if err != nil {
				return fmt.Errorf("cannot dial source controller: %w", err)
			}
			defer func() { _ = conn.Close() }()

			facade, err := w.config.NewFacade(conn)
			if err != nil {
				return err
			}

			return facade.Report(ctx, status.MigrationId, status.Phase, success)
		},
		IsFatalError: func(err error) bool {
			return false
		},
		NotifyFunc: func(lastError error, attempt int) {
			w.config.Logger.Warningf(ctx, "report migration status failed (attempt %d): %v", attempt, lastError)
		},
		Clock:       w.config.Clock,
		Delay:       initialRetryDelay,
		Attempts:    maxRetries,
		BackoffFunc: jujuretry.DoubleDelay,
		Stop:        ctx.Done(),
	})
	if err != nil {
		return fmt.Errorf("cannot report migration status %v success=%v: %w", status, success, err)
	}
	return nil
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
