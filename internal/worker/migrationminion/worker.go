// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

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

	// maxRedirects is the maximum number of redirects we'll follow when
	// dialing the target controller. This is to prevent infinite loops in case
	// of misconfiguration or a redirect loop.
	// If we reach this limit, we'll return an error.
	maxRedirects = 5
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
		Name: "migration-minion",
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type newDetails struct {
	targetAPIAddrs []string
	caCert         string
}

// Worker waits for a model migration to be active, then locks down the
// configured fortress and implements the migration.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	processed            map[string]migration.Phase
	newControllerDetails *newDetails
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
		// If the phase is not running, we can unlock the fortress, but remove
		// the migration from the processed map first.
		delete(w.processed, status.MigrationId)

		return w.config.Guard.Unlock(ctx)
	}

	// We've already processed this phase, so we can ignore it.
	// It's important to do this before we lockdown the fortress, as we want
	// to pretend that we've never seen this message.
	if p, ok := w.processed[status.MigrationId]; ok && p == status.Phase {
		return nil
	}

	// Ensure that all workers related to migration fortress have
	// stopped and aren't allowed to restart.
	err := w.config.Guard.Lockdown(ctx)
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

func (w *Worker) doQUIESCE(ctx context.Context, status watcher.MigrationStatus) error {
	// Report that the minion is ready and that all workers that
	// should be shut down have done so.
	return w.report(ctx, status, true)
}

func (w *Worker) doVALIDATION(ctx context.Context, status watcher.MigrationStatus) error {
	// Attempt the validation multiple times, with exponential backoff.
	// If this fails, that's it, we can't proceed. There isn't a guarantee
	// that we'll get another change event to retry.
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			return w.validate(ctx, status)
		},
		NotifyFunc: func(lastError error, attempt int) {
			w.config.Logger.Warningf(ctx, "validation failed (attempt %d): %v", attempt, lastError)
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
		w.config.Logger.Warningf(ctx, `validation failed: try changing "agent-ratelimit-max" and "agent-ratelimit-rate", before trying again: %v`, err)
		return ErrRetryable
	} else if err != nil {
		// Don't return this error just log it and report to the
		// migrationmaster that things didn't work out.
		w.config.Logger.Errorf(ctx, "validation failed: %v", err)
	}
	return w.report(ctx, status, err == nil)
}

func (w *Worker) validate(ctx context.Context, status watcher.MigrationStatus) error {
	conn, newDetails, err := w.dialNewController(ctx, status.TargetAPIAddrs, status.TargetCACert)
	if err != nil {
		return errors.Annotate(err, "failed to open API to target controller")
	}
	defer func() { _ = conn.Close() }()

	// Ask the agent to confirm that things look ok.
	err = w.config.ValidateMigration(ctx, conn)
	if err != nil {
		return errors.Trace(err)
	}

	// If the validation was successful, we can store the new controller details
	// for use in the SUCCESS phase.
	w.newControllerDetails = &newDetails
	return nil
}

// dialNewController dials the target controller and returns a connection
// and the newDetails struct containing the target API addresses and CA cert.
// It uses the current agent configuration to get the API connection details.
// If the connection is redirected, it will follow the redirect and return
// the final addresses and CA cert in the newDetails struct.
func (w *Worker) dialNewController(ctx context.Context, addrs []string, caCert string) (api.Connection, newDetails, error) {
	agentConf := w.config.Agent.CurrentConfig()
	apiInfo, ok := agentConf.APIInfo()
	if !ok {
		return nil, newDetails{}, errors.New("no API connection details")
	}
	apiInfo.Addrs = addrs
	apiInfo.CACert = caCert
	// Application agents (k8s) use old password.
	if apiInfo.Password == "" {
		apiInfo.Password = agentConf.OldPassword()
	}

	// Use zero DialOpts (no retries) because the worker must stay
	// responsive to Kill requests. We don't want it to be blocked by
	// a long set of retry attempts.
	conn, err := w.dialWithRedirect(ctx, apiInfo, api.DialOpts{}, 0)
	if err != nil {
		return nil, newDetails{}, errors.Annotate(err, "failed to open API to target controller")
	}
	return conn, newDetails{
		targetAPIAddrs: apiInfo.Addrs,
		caCert:         apiInfo.CACert,
	}, nil
}

func (w *Worker) dialWithRedirect(ctx context.Context, apiInfo *api.Info, dialOpts api.DialOpts, redirectCount int) (api.Connection, error) {
	select {
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	default:
	}

	if redirectCount >= maxRedirects {
		return nil, errors.Errorf("too many redirects (%d) when connecting to target controller", redirectCount)
	}
	conn, err := w.config.APIOpen(ctx, apiInfo, api.DialOpts{})
	if err != nil {
		if redirectErr, ok := errors.Cause(err).(*api.RedirectError); ok {
			w.config.Logger.Infof(ctx, "following redirect to %v", redirectErr.Servers)
			apiInfo.Addrs = network.CollapseToHostPorts(redirectErr.Servers).Strings()
			apiInfo.CACert = redirectErr.CACert
			return w.dialWithRedirect(ctx, apiInfo, dialOpts, redirectCount+1)
		}
		return nil, errors.Annotatef(err, "failed to open API to target controller")
	}
	return conn, nil
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

	// If the new details struct is nil, it means that the agent restarted between the
	// VALIDATION and SUCCESS phases, and we need to re-dial the new controller to
	// ensure that we follow any redirects that may have occurred.
	if w.newControllerDetails == nil {
		conn, newDetails, err := w.dialNewController(ctx, status.TargetAPIAddrs, status.TargetCACert)
		if err != nil {
			return errors.Annotate(err, "failed to open API to target controller")
		}
		conn.Close()
		w.newControllerDetails = &newDetails
	}

	hps, err := network.ParseProviderHostPorts(w.newControllerDetails.targetAPIAddrs...)
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
		conf.SetCACert(w.newControllerDetails.caCert)
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

	err = retry.Call(retry.CallArgs{
		Func: func() error {
			w.config.Logger.Infof(ctx, "reporting back for phase %s: %v", status.Phase, success)

			conn, err := w.dialWithRedirect(ctx, apiInfo, api.DialOpts{}, 0)
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
		BackoffFunc: retry.DoubleDelay,
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
