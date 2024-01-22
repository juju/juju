// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"
)

const (
	// defaultAmberGracePeriod is the time that the unit is allowed to
	// function without a working API connection before its meter
	// status is switched to AMBER.
	defaultAmberGracePeriod = time.Minute * 5

	// defaultRedGracePeriod is the time that a unit is allowed to function
	// without a working API connection before its meter status is
	// switched to RED.
	defaultRedGracePeriod = time.Minute * 15
)

// workerState defines all the possible states the isolatedStatusWorker can be in.
type WorkerState int

const (
	Uninitialized WorkerState = iota
	WaitingAmber              // Waiting for a signal to switch to AMBER status.
	WaitingRed                // Waiting for a signal to switch to RED status.
	Done                      // No more transitions to perform.
)

// IsolatedConfig stores all the dependencies required to create an isolated meter status worker.
type IsolatedConfig struct {
	Runner           HookRunner
	StateReadWriter  StateReadWriter
	Clock            Clock
	Logger           Logger
	AmberGracePeriod time.Duration
	RedGracePeriod   time.Duration
	TriggerFactory   TriggerCreator
}

// Validate validates the config structure and returns an error on failure.
func (c IsolatedConfig) Validate() error {
	if c.Runner == nil {
		return errors.NotValidf("missing Runner")
	}
	if c.StateReadWriter == nil {
		return errors.NotValidf("missing StateReadWriter")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.AmberGracePeriod <= 0 {
		return errors.NotValidf("amber grace period")
	}
	if c.RedGracePeriod <= 0 {
		return errors.NotValidf("red grace period")
	}
	if c.AmberGracePeriod >= c.RedGracePeriod {
		return errors.New("amber grace period must be shorter than the red grace period")
	}
	return nil
}

// isolatedStatusWorker is a worker that is instantiated by the
// meter status manifold when the API connection is unavailable.
// Its main function is to escalate the meter status of the unit
// to amber and later to red.
type isolatedStatusWorker struct {
	config IsolatedConfig

	tomb tomb.Tomb
}

// NewIsolatedStatusWorker creates a new status worker that runs without an API connection.
func NewIsolatedStatusWorker(cfg IsolatedConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &isolatedStatusWorker{
		config: cfg,
	}
	w.tomb.Go(w.loop)
	return w, nil
}

func (w *isolatedStatusWorker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	st, err := w.config.StateReadWriter.Read()
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}

		// No state found; create a blank instance.
		st = new(State)
	}

	// Disconnected time has not been recorded yet.
	if st.Disconnected == nil {
		st.Disconnected = &Disconnected{Disconnected: w.config.Clock.Now().Unix(), State: WaitingAmber}
	}

	amberSignal, redSignal := w.config.TriggerFactory(st.Disconnected.State, st.Code, st.Disconnected.When(), w.config.Clock, w.config.AmberGracePeriod, w.config.RedGracePeriod)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-redSignal:
			w.config.Logger.Debugf("triggering meter status transition to RED due to loss of connection")
			currentCode := "RED"
			currentInfo := "unit agent has been disconnected"

			w.applyStatus(ctx, currentCode, currentInfo)
			st.Code, st.Info = currentCode, currentInfo
			st.Disconnected.State = Done
		case <-amberSignal:
			w.config.Logger.Debugf("triggering meter status transition to AMBER due to loss of connection")
			currentCode := "AMBER"
			currentInfo := "unit agent has been disconnected"

			w.applyStatus(ctx, currentCode, currentInfo)
			st.Code, st.Info = currentCode, currentInfo
			st.Disconnected.State = WaitingRed
		}
		if err := w.config.StateReadWriter.Write(st); err != nil {
			return errors.Annotate(err, "failed to record meter status worker state")
		}
	}
}

func (w *isolatedStatusWorker) applyStatus(ctx context.Context, code, info string) {
	w.config.Logger.Tracef("applying meter status change: %q (%q)", code, info)
	w.config.Runner.RunHook(ctx, code, info)
}

// Kill is part of the worker.Worker interface.
func (w *isolatedStatusWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *isolatedStatusWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *isolatedStatusWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}
