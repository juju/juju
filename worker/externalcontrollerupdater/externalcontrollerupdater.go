// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"io"
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/crosscontroller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.externalcontrollerupdater")

// ExternalControllerWatcherClient defines the interface for watching changes
// to the local controller's external controller records, and obtaining and
// updating their values. This will communicate only with the local controller.
type ExternalControllerUpdaterClient interface {
	WatchExternalControllers() (watcher.StringsWatcher, error)
	ExternalControllerInfo(controllerUUID string) (*crossmodel.ControllerInfo, error)
	SetExternalControllerInfo(crossmodel.ControllerInfo) error
}

// ExternalControllerWatcherClientCloser extends the ExternalControllerWatcherClient
// interface with a Close method, for closing the API connection associated with
// the client.
type ExternalControllerWatcherClientCloser interface {
	ExternalControllerWatcherClient
	io.Closer
}

// ExternalControllerWatcherClient defines the interface for watching changes
// to and obtaining the current API information for a controller. This will
// communicate with an external controller.
type ExternalControllerWatcherClient interface {
	WatchControllerInfo() (watcher.NotifyWatcher, error)
	ControllerInfo() (*crosscontroller.ControllerInfo, error)
}

// NewExternalControllerWatcherClientFunc is a function type that
// returns an ExternalControllerWatcherClientCloser, given an
// *api.Info. The api.Info should be for making a controller-only
// connection to a remote/external controller.
type NewExternalControllerWatcherClientFunc func(*api.Info) (ExternalControllerWatcherClientCloser, error)

// New returns a new external controller updater worker.
func New(
	externalControllers ExternalControllerUpdaterClient,
	newExternalControllerWatcherClient NewExternalControllerWatcherClientFunc,
	clock clock.Clock,
) (worker.Worker, error) {
	w := updaterWorker{
		watchExternalControllers:           externalControllers.WatchExternalControllers,
		externalControllerInfo:             externalControllers.ExternalControllerInfo,
		setExternalControllerInfo:          externalControllers.SetExternalControllerInfo,
		newExternalControllerWatcherClient: newExternalControllerWatcherClient,
		runner: worker.NewRunner(worker.RunnerParams{
			// One of the controller watchers fails should not
			// prevent the others from running.
			IsFatal: func(error) bool { return false },

			// If the API connection fails, try again in 1 minute.
			RestartDelay: time.Minute,
			Clock:        clock,
		}),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return &w, nil
}

type updaterWorker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	watchExternalControllers           func() (watcher.StringsWatcher, error)
	externalControllerInfo             func(controllerUUID string) (*crossmodel.ControllerInfo, error)
	setExternalControllerInfo          func(crossmodel.ControllerInfo) error
	newExternalControllerWatcherClient NewExternalControllerWatcherClientFunc
}

// Kill is part of the worker.Worker interface.
func (w *updaterWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *updaterWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *updaterWorker) loop() error {
	watcher, err := w.watchExternalControllers()
	if err != nil {
		return errors.Annotate(err, "watching external controllers")
	}
	w.catacomb.Add(watcher)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case ids, ok := <-watcher.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}

			if len(ids) == 0 {
				continue
			}

			logger.Debugf("external controllers changed: %q", ids)
			tags := make([]names.ControllerTag, len(ids))
			for i, id := range ids {
				if !names.IsValidController(id) {
					return errors.Errorf("%q is not a valid controller tag", id)
				}
				tags[i] = names.NewControllerTag(id)
			}

			watchers := names.NewSet()
			for _, tag := range tags {
				// We're informed when an external controller
				// is added or removed, so treat as a toggle.
				if watchers.Contains(tag) {
					logger.Infof("stopping watcher for external controller %q", tag.Id())
					w.runner.StopWorker(tag.Id())
					watchers.Remove(tag)
					continue
				}
				logger.Infof("starting watcher for external controller %q", tag.Id())
				watchers.Add(tag)
				if err := w.runner.StartWorker(tag.Id(), func() (worker.Worker, error) {
					cw := controllerWatcher{
						tag: tag,
						setExternalControllerInfo:          w.setExternalControllerInfo,
						externalControllerInfo:             w.externalControllerInfo,
						newExternalControllerWatcherClient: w.newExternalControllerWatcherClient,
					}
					if err := catacomb.Invoke(catacomb.Plan{
						Site: &cw.catacomb,
						Work: cw.loop,
					}); err != nil {
						return nil, errors.Trace(err)
					}
					return &cw, nil
				}); err != nil {
					return errors.Annotatef(err, "starting watcher for external controller %q", tag.Id())
				}
			}
		}
	}
}

// controllerWatcher is a worker that watches for changes to the external
// controller with the given tag. The external controller must be known
// to the local controller.
type controllerWatcher struct {
	catacomb catacomb.Catacomb

	tag                                names.ControllerTag
	setExternalControllerInfo          func(crossmodel.ControllerInfo) error
	externalControllerInfo             func(controllerUUID string) (*crossmodel.ControllerInfo, error)
	newExternalControllerWatcherClient NewExternalControllerWatcherClientFunc
}

// Kill is part of the worker.Worker interface.
func (w *controllerWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *controllerWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *controllerWatcher) loop() error {
	// We get the API info from the local controller initially.
	info, err := w.externalControllerInfo(w.tag.Id())
	if err != nil {
		return errors.Annotate(err, "getting cached external controller info")
	}
	logger.Debugf("controller info for controller %q: %v", w.tag.Id(), info)

	var nw watcher.NotifyWatcher
	var client ExternalControllerWatcherClientCloser
	defer func() {
		if client != nil {
			client.Close()
		}
	}()

	for {
		if client == nil {
			apiInfo := &api.Info{
				Addrs:  info.Addrs,
				CACert: info.CACert,
				Tag:    names.NewUserTag(api.AnonymousUsername),
			}
			client, err = w.newExternalControllerWatcherClient(apiInfo)
			if err != nil {
				return errors.Annotate(err, "getting external controller client")
			}
			nw, err = client.WatchControllerInfo()
			if err != nil {
				return errors.Annotate(err, "watching external controller")
			}
			w.catacomb.Add(nw)
		}

		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-nw.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}

			newInfo, err := client.ControllerInfo()
			if err != nil {
				return errors.Annotate(err, "getting external controller info")
			}
			if reflect.DeepEqual(newInfo.Addrs, info.Addrs) {
				continue
			}
			// API addresses have changed. Save the details to the
			// local controller and stop the existing notify watcher
			// and set it to nil, so we'll restart it with the new
			// addresses.
			info.Addrs = newInfo.Addrs
			if err := w.setExternalControllerInfo(crossmodel.ControllerInfo{
				ControllerTag: w.tag,
				Alias:         info.Alias,
				Addrs:         info.Addrs,
				CACert:        info.CACert,
			}); err != nil {
				return errors.Annotate(err, "caching external controller info")
			}
			logger.Infof("new controller info for controller %q: %v", w.tag.Id(), info)
			if err := worker.Stop(nw); err != nil {
				return errors.Trace(err)
			}
			if err := client.Close(); err != nil {
				return errors.Trace(err)
			}
			client = nil
			nw = nil
		}
	}
}
