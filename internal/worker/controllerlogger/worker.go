// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlogger

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// modelConfigLoggerAPI adapts a ModelConfigService to the logger.LoggerAPI
// interface required by the shared logger worker.
type modelConfigLoggerAPI struct {
	service ModelConfigService
}

// LoggingConfig returns the logging configuration string from the controller
// model config. The agentTag parameter is accepted for interface compatibility
// but is not used for filtering; the controller model config applies to the
// controller agent.
func (a *modelConfigLoggerAPI) LoggingConfig(ctx context.Context, _ names.Tag) (string, error) {
	cfg, err := a.service.ModelConfig(ctx)
	if err != nil {
		return "", err
	}
	return cfg.LoggingConfig(), nil
}

// WatchLoggingConfig returns a notify watcher that fires when the logging
// config in the controller model changes. It wraps the underlying strings
// watcher (which returns changed keys) into a notify watcher that fires only
// when the logging-config key is among the changed keys.
func (a *modelConfigLoggerAPI) WatchLoggingConfig(ctx context.Context, _ names.Tag) (watcher.NotifyWatcher, error) {
	sw, err := a.service.Watch(ctx)
	if err != nil {
		return nil, err
	}
	return newLoggingConfigWatcher(sw), nil
}

// loggingConfigWatcher wraps a StringsWatcher and converts it to a
// NotifyWatcher that fires only when the logging-config key changes.
type loggingConfigWatcher struct {
	sw      watcher.StringsWatcher
	changes chan struct{}
	done    chan struct{}
}

func newLoggingConfigWatcher(sw watcher.StringsWatcher) *loggingConfigWatcher {
	w := &loggingConfigWatcher{
		sw:      sw,
		changes: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	go w.loop()
	return w
}

func (w *loggingConfigWatcher) loop() {
	defer close(w.done)
	for keys := range w.sw.Changes() {
		for _, key := range keys {
			if key == config.LoggingConfigKey {
				// Non-blocking send.
				select {
				case w.changes <- struct{}{}:
				default:
				}
				break
			}
		}
	}
}

// Changes returns the notify channel.
func (w *loggingConfigWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

// Kill stops the underlying strings watcher.
func (w *loggingConfigWatcher) Kill() {
	w.sw.Kill()
}

// Wait waits for the underlying strings watcher to stop and the loop to exit.
func (w *loggingConfigWatcher) Wait() error {
	err := w.sw.Wait()
	<-w.done
	return err
}
