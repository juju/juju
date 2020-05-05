// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"fmt"
	"io"
	"io/ioutil"
	stdexec "os/exec"

	"github.com/juju/errors"
	"github.com/juju/os"
	"github.com/juju/os/series"
	"github.com/juju/packaging/commands"
	"github.com/juju/packaging/config"
	"github.com/juju/proxy"
	"github.com/juju/utils/exec"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/proxyupdater"
	"github.com/juju/juju/core/snap"
	"github.com/juju/juju/core/watcher"
)

type Config struct {
	SupportLegacyValues bool
	RegistryPath        string
	EnvFiles            []string
	SystemdFiles        []string
	API                 API
	ExternalUpdate      func(proxy.Settings) error
	InProcessUpdate     func(proxy.Settings) error
	RunFunc             func(string, string, ...string) (string, error)
	Logger              Logger
}

// Validate ensures that all the required fields have values.
func (c *Config) Validate() error {
	if c.API == nil {
		return errors.NotValidf("missing API")
	}
	if c.InProcessUpdate == nil {
		return errors.NotValidf("missing InProcessUpdate")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// API is an interface that is provided to New
// which can be used to fetch the API host ports
type API interface {
	ProxyConfig() (proxyupdater.ProxyConfiguration, error)
	WatchForProxyConfigAndAPIHostPortChanges() (watcher.NotifyWatcher, error)
}

// proxyWorker is responsible for monitoring the juju environment
// configuration and making changes on the physical (or virtual) machine as
// necessary to match the environment changes.  Examples of these types of
// changes are apt proxy configuration and the juju proxies stored in the juju
// proxy file.
type proxyWorker struct {
	aptProxy proxy.Settings
	proxy    proxy.Settings

	snapProxy           proxy.Settings
	snapStoreProxy      string
	snapStoreAssertions string
	snapStoreProxyURL   string

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
	if err := config.Validate(); err != nil {
		return nil, err
	}
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
			w.config.Logger.Errorf("Error updating environment file %s - %v", file, err)
		}
	}
	for _, file := range w.config.SystemdFiles {
		err := ioutil.WriteFile(file, []byte(w.proxy.AsSystemdDefaultEnv()), 0644)
		if err != nil {
			w.config.Logger.Errorf("Error updating systemd file - %v", err)
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
		w.config.Logger.Errorf("saveProxySettingsToRegistry couldn't write proxy settings to registry: %s", err)
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
		w.config.Logger.Errorf("failed writing new proxy values: \n%s\n%s", result.Stdout, result.Stderr)
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

func (w *proxyWorker) handleProxyValues(legacyProxySettings, jujuProxySettings proxy.Settings) {
	// Legacy proxy settings update the environment, and also call the
	// InProcessUpdate, which installs the proxy into the default HTTP
	// transport. The same occurs for jujuProxySettings.
	settings := jujuProxySettings
	if jujuProxySettings.HasProxySet() {
		w.config.Logger.Debugf("applying in-process juju proxy settings %#v", jujuProxySettings)
	} else {
		settings = legacyProxySettings
		w.config.Logger.Debugf("applying in-process legacy proxy settings %#v", legacyProxySettings)
	}

	settings.SetEnvironmentValues()
	if err := w.config.InProcessUpdate(settings); err != nil {
		w.config.Logger.Errorf("error updating in-process proxy settings: %v", err)
	}

	// If the external update function is passed in, it is to update the LXD
	// proxies. We want to set this to the proxy specified regardless of whether
	// it was set with the legacy fields or the new juju fields.
	if externalFunc := w.config.ExternalUpdate; externalFunc != nil {
		if err := externalFunc(settings); err != nil {
			// It isn't really fatal, but we should record it.
			w.config.Logger.Errorf("%v", err)
		}
	}

	// Here we write files to disk. This is done only for legacyProxySettings.
	if w.config.SupportLegacyValues && (legacyProxySettings != w.proxy || w.first) {
		w.config.Logger.Debugf("saving new legacy proxy settings %#v", legacyProxySettings)
		w.proxy = legacyProxySettings
		if err := w.saveProxySettings(); err != nil {
			// It isn't really fatal, but we should record it.
			w.config.Logger.Errorf("error saving proxy settings: %v", err)
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

func (w *proxyWorker) handleSnapProxyValues(proxy proxy.Settings, storeID, storeAssertions, storeProxyURL string) {
	if os.HostOS() == os.Windows {
		w.config.Logger.Tracef("no snap proxies on windows")
		return
	}
	if w.config.RunFunc == nil {
		w.config.Logger.Tracef("snap proxies not updated by unit agents")
		return
	}

	var snapSettings []string
	maybeAddSettings := func(setting, value, saved string) {
		if value != saved || w.first {
			snapSettings = append(snapSettings, setting+"="+value)
		}
	}
	maybeAddSettings("proxy.http", proxy.Http, w.snapProxy.Http)
	maybeAddSettings("proxy.https", proxy.Https, w.snapProxy.Https)

	// Proxy URL changed; either a new proxy has been provided or the proxy
	// has been removed. Proxy URL changes have a higher precedence than
	// manually specifying the assertions and store ID.
	if storeProxyURL != w.snapStoreProxyURL {
		if storeProxyURL != "" {
			var err error
			if storeAssertions, storeID, err = snap.LookupAssertions(storeProxyURL); err != nil {
				w.config.Logger.Errorf("unable to lookup snap store assertions: %v", err)
				return
			} else {
				w.config.Logger.Infof("auto-detected snap store assertions from proxy")
				w.config.Logger.Infof("auto-detected snap store ID as %q", storeID)
			}
		} else if storeAssertions != "" && storeID != "" {
			// The proxy URL has been removed. However, if the user
			// has manually provided assertion/store ID config
			// options we should restore them. To do this, we reset
			// the last seen values so we can force-apply the
			// previously specified manual values. Otherwise, the
			// provided storeAssertions/storeID values are empty
			// and we simply fall through to allow the code to
			// reset the proxy.store setting to an empty value.
			w.snapStoreAssertions, w.snapStoreProxy = "", ""
		}
		w.snapStoreProxyURL = storeProxyURL
	} else if storeProxyURL != "" {
		// Re-use the storeID and assertions obtained by querying the
		// proxy during the last update.
		storeAssertions, storeID = w.snapStoreAssertions, w.snapStoreProxy
	}

	maybeAddSettings("proxy.store", storeID, w.snapStoreProxy)

	// If an assertion file was provided we need to "snap ack" it before
	// configuring snap to use the store ID.
	if storeAssertions != w.snapStoreAssertions && storeAssertions != "" {
		output, err := w.config.RunFunc(storeAssertions, "snap", "ack", "/dev/stdin")
		if err != nil {
			w.config.Logger.Warningf("unable to acknowledge assertions: %v, output: %q", err, output)
			return
		}
		w.snapStoreAssertions = storeAssertions
	}

	if len(snapSettings) > 0 {
		args := append([]string{"set", "core"}, snapSettings...)
		output, err := w.config.RunFunc(noStdIn, "snap", args...)
		if err != nil {
			w.config.Logger.Warningf("unable to set snap core settings %v: %v, output: %q", snapSettings, err, output)
		} else {
			w.config.Logger.Debugf("snap core settings %v updated, output: %q", snapSettings, output)
			w.snapProxy = proxy
			w.snapStoreProxy = storeID
		}
	}
}

func (w *proxyWorker) handleAptProxyValues(aptSettings proxy.Settings) error {
	if aptSettings != w.aptProxy || w.first {
		w.config.Logger.Debugf("new apt proxy settings %#v", aptSettings)
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
			w.config.Logger.Errorf("error writing apt proxy config file: %v", err)
		}
	}
	return nil
}

func (w *proxyWorker) onChange() error {
	config, err := w.config.API.ProxyConfig()
	if err != nil {
		return err
	}

	w.handleProxyValues(config.LegacyProxy, config.JujuProxy)
	w.handleSnapProxyValues(config.SnapProxy, config.SnapStoreProxyId, config.SnapStoreProxyAssertions, config.SnapStoreProxyURL)
	return w.handleAptProxyValues(config.APTProxy)
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

const noStdIn = ""

// Execute the command specified with the args with optional stdin.
func RunWithStdIn(input string, command string, args ...string) (string, error) {
	cmd := stdexec.Command(command, args...)

	if input != "" {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return "", errors.Annotate(err, "getting stdin pipe")
		}

		go func() {
			defer stdin.Close()
			io.WriteString(stdin, input)
		}()
	}

	out, err := cmd.CombinedOutput()
	output := string(out)
	return output, err
}
