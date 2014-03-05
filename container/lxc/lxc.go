// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/tailer"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loggo/loggo"
	"launchpad.net/golxc"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/utils/fslock"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.container.lxc")

var (
	defaultTemplate  = "ubuntu-cloud"
	LxcContainerDir  = "/var/lib/lxc"
	LxcRestartDir    = "/etc/lxc/auto"
	LxcObjectFactory = golxc.Factory()
)

const (
	// DefaultLxcBridge is the package created container bridge
	DefaultLxcBridge = "lxcbr0"
	// Btrfs is special as we treat it differently for create and clone.
	Btrfs = "btrfs"
)

// DefaultNetworkConfig returns a valid NetworkConfig to use the
// defaultLxcBridge that is created by the lxc package.
func DefaultNetworkConfig() *container.NetworkConfig {
	return container.BridgeNetworkConfig(DefaultLxcBridge)
}

// FsCommandOutput calls cmd.Output, this is used as an overloading point so
// we can test what *would* be run without actually executing another program
var FsCommandOutput = (*exec.Cmd).CombinedOutput

func containerDirFilesystem() (string, error) {
	cmd := exec.Command("df", "--output=fstype", LxcContainerDir)
	out, err := FsCommandOutput(cmd)
	if err != nil {
		return "", err
	}
	// The filesystem is the second line.
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		logger.Errorf("unexpected output: ", out)
		return "", fmt.Errorf("could not determine filesystem type")
	}
	return lines[1], nil
}

type containerManager struct {
	name              string
	logdir            string
	createWithClone   bool
	backingFilesystem string
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

// NewContainerManager returns a manager object that can start and stop lxc
// containers. The containers that are created are namespaced by the name
// parameter.
func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	name := conf[container.ConfigName]
	logDir := conf[container.LogDir]
	if logDir == "" {
		logDir = "/var/log/juju"
	}
	useClone := false
	if conf["use-clone"] == "true" {
		logger.Tracef("using lxc-clone for creating containers")
		useClone = true
	}
	backingFS, err := containerDirFilesystem()
	if err != nil {
		return nil, err
	}
	return &containerManager{
		name:              name,
		logdir:            logDir,
		createWithClone:   useClone,
		backingFilesystem: backingFS,
	}, nil
}

func (manager *containerManager) StartContainer(
	machineConfig *cloudinit.MachineConfig,
	series string,
	network *container.NetworkConfig,
) (instance.Instance, *instance.HardwareCharacteristics, error) {

	name := names.MachineTag(machineConfig.MachineId)
	if manager.name != "" {
		name = fmt.Sprintf("%s-%s", manager.name, name)
	}
	// Create the cloud-init.
	directory, err := container.NewDirectory(name)
	if err != nil {
		return nil, nil, err
	}
	logger.Tracef("write cloud-init")
	if manager.createWithClone {
		// If we are using clone, disable the apt-get steps
		machineConfig.DisablePackageCommands = true
	}
	userDataFilename, err := container.WriteUserData(machineConfig, directory)
	if err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return nil, nil, err
	}
	logger.Tracef("write the lxc.conf file")
	useAutostart := true
	configFile, err := writeLxcConfig(network, directory, manager.logdir, useAutostart)
	if err != nil {
		logger.Errorf("failed to write config file: %v", err)
		return nil, nil, err
	}

	var lxcContainer golxc.Container
	if manager.createWithClone {
		var templateContainer golxc.Container
		if templateContainer, err = manager.ensureCloneTemplate(
			machineConfig, series, network); err != nil {
			return nil, nil, err
		}
		templateParams := []string{
			"--debug",                      // Debug errors in the cloud image
			"--userdata", userDataFilename, // Our groovey cloud-init
			"--hostid", name, // Use the container name as the hostid
		}
		// TODO: fslock on clone template
		lxcContainer, err = templateContainer.Clone(name, nil, templateParams) // TODO:. ...
		if err != nil {
			logger.Errorf("lxc container cloning failed: %v", err)
			return nil, nil, err
		}
	} else {
		// Note here that the lxcObjectFacotry only returns a valid container
		// object, and doesn't actually construct the underlying lxc container on
		// disk.
		lxcContainer = LxcObjectFactory.New(name)
		templateParams := []string{
			"--debug",                      // Debug errors in the cloud image
			"--userdata", userDataFilename, // Our groovey cloud-init
			"--hostid", name, // Use the container name as the hostid
			"-r", series,
		}
		// Create the container.
		logger.Tracef("create the container")
		if err := lxcContainer.Create(configFile, defaultTemplate, nil, templateParams); err != nil {
			logger.Errorf("lxc container creation failed: %v", err)
			return nil, nil, err
		}
		// Make sure that the mount dir has been created.
		logger.Tracef("make the mount dir for the shared logs")
		if err := os.MkdirAll(internalLogDir(name), 0755); err != nil {
			logger.Errorf("failed to create internal /var/log/juju mount dir: %v", err)
			return nil, nil, err
		}
		logger.Tracef("lxc container created")
		// We know that this check is only needed here as if we are using clone,
		// we know we have a modern enough lxc that it isn't using the restart
		// dir.

		// Now symlink the config file into the restart directory, if it exists.
		// This is for backwards compatiblity. From Trusty onwards, the auto start
		// option should be set in the LXC config file, this is done in the lxcConfigTemplate
		// function below.
		if useRestartDir() {
			containerConfigFile := filepath.Join(LxcContainerDir, name, "config")
			if err := os.Symlink(containerConfigFile, restartSymlink(name)); err != nil {
				return nil, nil, err
			}
			logger.Tracef("auto-restart link created")
		}
	}

	// Start the lxc container with the appropriate settings for grabbing the
	// console output and a log file.
	consoleFile := filepath.Join(directory, "console.log")
	lxcContainer.SetLogFile(filepath.Join(directory, "container.log"), golxc.LogDebug)
	logger.Tracef("start the container")
	// We explicitly don't pass through the config file to the container.Start
	// method as we have passed it through at container creation time.  This
	// is necessary to get the appropriate rootfs reference without explicitly
	// setting it ourselves.
	if err = lxcContainer.Start("", consoleFile); err != nil {
		logger.Errorf("container failed to start: %v", err)
		return nil, nil, err
	}
	arch := version.Current.Arch
	hardware := &instance.HardwareCharacteristics{
		Arch: &arch,
	}
	logger.Tracef("container started")
	return &lxcInstance{lxcContainer, name}, hardware, nil
}

