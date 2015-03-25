// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/tailer"
	"launchpad.net/golxc"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/version"
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
	aptMirror string,
	enablePackageUpdates bool,
	enableOSUpgrades bool,
	networkConfig *container.NetworkConfig,
) ([]byte, error) {
	var config *corecloudinit.Config
	if networkConfig != nil {
		var err error
		config, err = container.NewCloudInitConfigWithNetworks(series, networkConfig)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		config := cloudinit.New()
	}
	config.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
	)
	// For LTS series which need support for the cloud-tools archive,
	// we need to enable apt-get update regardless of the environ
	// setting, otherwise provisioning will fail.
	if series == "precise" && !enablePackageUpdates {
		logger.Warningf("series %q requires cloud-tools archive: enabling updates", series)
		enablePackageUpdates = true
	}

	config.AddSSHAuthorizedKeys(authorizedKeys)
	// add centos magic here
	if enablePackageUpdates {
		cloudconfig.MaybeAddCloudArchiveCloudTools(config, series)
	}
	cloudconfig.AddAptCommands(series, aptProxy, aptMirror, config, enablePackageUpdates, enableOSUpgrades)

	initSystem, err := containerInitSystem(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cmds, err := shutdownInitCommands(initSystem)
	if err != nil {
		return nil, errors.Trace(err)
	}
	config.AddScripts(strings.Join(cmds, "\n"))

	renderer, err := cloudinit.NewRenderer(series)
	if err != nil {
		return nil, err
	}
	data, err := renderer.Render(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return data, nil
}

func containerInitSystem(series string) (string, error) {
	osName, err := version.GetOSFromSeries(series)
	if err != nil {
		return "", errors.Trace(err)
	}
	vers := version.Binary{
		OS:     osName,
		Series: series,
	}
	initSystem, ok := service.VersionInitSystem(vers)
	if !ok {
		return "", errors.NotFoundf("init system for series %q", series)
	}
	logger.Debugf("using init system %q for shutdown script", initSystem)
	return initSystem, nil
}

// defaultEtcNetworkInterfaces is the contents of
// /etc/network/interfaces file which is left on the template LXC
// container on shutdown. This is needed to allow cloned containers to
// start in case no network config is provided during cloud-init, e.g.
// when AUFS is used or with the local provider (see bug #1431888).
const defaultEtcNetworkInterfaces = `
# loopback interface
auto lo
iface lo inet loopback

# primary interface
auto eth0
iface eth0 inet dhcp
`

func shutdownInitCommands(initSystem string) ([]string, error) {
	// These files are removed just before the template shuts down.
	cleanupOnShutdown := []string{
		// We remove any dhclient lease files so there's no chance a
		// clone to reuse a lease from the template it was cloned
		// from.
		"/var/lib/dhcp/dhclient*",
		// Both of these sets of files below are recreated on boot and
		// if we leave them in the template's rootfs boot logs coming
		// from cloned containers will be appended. It's better to
		// keep clean logs for diagnosing issues / debugging.
		"/var/log/cloud-init*.log",
	}

	// Using EOC below as the template shutdown script is itself
	// passed through cat > ... < EOF.
	replaceNetConfCmd := fmt.Sprintf(
		"/bin/cat > /etc/network/interfaces << EOC%sEOC\n  ",
		defaultEtcNetworkInterfaces,
	)
	paths := strings.Join(cleanupOnShutdown, " ")
	removeCmd := fmt.Sprintf("/bin/rm -fr %s\n  ", paths)
	shutdownCmd := "/sbin/shutdown -h now"
	name := "juju-template-restart"
	desc := "juju shutdown job"
	conf := common.Conf{
		Desc:         desc,
		Transient:    true,
		AfterStopped: "cloud-final",
		ExecStart:    replaceNetConfCmd + removeCmd + shutdownCmd,
	}
	svc, err := service.NewService(name, conf, initSystem)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cmds, err := svc.InstallCommands()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cmds, nil
}

func AcquireTemplateLock(name, message string) (*container.Lock, error) {
	logger.Infof("wait for flock on %v", name)
	lock, err := container.NewLock(TemplateLockDir, name)
	if err != nil {
		logger.Tracef("failed to create flock for template: %v", err)
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
	networkConfig *container.NetworkConfig,
	authorizedKeys string,
	aptProxy proxy.Settings,
	aptMirror string,
	enablePackageUpdates bool,
	enableOSUpgrades bool,
	imageURLGetter container.ImageURLGetter,
	useAUFS bool,
) (golxc.Container, error) {
	name := fmt.Sprintf("juju-%s-lxc-template", series)
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

	userData, err := templateUserData(
		series,
		authorizedKeys,
		aptProxy,
		aptMirror,
		enablePackageUpdates,
		enableOSUpgrades,
		networkConfig,
	)
	if err != nil {
		logger.Tracef("failed to create template user data for template: %v", err)
		return nil, err
	}
	userDataFilename, err := container.WriteCloudInitFile(containerDirectory, userData)
	if err != nil {
		return nil, err
	}

	templateParams := []string{
		"--debug",                      // Debug errors in the cloud image
		"--userdata", userDataFilename, // Our groovey cloud-init
		"--hostid", name, // Use the container name as the hostid
		"-r", series,
	}
	var caCert []byte
	if imageURLGetter != nil {
		arch := arch.HostArch()
		imageURL, err := imageURLGetter.ImageURL(instance.LXC, series, arch)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot determine cached image URL")
		}
		templateParams = append(templateParams, "-T", imageURL)
		caCert = imageURLGetter.CACert()
	}
	var extraCreateArgs []string
	if backingFilesystem == Btrfs {
		extraCreateArgs = append(extraCreateArgs, "-B", Btrfs)
	}

	// Create the container.
	logger.Tracef("create the template container")
	err = createContainer(
		lxcContainer,
		containerDirectory,
		networkConfig,
		extraCreateArgs,
		templateParams,
		caCert,
	)
	if err != nil {
		logger.Errorf("lxc template container creation failed: %v", err)
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
