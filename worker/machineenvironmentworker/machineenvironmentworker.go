// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineenvironmentworker

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/state/api/environment"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.machineenvironment")

// MachineEnvironmentWorker is responsible for monitoring the juju environment
// configuration and making changes on the physical (or virtual) machine as
// necessary to match the environment changes.  Examples of these types of
// changes are apt proxy configuration and the juju proxies stored in the juju
// proxy file.
type MachineEnvironmentWorker struct {
	api      *environment.Facade
	aptProxy osenv.ProxySettings
	proxy    osenv.ProxySettings
}

var _ worker.NotifyWatchHandler = (*MachineEnvironmentWorker)(nil)

// NewMachineEnvironmentWorker returns a worker.Worker that uses the notify
// watcher returned from the setup.
func NewMachineEnvironmentWorker(api *environment.Facade) worker.Worker {
	envWorker := &MachineEnvironmentWorker{
		api: api,
	}
	return worker.NewNotifyWorker(envWorker)
}

func (w *MachineEnvironmentWorker) onChange() error {
	env, err := w.api.EnvironConfig()
	if err != nil {
		return err
	}
	proxySettings := env.ProxySettings()
	if proxySettings != w.proxy {
		logger.Debugf("new proxy settings %#v", proxySettings)
		w.proxy = proxySettings
		w.proxy.SetEnvironmentValues()
	}
	// TODO...
	_ = env
	return nil
}

func (w *MachineEnvironmentWorker) SetUp() (watcher.NotifyWatcher, error) {
	// We need to set this up initially as the NotifyWorker sucks up the first
	// event.
	err := w.onChange()
	if err != nil {
		return nil, err
	}
	return w.api.WatchForEnvironConfigChanges()
}

func (w *MachineEnvironmentWorker) Handle() error {
	return w.onChange()
}

func (w *MachineEnvironmentWorker) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	return nil
}