func (manager *containerManager) StopContainer(instance instance.Instance) error {
	name := string(instance.Id())
	lxcContainer := LxcObjectFactory.New(name)
	if useRestartDir() {
		// Remove the autostart link.
		if err := os.Remove(restartSymlink(name)); err != nil {
			logger.Errorf("failed to remove restart symlink: %v", err)
			return err
		}
	}
	if err := lxcContainer.Destroy(); err != nil {
		logger.Errorf("failed to destroy lxc container: %v", err)
		return err
	}

	return container.RemoveDirectory(name)
}

func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	containers, err := LxcObjectFactory.List()
	if err != nil {
		logger.Errorf("failed getting all instances: %v", err)
		return
	}
	managerPrefix := ""
	if manager.name != "" {
		managerPrefix = fmt.Sprintf("%s-", manager.name)
	}

	for _, container := range containers {
		// Filter out those not starting with our name.
		name := container.Name()
		if !strings.HasPrefix(name, managerPrefix) {
			continue
		}
		if container.IsRunning() {
			result = append(result, &lxcInstance{container, name})
		}
	}
	return
}

const internalLogDirTemplate = "%s/%s/rootfs/var/log/juju"

func internalLogDir(containerName string) string {
	return fmt.Sprintf(internalLogDirTemplate, LxcContainerDir, containerName)
}

func restartSymlink(name string) string {
	return filepath.Join(LxcRestartDir, name+".conf")
}

const localConfig = `%s
lxc.mount.entry=%s var/log/juju none defaults,bind 0 0
`

const networkTemplate = `
lxc.network.type = %s
lxc.network.link = %s
lxc.network.flags = up
`

func lxcConfigTemplate(networkType, networkLink string) string {
	return fmt.Sprintf(networkTemplate, networkType, networkLink)
}

func generateLXCConfigTemplate(
	network *container.NetworkConfig,
	useAutostart bool,
) string {
	var lxcConfig string
	if network == nil {
		logger.Warningf("network unspecified, using default networking config")
		network = DefaultNetworkConfig()
	}
	switch network.NetworkType {
	case container.PhysicalNetwork:
		lxcConfig = lxcConfigTemplate("phys", network.Device)
	default:
		logger.Warningf("Unknown network config type %q: using bridge", network.NetworkType)
		fallthrough
	case container.BridgeNetwork:
		lxcConfig = lxcConfigTemplate("veth", network.Device)
	}

	if useAutostart && !useRestartDir() {
		lxcConfig += "lxc.start.auto = 1\n"
		logger.Tracef("Setting auto start to true in lxc config.")
	}

	return lxcConfig
}

