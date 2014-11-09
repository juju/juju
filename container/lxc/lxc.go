// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/symlink"
	"launchpad.net/golxc"

	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.container.lxc")

var (
	defaultTemplate       = "ubuntu-cloud"
	LxcContainerDir       = golxc.GetDefaultLXCContainerDir()
	LxcRestartDir         = "/etc/lxc/auto"
	LxcObjectFactory      = golxc.Factory()
	initProcessCgroupFile = "/proc/1/cgroup"
	runtimeGOOS           = runtime.GOOS
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

// IsLXCSupported returns a boolean value indicating whether or not
// we can run LXC containers
func IsLXCSupported() (bool, error) {
	if runtimeGOOS != "linux" {
		return false, nil
	}

	file, err := os.Open(initProcessCgroupFile)
	if err != nil {
		return false, errors.Trace(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) != 3 {
			return false, errors.Errorf("Malformed cgroup file")
		}
		if fields[2] != "/" {
			// When running in a container the anchor point will be something
			// other then "/". Return false here as we do not support nested LXC
			// containers
			return false, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, errors.Errorf("Failed to read cgroup file")
	}
	return true, nil
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

// CreateContainer creates or clones an LXC container.
func (manager *containerManager) CreateContainer(
	machineConfig *cloudinit.MachineConfig,
	series string,
	network *container.NetworkConfig,
) (inst instance.Instance, _ *instance.HardwareCharacteristics, err error) {

	// Check our preconditions
	if manager == nil {
		panic("manager is nil")
	} else if series == "" {
		panic("series not set")
	} else if network == nil {
		panic("network is nil")
	}

	// Log how long the start took
	defer func(start time.Time) {
		if err == nil {
			logger.Tracef("container %q started: %v", inst.Id(), time.Now().Sub(start))
		}
	}(time.Now())

	name := names.NewMachineTag(machineConfig.MachineId).String()
	if manager.name != "" {
		name = fmt.Sprintf("%s-%s", manager.name, name)
	}

	// Create the cloud-init.
	directory, err := container.NewDirectory(name)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create a directory for the container")
	}
	logger.Tracef("write cloud-init")
	userDataFilename, err := container.WriteUserData(machineConfig, directory)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to write user data")
	}

	logger.Tracef("write the lxc.conf file")
	configFile, err := writeLxcConfig(network, directory)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to write config file")
	}

	var lxcContainer golxc.Container
	if manager.createWithClone {
		templateContainer, err := EnsureCloneTemplate(
			manager.backingFilesystem,
			series,
			network,
			machineConfig.AuthorizedKeys,
			machineConfig.AptProxySettings,
			machineConfig.AptMirror,
			machineConfig.EnableOSRefreshUpdate,
			machineConfig.EnableOSUpgrade,
		)
		if err != nil {
			return nil, nil, errors.Annotate(err, "failed to retrieve the template to clone")
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
			return nil, nil, errors.Annotate(err, "failed to acquire lock on template")
		}
		defer lock.Unlock()
		lxcContainer, err = templateContainer.Clone(name, extraCloneArgs, templateParams)
		if err != nil {
			return nil, nil, errors.Annotate(err, "lxc container cloning failed")
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
			return nil, nil, errors.Annotate(err, "lxc container creation failed")
		}
	}

	if err := autostartContainer(name); err != nil {
		return nil, nil, errors.Annotate(err, "failed to configure the container for autostart")
	}
	if err := mountHostLogDir(name, manager.logdir); err != nil {
		return nil, nil, errors.Annotate(err, "failed to mount the directory to log to")
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
		logger.Warningf("container failed to start %v", err)
		// if the container fails to start we should try to destroy it
		// check if the container has been constructed
		if lxcContainer.IsConstructed() {
			// if so, then we need to destroy the leftover container
			if derr := lxcContainer.Destroy(); derr != nil {
				// if an error is reported there is probably a leftover
				// container that the user should clean up manually
				logger.Errorf("container failed to start and failed to destroy: %v", derr)
				return nil, nil, errors.Annotate(err, "container failed to start and failed to destroy: manual cleanup of containers needed")
			}
			logger.Warningf("container failed to start and was destroyed - safe to retry")
			return nil, nil, errors.Wrap(err, instance.NewRetryableCreationError("container failed to start and was destroyed: "+lxcContainer.Name()))
		}
		logger.Warningf("container failed to start: %v", err)
		return nil, nil, errors.Annotate(err, "container failed to start")
	}

	hardware := &instance.HardwareCharacteristics{
		Arch: &version.Current.Arch,
	}

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
		if err := symlink.New(
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
	internalDir := internalLogDir(name)
	logger.Tracef("make the mount dir for the shared logs: %s", internalDir)
	if err := os.MkdirAll(internalDir, 0755); err != nil {
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

func (manager *containerManager) IsInitialized() bool {
	requiredBinaries := []string{
		"lxc-ls",
	}
	for _, bin := range requiredBinaries {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}
	return true
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
