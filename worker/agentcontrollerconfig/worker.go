package agentcontrollerconfig

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
	stateReload  = "reload"
)

// WorkerConfig encapsulates the configuration options for the
// agent controller config worker.
type WorkerConfig struct {
	Logger Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

type configWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	tomb           tomb.Tomb
}

// NewWorker creates a new tracer worker.
func NewWorker(cfg WorkerConfig) (*configWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*configWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &configWorker{
		internalStates: internalStates,
		cfg:            cfg,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *configWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *configWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *configWorker) loop() error {
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal is sent.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)

	// Report the initial started state.
	w.reportInternalState(stateStarted)

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-ch:
			w.reportInternalState(stateReload)

			w.cfg.Logger.Infof("SIGHUP received, reloading config")
		}
	}
}

func (w *configWorker) reportInternalState(state string) {
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