func writeLxcConfig(
	network *container.NetworkConfig,
	directory,
	logdir string,
	useAutostart bool,
) (string, error) {
	networkConfig := generateLXCConfigTemplate(network, useAutostart)
	configFilename := filepath.Join(directory, "lxc.conf")
	configContent := fmt.Sprintf(localConfig, networkConfig, logdir)
	if err := ioutil.WriteFile(configFilename, []byte(configContent), 0644); err != nil {
		return "", err
	}
	return configFilename, nil
}

// useRestartDir is used to determine whether or not to use a symlink to the
// container config as the restart mechanism.  Older versions of LXC had the
// /etc/lxc/auto directory that would indicate that a container shoud auto-
// restart when the machine boots by having a symlink to the lxc.conf file.
// Newer versions don't do this, but instead have a config value inside the
// lxc.conf file.
func useRestartDir() bool {
	if _, err := os.Stat(LxcRestartDir); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

const (
	templateShutdownUpstartFilename = "/etc/init/juju-template-restart.conf"
	templateShutdownUpstartScript   = `
description "Juju lxc template shutdown job"
author "Juju Team <juju@lists.ubuntu.com>"
start on stopped cloud-final

script
  echo "Shutting down"
  shutdown -h now
end script

post-stop script
  echo "Removing restart script"
  rm ` + templateShutdownUpstartFilename + `
end script
`
)

// templateUserData returns a minimal user data necessary for the template.
// This should have the authorized keys, base packages, the cloud archive if
// necessary,  initial apt proxy config, and it should do the apt-get
// update/upgrade initially.
func templateUserData(machineConfig *cloudinit.MachineConfig) ([]byte, error) {
	cloudConfig := coreCloudinit.New()
	cloudConfig.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
	)
	machineConfig.MaybeAddCloudArchiveCloudTools(cloudConfig)
	cloudConfig.AddSSHAuthorizedKeys(machineConfig.AuthorizedKeys)
	cloudinit.AddAptCommands(machineConfig, cloudConfig)
	cloudConfig.AddScripts(
		fmt.Sprintf(
			"printf '%%s\n' %s > %s",
			utils.ShQuote(templateShutdownUpstartScript),
			templateShutdownUpstartFilename,
		))
	data, err := cloudConfig.Render()
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Make sure a template exists that we can clone from.
func (manager *containerManager) ensureCloneTemplate(
	machineConfig *cloudinit.MachineConfig,
	series string,
	network *container.NetworkConfig,
) (golxc.Container, error) {
	name := fmt.Sprintf("juju-%s-template", series)
	containerDirectory, err := container.NewDirectory(name)
	if err != nil {
		return nil, err
	}

	logger.Infof("wait for fslock on %v", name)
	lock, err := fslock.NewLock(
		filepath.Join(machineConfig.DataDir, "locks"),
		name)
	if err != nil {
		logger.Tracef("failed to create gslock for template: %v", err)
		return nil, err
	}
	err = lock.Lock("manager ensure existence")
	if err != nil {
		logger.Tracef("failed to acquire lock for template: %v", err)
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

	userData, err := templateUserData(machineConfig)
	if err != nil {
		logger.Tracef("filed to create tempalte user data for template: %v", err)
		return nil, err
	}
	userDataFilename, err := container.WriteCloudInitFile(containerDirectory, userData)
	if err != nil {
		return nil, err
	}

	noAutostart := false
	configFile, err := writeLxcConfig(network, containerDirectory, manager.logdir, noAutostart)
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
	// Create the container.
	logger.Tracef("create the container")
	if err := lxcContainer.Create(configFile, defaultTemplate, nil, templateParams); err != nil {
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

	tailWriter := &logTail{}
	consoleTailer := tailer.NewTailer(console, tailWriter, 0, nil)
	defer consoleTailer.Stop()

	// We should wait maybe 1 minute between output?
	// if no output check to see if stopped
	// If we have no output and still running, something has probably gone wrong
	for lxcContainer.IsRunning() {
		if tailWriter.lastTick().Before(time.Now().Add(-5 * time.Minute)) {
			logger.Infof("not heard anything from the template log for five minutes")
			return nil, fmt.Errorf("template container did not stop")
		}
		time.Sleep(200 * time.Millisecond)
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
