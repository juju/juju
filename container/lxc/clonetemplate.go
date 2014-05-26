// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"launchpad.net/golxc"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/fslock"
	"launchpad.net/juju-core/utils/proxy"
	"launchpad.net/juju-core/utils/tailer"
)

const (
	templateShutdownUpstartFilename = "/etc/init/juju-template-restart.conf"
	templateShutdownUpstartScript   = `
description "Juju lxc template shutdown job"
author "Juju Team <juju@lists.ubuntu.com>"
start on stopped cloud-final

script
  shutdown -h now
end script

post-stop script
  rm ` + templateShutdownUpstartFilename + `
end script
`
)

var (
	TemplateLockDir = "/var/lib/juju/locks"

	TemplateStopTimeout = 5 * time.Minute
)

// templateUserData returns a minimal user data necessary for the template.
// This should have the authorized keys, base packages, the cloud archive if
// necessary,  initial apt proxy config, and it should do the apt-get
// update/upgrade initially.
func templateUserData(
	series string,
	authorizedKeys string,
	aptProxy proxy.Settings,
) ([]byte, error) {
	config := coreCloudinit.New()
	config.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
	)
	config.AddSSHAuthorizedKeys(authorizedKeys)
	cloudinit.MaybeAddCloudArchiveCloudTools(config, series)
	cloudinit.AddAptCommands(aptProxy, config)
	config.AddScripts(
		fmt.Sprintf(
			"printf '%%s\n' %s > %s",
			utils.ShQuote(templateShutdownUpstartScript),
			templateShutdownUpstartFilename,
		))
	data, err := config.Render()
	if err != nil {
		return nil, err
	}
	return data, nil
}

func AcquireTemplateLock(name, message string) (*fslock.Lock, error) {
	logger.Infof("wait for fslock on %v", name)
	lock, err := fslock.NewLock(TemplateLockDir, name)
	if err != nil {
		logger.Tracef("failed to create fslock for template: %v", err)
		return nil, err
	}
	err = lock.Lock(message)
	if err != nil {
		logger.Tracef("failed to acquire lock for template: %v", err)
		return nil, err
	}
	return lock, nil
}

// Make sure a template exists that we can clone from.
func EnsureCloneTemplate(
	backingFilesystem string,
	series string,
	network *container.NetworkConfig,
	authorizedKeys string,
	aptProxy proxy.Settings,
) (golxc.Container, error) {
	name := fmt.Sprintf("juju-%s-template", series)
	containerDirectory, err := container.NewDirectory(name)
	if err != nil {
		return nil, err
	}

	lock, err := AcquireTemplateLock(name, "ensure clone exists")
	if err != nil {
		return nil, err
	}
	defer lock.Unlock()

	lxcContainer := LxcObjectFactory.New(name)
	// Early exit if the container has been constructed before.
	if lxcContainer.IsConstructed() {
		logger.Infof("template exists, continuing")
		return lxcContainer, nil
	}
	logger.Infof("template does not exist, creating")

	userData, err := templateUserData(series, authorizedKeys, aptProxy)
	if err != nil {
		logger.Tracef("failed to create template user data for template: %v", err)
		return nil, err
	}
	userDataFilename, err := container.WriteCloudInitFile(containerDirectory, userData)
	if err != nil {
		return nil, err
	}

	configFile, err := writeLxcConfig(network, containerDirectory)
	if err != nil {
		logger.Errorf("failed to write config file: %v", err)
		return nil, err
	}
	templateParams := []string{
		"--debug",                      // Debug errors in the cloud image
		"--userdata", userDataFilename, // Our groovey cloud-init
		"--hostid", name, // Use the container name as the hostid
		"-r", series,
	}
	var extraCreateArgs []string
	if backingFilesystem == Btrfs {
		extraCreateArgs = append(extraCreateArgs, "-B", Btrfs)
	}
	// Create the container.
	logger.Tracef("create the container")
	if err := lxcContainer.Create(configFile, defaultTemplate, extraCreateArgs, templateParams); err != nil {
		logger.Errorf("lxc container creation failed: %v", err)
		return nil, err
	}
	// Make sure that the mount dir has been created.
	logger.Tracef("make the mount dir for the shared logs")
	if err := os.MkdirAll(internalLogDir(name), 0755); err != nil {
		logger.Tracef("failed to create internal /var/log/juju mount dir: %v", err)
		return nil, err
	}

	// Start the lxc container with the appropriate settings for grabbing the
	// console output and a log file.
	consoleFile := filepath.Join(containerDirectory, "console.log")
	lxcContainer.SetLogFile(filepath.Join(containerDirectory, "container.log"), golxc.LogDebug)
	logger.Tracef("start the container")
	// We explicitly don't pass through the config file to the container.Start
	// method as we have passed it through at container creation time.  This
	// is necessary to get the appropriate rootfs reference without explicitly
	// setting it ourselves.
	if err = lxcContainer.Start("", consoleFile); err != nil {
		logger.Errorf("container failed to start: %v", err)
		return nil, err
	}
	logger.Infof("template container started, now wait for it to stop")
	// Perhaps we should wait for it to finish, and the question becomes "how
	// long do we wait for it to complete?"

	console, err := os.Open(consoleFile)
	if err != nil {
		// can't listen
		return nil, err
	}

	tailWriter := &logTail{tick: time.Now()}
	consoleTailer := tailer.NewTailer(console, tailWriter, nil)
	defer consoleTailer.Stop()

	// We should wait maybe 1 minute between output?
	// if no output check to see if stopped
	// If we have no output and still running, something has probably gone wrong
	for lxcContainer.IsRunning() {
		if tailWriter.lastTick().Before(time.Now().Add(-TemplateStopTimeout)) {
			logger.Infof("not heard anything from the template log for five minutes")
			return nil, fmt.Errorf("template container %q did not stop", name)
		}
		time.Sleep(time.Second)
	}

	return lxcContainer, nil
}

type logTail struct {
	tick  time.Time
	mutex sync.Mutex
}

var _ io.Writer = (*logTail)(nil)

func (t *logTail) Write(data []byte) (int, error) {
	logger.Tracef(string(data))
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.tick = time.Now()
	return len(data), nil
}

func (t *logTail) lastTick() time.Time {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	tick := t.tick
	return tick
}
