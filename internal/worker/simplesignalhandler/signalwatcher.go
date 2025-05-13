// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplesignalhandler

import (
	"fmt"
	"os"

	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
)

// SignalHandlerFunc is func definition for returning an error based on a
// received signal.
type SignalHandlerFunc func(os.Signal) error

// SignalWatcher is the worker responsible for watching signals and returning
// the appropriate error from a handler.
type SignalWatcher struct {
	catacomb catacomb.Catacomb
	handler  SignalHandlerFunc
	logger   logger.Logger
	sigCh    <-chan os.Signal
}

// Kill implements worker.Kill
func (s *SignalWatcher) Kill() {
	s.catacomb.Kill(nil)
}

// NewSignalWatcher constructs a new signal watcher worker with the specified
// signal channel and handler func.
func NewSignalWatcher(
	logger logger.Logger,
	sig <-chan os.Signal,
	handler SignalHandlerFunc,
) (*SignalWatcher, error) {
	s := &SignalWatcher{
		handler: handler,
		logger:  logger,
		sigCh:   sig,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "signal-watcher",
		Site: &s.catacomb,
		Work: s.watch,
	}); err != nil {
		return s, fmt.Errorf("creating catacomb plan: %w", err)
	}

	return s, nil
}

// SignalHandler is a default implementation that uses signal mapping and a
// default error.
func SignalHandler(defaultErr error, signalMap map[os.Signal]error) SignalHandlerFunc {
	return func(sig os.Signal) error {
		if signalMap == nil {
			return defaultErr
		}

		err, exists := signalMap[sig]
		if exists {
			return err
		}
		return defaultErr
	}
}

// Wait implements worker.Wait
func (s *SignalWatcher) Wait() error {
	return s.catacomb.Wait()
}

// watch watches for signals on the provided channel and returns error returned
// by handler when a signal is received.
func (s *SignalWatcher) watch() error {
	select {
	case sig, ok := <-s.sigCh:
		if !ok {
			return errors.New("signal channel closed unexpectedly")
		}
		return s.handler(sig)
	case <-s.catacomb.Dying():
		return s.catacomb.ErrDying()
	}
}
