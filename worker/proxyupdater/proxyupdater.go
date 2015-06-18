// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"fmt"
	"io/ioutil"
	"path"

	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/packaging/commands"
	"github.com/juju/utils/packaging/config"
	proxyutils "github.com/juju/utils/proxy"

	"github.com/juju/juju/api/environment"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
)

var (
	logger = loggo.GetLogger("juju.worker.proxyupdater")

	// ProxyDirectory is the directory containing the proxy file that contains
	// the environment settings for the proxies based on the environment
	// config values.
	ProxyDirectory = "/home/ubuntu"

	// proxySettingsRegistryPath is the Windows registry path where the proxy settings are saved
	proxySettingsRegistryPath = `HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	// ProxyFile is the name of the file to be stored in the ProxyDirectory.
	ProxyFile = ".juju-proxy"

	// Started is a function that is called when the worker has started.
	Started = func() {}
)

// proxyWorker is responsible for monitoring the juju environment
// configuration and making changes on the physical (or virtual) machine as
// necessary to match the environment changes.  Examples of these types of
// changes are apt proxy configuration and the juju proxies stored in the juju
// proxy file.
type proxyWorker struct {
	api      *environment.Facade
	aptProxy proxyutils.Settings
	proxy    proxyutils.Settings

	writeSystemFiles bool
	// The whole point of the first value is to make sure that the the files
	// are written out the first time through, even if they are the same as
	// "last" time, as the initial value for last time is the zeroed struct.
	// There is the possibility that the files exist on disk with old
	// settings, and the environment has been updated to now not have them. We
	// need to make sure that the disk reflects the environment, so the first
	// time through, even if the proxies are empty, we write the files to
	// disk.
	first bool
}

var _ worker.NotifyWatchHandler = (*proxyWorker)(nil)

// New returns a worker.Worker that updates proxy environment variables for the
// process; and, if writeSystemFiles is true, for the whole machine.
var New = func(api *environment.Facade, writeSystemFiles bool) worker.Worker {
	logger.Debugf("write system files: %v", writeSystemFiles)
	envWorker := &proxyWorker{
		api:              api,
		writeSystemFiles: writeSystemFiles,
		first:            true,
	}
	return worker.NewNotifyWorker(envWorker)
}

func (w *proxyWorker) writeEnvironmentFile() error {
	// Writing the environment file is handled by executing the script for two
	// primary reasons:
	//
	// 1: In order to have the local provider specify the environment settings
	// for the machine agent running on the host, this worker needs to run,
	// but it shouldn't be touching any files on the disk.  If however there is
	// an ubuntu user, it will. This shouldn't be a problem.
	//
	// 2: On cloud-instance ubuntu images, the ubuntu user is uid 1000, but in
	// the situation where the ubuntu user has been created as a part of the
	// manual provisioning process, the user will exist, and will not have the
	// same uid/gid as the default cloud image.
	//
	// It is easier to shell out to check both these things, and is also the
	// same way that the file is written in the cloud-init process, so
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

func (w *proxyWorker) writeEnvironmentToRegistry() error {
	// On windows we write the proxy settings to the registry.
	setProxyScript := `$value_path = "%s"
    $new_proxy = "%s"
    $proxy_val = Get-ItemProperty -Path $value_path -Name ProxySettings
    if ($? -eq $false){ New-ItemProperty -Path $value_path -Name ProxySettings -PropertyType String -Value $new_proxy }else{ Set-ItemProperty -Path $value_path -Name ProxySettings -Value $new_proxy }
    `
	result, err := exec.RunCommands(exec.RunParams{
		Commands: fmt.Sprintf(
			setProxyScript,
			proxySettingsRegistryPath,
			w.proxy.Http),
	})
	if err != nil {
		return err
	}
	if result.Code != 0 {
		logger.Errorf("failed writing new proxy values: \n%s\n%s", result.Stdout, result.Stderr)
	}
	return nil
}

func (w *proxyWorker) writeEnvironment() error {
	osystem, err := version.GetOSFromSeries(version.Current.Series)
	if err != nil {
		return err
	}
	switch osystem {
	case version.Windows:
		return w.writeEnvironmentToRegistry()
	default:
		return w.writeEnvironmentFile()
	}
}

func (w *proxyWorker) handleProxyValues(proxySettings proxyutils.Settings) {
	proxySettings.SetEnvironmentValues()
	if proxySettings != w.proxy || w.first {
		logger.Debugf("new proxy settings %#v", proxySettings)
		w.proxy = proxySettings
		if w.writeSystemFiles {
			if err := w.writeEnvironment(); err != nil {
				// It isn't really fatal, but we should record it.
				logger.Errorf("error writing proxy environment file: %v", err)
			}
		}
	}
}

// getPackageCommander is a helper function which returns the
// package commands implementation for the current system.
func getPackageCommander() (commands.PackageCommander, error) {
	return commands.NewPackageCommander(version.Current.Series)
}

func (w *proxyWorker) handleAptProxyValues(aptSettings proxyutils.Settings) error {
	if w.writeSystemFiles && (aptSettings != w.aptProxy || w.first) {
		logger.Debugf("new apt proxy settings %#v", aptSettings)
		paccmder, err := getPackageCommander()
		if err != nil {
			return err
		}
		w.aptProxy = aptSettings

		// Always finish with a new line.
		content := paccmder.ProxyConfigContents(w.aptProxy) + "\n"
		err = ioutil.WriteFile(config.AptProxyConfigFile, []byte(content), 0644)
		if err != nil {
			// It isn't really fatal, but we should record it.
			logger.Errorf("error writing apt proxy config file: %v", err)
		}
	}
	return nil
}

func (w *proxyWorker) onChange() error {
	env, err := w.api.EnvironConfig()
	if err != nil {
		return err
	}
	w.handleProxyValues(env.ProxySettings())
	err = w.handleAptProxyValues(env.AptProxySettings())
	if err != nil {
		return err
	}
	return nil
}

// SetUp is defined on the worker.NotifyWatchHandler interface.
func (w *proxyWorker) SetUp() (watcher.NotifyWatcher, error) {
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

// Handle is defined on the worker.NotifyWatchHandler interface.
func (w *proxyWorker) Handle(_ <-chan struct{}) error {
	return w.onChange()
}

// TearDown is defined on the worker.NotifyWatchHandler interface.
func (w *proxyWorker) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	return nil
}
