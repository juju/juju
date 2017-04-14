// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"fmt"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/os"
	"github.com/juju/utils/packaging/commands"
	"github.com/juju/utils/packaging/config"
	proxyutils "github.com/juju/utils/proxy"
	"github.com/juju/utils/series"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/watcher"
)

var (
	logger = loggo.GetLogger("juju.worker.proxyupdater")
)

type Config struct {
	RegistryPath    string
	EnvFiles        []string
	SystemdFiles    []string
	API             API
	ExternalUpdate  func(proxyutils.Settings) error
	InProcessUpdate func(proxyutils.Settings) error
}

// API is an interface that is provided to New
// which can be used to fetch the API host ports
type API interface {
	ProxyConfig() (proxyutils.Settings, proxyutils.Settings, error)
	WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error)
}

// proxyWorker is responsible for monitoring the juju environment
// configuration and making changes on the physical (or virtual) machine as
// necessary to match the environment changes.  Examples of these types of
// changes are apt proxy configuration and the juju proxies stored in the juju
// proxy file.
type proxyWorker struct {
	aptProxy proxyutils.Settings
	proxy    proxyutils.Settings

	// The whole point of the first value is to make sure that the the files
	// are written out the first time through, even if they are the same as
	// "last" time, as the initial value for last time is the zeroed struct.
	// There is the possibility that the files exist on disk with old
	// settings, and the environment has been updated to now not have them. We
	// need to make sure that the disk reflects the environment, so the first
	// time through, even if the proxies are empty, we write the files to
	// disk.
	first  bool
	config Config
}

// NewWorker returns a worker.Worker that updates proxy environment variables for the
// process and for the whole machine.
var NewWorker = func(config Config) (worker.Worker, error) {
	envWorker := &proxyWorker{
		first:  true,
		config: config,
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: envWorker,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (w *proxyWorker) saveProxySettingsToFiles() error {
	// The proxy settings are (usually) stored in three files:
	// - /etc/juju-proxy.conf - in 'env' format
	// - /etc/systemd/system.conf.d/juju-proxy.conf
	// - /etc/systemd/user.conf.d/juju-proxy.conf - both in 'systemd' format
	for _, file := range w.config.EnvFiles {
		err := ioutil.WriteFile(file, []byte(w.proxy.AsScriptEnvironment()), 0644)
		if err != nil {
			logger.Errorf("Error updating environment file %s - %v", file, err)
		}
	}
	for _, file := range w.config.SystemdFiles {
		err := ioutil.WriteFile(file, []byte(w.proxy.AsSystemdDefaultEnv()), 0644)
		if err != nil {
			logger.Errorf("Error updating systemd file %s - %v", file, err)
		}
	}
	return nil
}

func (w *proxyWorker) saveProxySettingsToRegistry() error {
	// On windows we write the proxy settings to the registry.
	setProxyScript := `$value_path = "%s"
    $new_proxy = "%s"
    $proxy_val = Get-ItemProperty -Path $value_path -Name ProxySettings
    if ($? -eq $false){ New-ItemProperty -Path $value_path -Name ProxySettings -PropertyType String -Value $new_proxy }else{ Set-ItemProperty -Path $value_path -Name ProxySettings -Value $new_proxy }
    `

	if w.config.RegistryPath == "" {
		err := fmt.Errorf("config.RegistryPath is empty")
		logger.Errorf("saveProxySettingsToRegistry couldn't write proxy settings to registry: %s", err)
		return err
	}

	result, err := exec.RunCommands(exec.RunParams{
		Commands: fmt.Sprintf(
			setProxyScript,
			w.config.RegistryPath,
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

func (w *proxyWorker) saveProxySettings() error {
	switch os.HostOS() {
	case os.Windows:
		return w.saveProxySettingsToRegistry()
	default:
		return w.saveProxySettingsToFiles()
	}
}

func (w *proxyWorker) handleProxyValues(proxySettings proxyutils.Settings) {
	proxySettings.SetEnvironmentValues()
	if err := w.config.InProcessUpdate(proxySettings); err != nil {
		logger.Errorf("error updating in-process proxy settings: %v", err)
	}
	if proxySettings != w.proxy || w.first {
		logger.Debugf("new proxy settings %#v", proxySettings)
		w.proxy = proxySettings
		if err := w.saveProxySettings(); err != nil {
			// It isn't really fatal, but we should record it.
			logger.Errorf("error saving proxy settings: %v", err)
		}
		if externalFunc := w.config.ExternalUpdate; externalFunc != nil {
			if err := externalFunc(proxySettings); err != nil {
				// It isn't really fatal, but we should record it.
				logger.Errorf("%v", err)
			}
		}
	}
}

// getPackageCommander is a helper function which returns the
// package commands implementation for the current system.
func getPackageCommander() (commands.PackageCommander, error) {
	hostSeries, err := series.HostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return commands.NewPackageCommander(hostSeries)
}

func (w *proxyWorker) handleAptProxyValues(aptSettings proxyutils.Settings) error {
	if aptSettings != w.aptProxy || w.first {
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
	proxySettings, APTProxySettings, err := w.config.API.ProxyConfig()
	if err != nil {
		return err
	}

	w.handleProxyValues(proxySettings)
	return w.handleAptProxyValues(APTProxySettings)
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
	return w.config.API.WatchForProxyConfigAndAPIHostPortChanges()
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
