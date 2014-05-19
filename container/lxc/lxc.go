// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/juju/loggo"
	"launchpad.net/golxc"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
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
		logger.Errorf("unexpected output: %q", out)
		return "", fmt.Errorf("could not determine filesystem type")
	}
	return lines[1], nil
}

type containerManager struct {
	name              string
	logdir            string
	createWithClone   bool
	useAUFS           bool
	backingFilesystem string
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

// NewContainerManager returns a manager object that can start and stop lxc
// containers. The containers that are created are namespaced by the name
// parameter.
func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	name := conf.PopValue(container.ConfigName)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	logDir := conf.PopValue(container.ConfigLogDir)
	if logDir == "" {
		logDir = agent.DefaultLogDir
	}
	var useClone bool
	useCloneVal := conf.PopValue("use-clone")
	if useCloneVal != "" {
		// Explicitly ignore the error result from ParseBool.
		// If it fails to parse, the value is false, and this suits
		// us fine.
		useClone, _ = strconv.ParseBool(useCloneVal)
	} else {
		// If no lxc-clone value is explicitly set in config, then
		// see if the Ubuntu series we are running on supports it
		// and if it does, we will use clone.
		useClone = preferFastLXC(releaseVersion())
	}
	useAUFS, _ := strconv.ParseBool(conf.PopValue("use-aufs"))
	backingFS, err := containerDirFilesystem()
	if err != nil {
		// Especially in tests, or a bot, the lxc dir may not exist
		// causing the test to fail. Since we only really care if the
		// backingFS is 'btrfs' and we treat the rest the same, just
		// call it 'unknown'.
		backingFS = "unknown"
	}
	logger.Tracef("backing filesystem: %q", backingFS)
	conf.WarnAboutUnused()
	return &containerManager{
		name:              name,
		logdir:            logDir,
		createWithClone:   useClone,
		useAUFS:           useAUFS,
		backingFilesystem: backingFS,
	}, nil
}

// releaseVersion is a function that returns a string representing the
// DISTRIB_RELEASE from the /etc/lsb-release file.
var releaseVersion = version.ReleaseVersion

// preferFastLXC returns true if the host is capable of
// LXC cloning from a template.
func preferFastLXC(release string) bool {
	if release == "" {
		return false
	}
	value, err := strconv.ParseFloat(release, 64)
	if err != nil {
		return false
	}
	return value >= 14.04
}

func (manager *containerManager) CreateContainer(
	machineConfig *cloudinit.MachineConfig,
	series string,
	network *container.NetworkConfig,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	start := time.Now()
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
	configFile, err := writeLxcConfig(network, directory)
	if err != nil {
		logger.Errorf("failed to write config file: %v", err)
		return nil, nil, err
	}

	var lxcContainer golxc.Container
	if manager.createWithClone {
		templateContainer, err := EnsureCloneTemplate(
			manager.backingFilesystem,
			series,
			network,
			machineConfig.AuthorizedKeys,
			machineConfig.AptProxySettings,
		)
		if err != nil {
			return nil, nil, err
		}
		templateParams := []string{
			"--debug",                      // Debug errors in the cloud image
			"--userdata", userDataFilename, // Our groovey cloud-init
			"--hostid", name, // Use the container name as the hostid
		}
		var extraCloneArgs []string
		if manager.backingFilesystem == Btrfs || manager.useAUFS {
			extraCloneArgs = append(extraCloneArgs, "--snapshot")
		}
		if manager.backingFilesystem != Btrfs && manager.useAUFS {
			extraCloneArgs = append(extraCloneArgs, "--backingstore", "aufs")
		}

		lock, err := AcquireTemplateLock(templateContainer.Name(), "clone")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to acquire lock on template: %v", err)
		}
		defer lock.Unlock()
		lxcContainer, err = templateContainer.Clone(name, extraCloneArgs, templateParams)
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
		logger.Tracef("lxc container created")
	}
	if err := autostartContainer(name); err != nil {
		return nil, nil, err
	}
	if err := mountHostLogDir(name, manager.logdir); err != nil {
		return nil, nil, err
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
	logger.Tracef("container %q started: %v", name, time.Now().Sub(start))
	return &lxcInstance{lxcContainer, name}, hardware, nil
}

func appendToContainerConfig(name, line string) error {
	file, err := os.OpenFile(
		containerConfigFilename(name), os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line)
	return err
}

func autostartContainer(name string) error {
	// Now symlink the config file into the restart directory, if it exists.
	// This is for backwards compatiblity. From Trusty onwards, the auto start
	// option should be set in the LXC config file, this is done in the networkConfigTemplate
	// function below.
	if useRestartDir() {
		if err := os.Symlink(
			containerConfigFilename(name),
			restartSymlink(name),
		); err != nil {
			return err
		}
		logger.Tracef("auto-restart link created")
	} else {
		logger.Tracef("Setting auto start to true in lxc config.")
		return appendToContainerConfig(name, "lxc.start.auto = 1\n")
	}
	return nil
}

func mountHostLogDir(name, logDir string) error {
	// Make sure that the mount dir has been created.
	logger.Tracef("make the mount dir for the shared logs")
	if err := os.MkdirAll(internalLogDir(name), 0755); err != nil {
		logger.Errorf("failed to create internal /var/log/juju mount dir: %v", err)
		return err
	}
	line := fmt.Sprintf(
		"lxc.mount.entry=%s var/log/juju none defaults,bind 0 0\n",
		logDir)
	return appendToContainerConfig(name, line)
}

func (manager *containerManager) DestroyContainer(id instance.Id) error {
	start := time.Now()
	name := string(id)
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

	err := container.RemoveDirectory(name)
	logger.Tracef("container %q stopped: %v", name, time.Now().Sub(start))
	return err
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

func containerConfigFilename(name string) string {
	return filepath.Join(LxcContainerDir, name, "config")
}

const networkTemplate = `
lxc.network.type = %s
lxc.network.link = %s
lxc.network.flags = up
`

func networkConfigTemplate(networkType, networkLink string) string {
	return fmt.Sprintf(networkTemplate, networkType, networkLink)
}

func generateNetworkConfig(network *container.NetworkConfig) string {
	var lxcConfig string
	if network == nil {
		logger.Warningf("network unspecified, using default networking config")
		network = DefaultNetworkConfig()
	}
	switch network.NetworkType {
	case container.PhysicalNetwork:
		lxcConfig = networkConfigTemplate("phys", network.Device)
	default:
		logger.Warningf("Unknown network config type %q: using bridge", network.NetworkType)
		fallthrough
	case container.BridgeNetwork:
		lxcConfig = networkConfigTemplate("veth", network.Device)
	}

	return lxcConfig
}

func writeLxcConfig(network *container.NetworkConfig, directory string) (string, error) {
	networkConfig := generateNetworkConfig(network)
	configFilename := filepath.Join(directory, "lxc.conf")
	if err := ioutil.WriteFile(configFilename, []byte(networkConfig), 0644); err != nil {
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
