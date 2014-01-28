// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineenvironmentworker

import (
	"fmt"
	"io/ioutil"
	"path"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/state/api/environment"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/exec"
	"launchpad.net/juju-core/worker"
)

var (
	logger = loggo.GetLogger("juju.worker.machineenvironment")

	// ProxyDirectory is the directory containing the proxy file that contains
	// the environment settings for the proxies based on the environment
	// config values.
	ProxyDirectory = "/home/ubuntu"

	// ProxyFile is the name of the file to be stored in the ProxyDirectory.
	ProxyFile = ".juju-proxy"

	// Started is a function that is called when the worker has started.
	Started = func() {}
)

// MachineEnvironmentWorker is responsible for monitoring the juju environment
// configuration and making changes on the physical (or virtual) machine as
// necessary to match the environment changes.  Examples of these types of
// changes are apt proxy configuration and the juju proxies stored in the juju
// proxy file.
type MachineEnvironmentWorker struct {
	api      *environment.Facade
	aptProxy osenv.ProxySettings
	proxy    osenv.ProxySettings
	first    bool
}

var _ worker.NotifyWatchHandler = (*MachineEnvironmentWorker)(nil)

// NewMachineEnvironmentWorker returns a worker.Worker that uses the notify
// watcher returned from the setup.
func NewMachineEnvironmentWorker(api *environment.Facade) worker.Worker {
	envWorker := &MachineEnvironmentWorker{
		api:   api,
		first: true,
	}
	return worker.NewNotifyWorker(envWorker)
}

func (w *MachineEnvironmentWorker) writeEnvironmentFile() error {
	// Writing the environment file is handled by executing the script for two
	// primary reasons:
	//
	// 1: In order to have the local provider specify the environment settings
	// for the machine agent running on the host, this worker needs to run,
	// but it should be touching any files on the disk.  If however there is
	// an ubuntu user, it will. This shouldn't be a problem.
	//
	// 2: On cloud-instance ubuntu images, the ubuntu user is uid 1000, but in
	// the situation where the ubuntu user has been created as a part of the
	// manual provisioning process, the user will exist, and will not have the
	// same uid/gid as the default cloud image.
	//
	// It is easier to shell out to check both these things, and is also the
	// same way that the file is writting in the cloud-init process, so
	// consistency FTW.
	filePath := path.Join(ProxyDirectory, ProxyFile)
	result, err := exec.RunCommands(exec.RunParams{
		Commands: fmt.Sprintf(
			`[ -e %s ] && (printf '%%s\n' %s > %s && chown ubuntu:ubuntu %s)`,
			ProxyDirectory,
			utils.ShQuote(w.proxy.AsScriptEnvironment()),
			filePath, filePath),
		WorkingDir: ProxyDirectory,
	})
	if err != nil {
		return err
	}
	if result.Code != 0 {
		logger.Errorf("failed writing new proxy values: \n%s\n%s", result.Stdout, result.Stderr)
	}
	return nil
}

func (w *MachineEnvironmentWorker) onChange() error {
	env, err := w.api.EnvironConfig()
	if err != nil {
		return err
	}
	proxySettings := env.ProxySettings()
	if proxySettings != w.proxy || w.first {
		logger.Debugf("new proxy settings %#v", proxySettings)
		w.proxy = proxySettings
		w.proxy.SetEnvironmentValues()
		if err := w.writeEnvironmentFile(); err != nil {
			// It isn't really fatal, but we should record it.
			logger.Errorf("error writing proxy environment file: %v", err)
		}
	}
	aptSettings := env.AptProxySettings()
	if aptSettings != w.aptProxy || w.first {
		logger.Debugf("new apt proxy settings %#v", aptSettings)
		w.aptProxy = aptSettings
		// Always finish with a new line.
		content := utils.AptProxyContent(w.aptProxy) + "\n"
		err := ioutil.WriteFile(utils.AptConfFile, []byte(content), 0644)
		if err != nil {
			// It isn't really fatal, but we should record it.
			logger.Errorf("error writing apt proxy config file: %v", err)
		}
	}
	return nil
}

func (w *MachineEnvironmentWorker) SetUp() (watcher.NotifyWatcher, error) {
	// We need to set this up initially as the NotifyWorker sucks up the first
	// event.
	err := w.onChange()
	if err != nil {
		return nil, err
	}
	w.first = false
	Started()
	return w.api.WatchForEnvironConfigChanges()
}

func (w *MachineEnvironmentWorker) Handle() error {
	return w.onChange()
}

func (w *MachineEnvironmentWorker) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	return nil
}
