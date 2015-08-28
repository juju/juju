// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/keyvalues"
	"github.com/juju/utils/symlink"
	"launchpad.net/golxc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxc/lxcutils"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/storage/looputil"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.container.lxc")

var (
	defaultTemplate  = "ubuntu-cloud"
	LxcContainerDir  = golxc.GetDefaultLXCContainerDir()
	LxcRestartDir    = "/etc/lxc/auto"
	LxcObjectFactory = golxc.Factory()
	runtimeGOOS      = runtime.GOOS
	runningInsideLXC = lxcutils.RunningInsideLXC
	writeWgetTmpFile = ioutil.WriteFile
)

const (
	// DefaultLxcBridge is the package created container bridge
	DefaultLxcBridge = "lxcbr0"
	// Btrfs is special as we treat it differently for create and clone.
	Btrfs = "btrfs"

	// etcNetworkInterfaces here is the path (inside the container's
	// rootfs) where the network config is stored.
	etcNetworkInterfaces = "/etc/network/interfaces"
)

// DefaultNetworkConfig returns a valid NetworkConfig to use the
// defaultLxcBridge that is created by the lxc package.
func DefaultNetworkConfig() *container.NetworkConfig {
	return container.BridgeNetworkConfig(DefaultLxcBridge, 0, nil)
}

// FsCommandOutput calls cmd.Output, this is used as an overloading point so
// we can test what *would* be run without actually executing another program
var FsCommandOutput = (*exec.Cmd).CombinedOutput

func containerDirFilesystem() (string, error) {
	cmd := exec.Command("df", "--output=fstype", LxcContainerDir)
	out, err := FsCommandOutput(cmd)
	if err != nil {
		return "", errors.Trace(err)
	}
	// The filesystem is the second line.
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		logger.Errorf("unexpected output: %q", out)
		return "", errors.Errorf("could not determine filesystem type")
	}
	return lines[1], nil
}

// IsLXCSupported returns a boolean value indicating whether or not
// we can run LXC containers.
func IsLXCSupported() (bool, error) {
	if runtimeGOOS != "linux" {
		return false, nil
	}
	// We do not support running nested LXC containers.
	insideLXC, err := runningInsideLXC()
	if err != nil {
		return false, errors.Trace(err)
	}
	return !insideLXC, nil
}

type containerManager struct {
	name              string
	logdir            string
	createWithClone   bool
	useAUFS           bool
	backingFilesystem string
	imageURLGetter    container.ImageURLGetter
	loopDeviceManager looputil.LoopDeviceManager
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

// NewContainerManager returns a manager object that can start and
// stop lxc containers. The containers that are created are namespaced
// by the name parameter inside the given ManagerConfig.
func NewContainerManager(
	conf container.ManagerConfig,
	imageURLGetter container.ImageURLGetter,
	loopDeviceManager looputil.LoopDeviceManager,
) (container.Manager, error) {
	name := conf.PopValue(container.ConfigName)
	if name == "" {
		return nil, errors.Errorf("name is required")
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
		imageURLGetter:    imageURLGetter,
		loopDeviceManager: loopDeviceManager,
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
	instanceConfig *instancecfg.InstanceConfig,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
) (inst instance.Instance, _ *instance.HardwareCharacteristics, err error) {
	// Check our preconditions
	if manager == nil {
		panic("manager is nil")
	} else if series == "" {
		panic("series not set")
	} else if networkConfig == nil {
		panic("networkConfig is nil")
	} else if storageConfig == nil {
		panic("storageConfig is nil")
	}

	// Log how long the start took
	defer func(start time.Time) {
		if err == nil {
			logger.Tracef("container %q started: %v", inst.Id(), time.Now().Sub(start))
		}
	}(time.Now())

	name := names.NewMachineTag(instanceConfig.MachineId).String()
	if manager.name != "" {
		name = fmt.Sprintf("%s-%s", manager.name, name)
	}

	// Create the cloud-init.
	directory, err := container.NewDirectory(name)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create a directory for the container")
	}
	logger.Tracef("write cloud-init")
	userDataFilename, err := containerinit.WriteUserData(instanceConfig, networkConfig, directory)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to write user data")
	}

	var lxcContainer golxc.Container
	if manager.createWithClone {
		templateContainer, err := EnsureCloneTemplate(
			manager.backingFilesystem,
			series,
			networkConfig,
			instanceConfig.AuthorizedKeys,
			instanceConfig.AptProxySettings,
			instanceConfig.AptMirror,
			instanceConfig.EnableOSRefreshUpdate,
			instanceConfig.EnableOSUpgrade,
			manager.imageURLGetter,
			manager.useAUFS,
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

		// Ensure the run-time effective config of the template
		// container has correctly ordered network settings, otherwise
		// Clone() below will fail. This is needed in case we haven't
		// created a new template now but are reusing an existing one.
		// See LP bug #1414016.
		configPath := containerConfigFilename(templateContainer.Name())
		if _, err := reorderNetworkConfig(configPath); err != nil {
			return nil, nil, errors.Annotate(err, "failed to reorder network settings")
		}

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
		var caCert []byte
		if manager.imageURLGetter != nil {
			arch := arch.HostArch()
			imageURL, err := manager.imageURLGetter.ImageURL(instance.LXC, series, arch)
			if err != nil {
				return nil, nil, errors.Annotatef(err, "cannot determine cached image URL")
			}
			templateParams = append(templateParams, "-T", imageURL)
			caCert = manager.imageURLGetter.CACert()
		}
		err = createContainer(
			lxcContainer,
			directory,
			networkConfig,
			nil,
			templateParams,
			caCert,
		)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	}

	if err := autostartContainer(name); err != nil {
		return nil, nil, errors.Annotate(err, "failed to configure the container for autostart")
	}
	if err := mountHostLogDir(name, manager.logdir); err != nil {
		return nil, nil, errors.Annotate(err, "failed to mount the directory to log to")
	}
	if storageConfig.AllowMount {
		// Add config to allow loop devices to be mounted inside the container.
		if err := allowLoopbackBlockDevices(name); err != nil {
			return nil, nil, errors.Annotate(err, "failed to configure the container for loopback devices")
		}
	}
	// Update the network settings inside the run-time config of the
	// container (e.g. /var/lib/lxc/<name>/config) before starting it.
	netConfig := generateNetworkConfig(networkConfig)
	if err := updateContainerConfig(name, netConfig); err != nil {
		return nil, nil, errors.Annotate(err, "failed to update network config")
	}
	configPath := containerConfigFilename(name)
	logger.Tracef("updated network config in %q for container %q", configPath, name)
	// Ensure the run-time config of the new container has correctly
	// ordered network settings, otherwise Start() below will fail. We
	// need this now because after lxc-create or lxc-clone the initial
	// lxc.conf generated inside createContainer gets merged with
	// other settings (e.g. system-wide overrides, changes made by
	// hooks, etc.) and the result can still be incorrectly ordered.
	// See LP bug #1414016.
	if _, err := reorderNetworkConfig(configPath); err != nil {
		return nil, nil, errors.Annotate(err, "failed to reorder network settings")
	}

	// To speed-up the initial container startup we pre-render the
	// /etc/network/interfaces directly inside the rootfs. This won't
	// work if we use AUFS snapshots, so it's disabled if useAUFS is
	// true (for now).
	if networkConfig != nil && len(networkConfig.Interfaces) > 0 {
		interfacesFile := filepath.Join(LxcContainerDir, name, "rootfs", etcNetworkInterfaces)
		if manager.useAUFS {
			logger.Tracef("not pre-rendering %q when using AUFS-backed rootfs", interfacesFile)
		} else {
			data, err := containerinit.GenerateNetworkConfig(networkConfig)
			if err != nil {
				return nil, nil, errors.Annotatef(err, "failed to generate %q", interfacesFile)
			}
			if err := utils.AtomicWriteFile(interfacesFile, []byte(data), 0644); err != nil {
				return nil, nil, errors.Annotatef(err, "cannot write generated %q", interfacesFile)
			}
			logger.Tracef("pre-rendered network config in %q", interfacesFile)
		}
	}

	// Start the lxc container with the appropriate settings for
	// grabbing the console output and a log file.
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

	arch := arch.HostArch()
	hardware := &instance.HardwareCharacteristics{
		Arch: &arch,
	}

	return &lxcInstance{lxcContainer, name}, hardware, nil
}

func createContainer(
	lxcContainer golxc.Container,
	directory string,
	networkConfig *container.NetworkConfig,
	extraCreateArgs, templateParams []string,
	caCert []byte,
) error {
	// Generate initial lxc.conf with networking settings.
	netConfig := generateNetworkConfig(networkConfig)
	configPath := filepath.Join(directory, "lxc.conf")
	if err := ioutil.WriteFile(configPath, []byte(netConfig), 0644); err != nil {
		return errors.Annotatef(err, "failed to write container config %q", configPath)
	}
	logger.Tracef("wrote initial config %q for container %q", configPath, lxcContainer.Name())

	var err error
	var execEnv []string = nil
	var closer func()
	if caCert != nil {
		execEnv, closer, err = wgetEnvironment(caCert)
		if err != nil {
			return errors.Annotatef(err, "failed to get environment for wget execution")
		}
		defer closer()
	}

	// Create the container.
	logger.Debugf("creating lxc container %q", lxcContainer.Name())
	logger.Debugf("lxc-create template params: %v", templateParams)
	if err := lxcContainer.Create(configPath, defaultTemplate, extraCreateArgs, templateParams, execEnv); err != nil {
		return errors.Annotatef(err, "lxc container creation failed")
	}
	return nil
}

// wgetEnvironment creates a script to call wget with the
// --no-check-certificate argument, patching the PATH to ensure
// the script is invoked by the lxc template bash script.
// It returns a slice of env variables to pass to the lxc create command.
func wgetEnvironment(caCert []byte) (execEnv []string, closer func(), _ error) {
	env := os.Environ()
	kv, err := keyvalues.Parse(env, true)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	// Create a wget bash script in a temporary directory.
	tmpDir, err := ioutil.TempDir("", "wget")
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	closer = func() {
		os.RemoveAll(tmpDir)
	}
	// Write the ca cert.
	caCertPath := filepath.Join(tmpDir, "ca-cert.pem")
	err = ioutil.WriteFile(caCertPath, caCert, 0755)
	if err != nil {
		defer closer()
		return nil, nil, errors.Trace(err)
	}

	// Write the wget script.  Don't use a proxy when getting
	// the image as it's going through the state server.
	wgetTmpl := `#!/bin/bash
/usr/bin/wget --no-proxy --ca-certificate=%s $*
`
	wget := fmt.Sprintf(wgetTmpl, caCertPath)
	err = writeWgetTmpFile(filepath.Join(tmpDir, "wget"), []byte(wget), 0755)
	if err != nil {
		defer closer()
		return nil, nil, errors.Trace(err)
	}

	// Update the path to point to the script.
	for k, v := range kv {
		if strings.ToUpper(k) == "PATH" {
			v = strings.Join([]string{tmpDir, v}, string(os.PathListSeparator))
		}
		execEnv = append(execEnv, fmt.Sprintf("%s=%s", k, v))
	}
	return execEnv, closer, nil
}

// parseConfigLine tries to parse a line from an LXC config file.
// Empty lines, comments, and lines not starting with "lxc." are
// ignored. If successful the setting and its value are returned
// stripped of leading/trailing whitespace and line comments (e.g.
// "lxc.rootfs" and "/some/path"), otherwise both results are empty.
func parseConfigLine(line string) (setting, value string) {
	input := strings.TrimSpace(line)
	if len(input) == 0 ||
		strings.HasPrefix(input, "#") ||
		!strings.HasPrefix(input, "lxc.") {
		return "", ""
	}
	parts := strings.SplitN(input, "=", 2)
	if len(parts) != 2 {
		// Still not what we're looking for.
		return "", ""
	}
	setting = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	if strings.Contains(value, "#") {
		parts = strings.SplitN(value, "#", 2)
		if len(parts) == 2 {
			value = strings.TrimSpace(parts[0])
		}
	}
	return setting, value
}

// updateContainerConfig selectively replaces, deletes, and/or appends
// lines in the named container's current config file, depending on
// the contents of newConfig. First, newConfig is split into multiple
// lines and parsed, ignoring comments, empty lines and spaces. Then
// the occurrence of a setting in a line of the config file will be
// replaced by values from newConfig. Values in newConfig are only
// used once (in the order provided), so multiple replacements must be
// supplied as multiple input values for the same setting in
// newConfig. If the value of a setting is empty, the setting will be
// removed if found. Settings that are not found and have values will
// be appended (also if more values are given than exist).
//
// For example, with existing config like "lxc.foo = off\nlxc.bar=42\n",
// and newConfig like "lxc.bar=\nlxc.foo = bar\nlxc.foo = baz # xx",
// the updated config file contains "lxc.foo = bar\nlxc.foo = baz\n".
// TestUpdateContainerConfig has this example in code.
func updateContainerConfig(name, newConfig string) error {
	lines := strings.Split(newConfig, "\n")
	if len(lines) == 0 {
		return nil
	}
	// Extract unique set of line prefixes to match later. Also, keep
	// a slice of values to replace for each key, as well as a slice
	// of parsed prefixes to preserve the order when replacing or
	// appending.
	parsedLines := make(map[string][]string)
	var parsedPrefixes []string
	for _, line := range lines {
		prefix, value := parseConfigLine(line)
		if prefix == "" {
			// Ignore comments, empty lines, and unknown prefixes.
			continue
		}
		if values, found := parsedLines[prefix]; !found {
			parsedLines[prefix] = []string{value}
		} else {
			values = append(values, value)
			parsedLines[prefix] = values
		}
		parsedPrefixes = append(parsedPrefixes, prefix)
	}

	path := containerConfigFilename(name)
	currentConfig, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Annotatef(err, "cannot open config %q for container %q", path, name)
	}
	input := bytes.NewBuffer(currentConfig)

	// Read the original config and prepare the output to replace it
	// with.
	var output bytes.Buffer
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		prefix, _ := parseConfigLine(line)
		values, found := parsedLines[prefix]
		if !found || len(values) == 0 {
			// No need to change, just preserve.
			output.WriteString(line + "\n")
			continue
		}
		// We need to change this line. Pop the first value of the
		// list and set it.
		var newValue string
		newValue, values = values[0], values[1:]
		parsedLines[prefix] = values

		if newValue == "" {
			logger.Tracef("removing %q from container %q config %q", line, name, path)
			continue
		}
		newLine := prefix + " = " + newValue
		if newLine == line {
			// No need to change and pollute the log, just write it.
			output.WriteString(line + "\n")
			continue
		}
		logger.Tracef(
			"replacing %q with %q in container %q config %q",
			line, newLine, name, path,
		)
		output.WriteString(newLine + "\n")
	}
	if err := scanner.Err(); err != nil {
		return errors.Annotatef(err, "cannot read config for container %q", name)
	}

	// Now process any prefixes with values still left for appending,
	// in the original order.
	for _, prefix := range parsedPrefixes {
		values := parsedLines[prefix]
		for _, value := range values {
			if value == "" {
				// No need to remove what's not there.
				continue
			}
			newLine := prefix + " = " + value + "\n"
			logger.Tracef("appending %q to container %q config %q", newLine, name, path)
			output.WriteString(newLine)
		}
		// Reset the values, so we only append the once per prefix.
		parsedLines[prefix] = []string{}
	}

	// Reset the original file and overwrite it atomically.
	if err := utils.AtomicWriteFile(path, output.Bytes(), 0644); err != nil {
		return errors.Annotatef(err, "cannot write new config %q for container %q", path, name)
	}
	return nil
}

// reorderNetworkConfig reads the contents of the given container
// config file and the modifies the contents, if needed, so that any
// lxc.network.* setting comes after the first lxc.network.type
// setting preserving the order. Every line formatting is preserved in
// the modified config, including whitespace and comments. The
// wasReordered flag will be set if the config was modified.
//
// This ensures the lxc tools won't report parsing errors for network
// settings. See also LP bug #1414016.
func reorderNetworkConfig(configFile string) (wasReordered bool, err error) {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return false, errors.Annotatef(err, "cannot read config %q", configFile)
	}
	if len(data) == 0 {
		// Nothing to do.
		return false, nil
	}
	input := bytes.NewBuffer(data)
	scanner := bufio.NewScanner(input)
	var output bytes.Buffer
	firstNetworkType := ""
	var networkLines []string
	mayNeedReordering := true
	foundFirstType := false
	doneReordering := false
	for scanner.Scan() {
		line := scanner.Text()
		prefix, _ := parseConfigLine(line)
		if mayNeedReordering {
			if strings.HasPrefix(prefix, "lxc.network.type") {
				if len(networkLines) == 0 {
					// All good, no need to change.
					logger.Tracef("correct network settings order in config %q", configFile)
					return false, nil
				}
				// We found the first type.
				firstNetworkType = line
				foundFirstType = true
				logger.Tracef(
					"moving line(s) %q after line %q in config %q",
					strings.Join(networkLines, "\n"), firstNetworkType, configFile,
				)
			} else if strings.HasPrefix(prefix, "lxc.network.") {
				if firstNetworkType != "" {
					// All good, no need to change.
					logger.Tracef("correct network settings order in config %q", configFile)
					return false, nil
				}
				networkLines = append(networkLines, line)
				logger.Tracef("need to move line %q in config %q", line, configFile)
				continue
			}
		}
		output.WriteString(line + "\n")
		if foundFirstType && len(networkLines) > 0 {
			// Now add the skipped networkLines.
			output.WriteString(strings.Join(networkLines, "\n") + "\n")
			doneReordering = true
			mayNeedReordering = false
			firstNetworkType = ""
			networkLines = nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, errors.Annotatef(err, "cannot read config %q", configFile)
	}
	if !doneReordering {
		if len(networkLines) > 0 {
			logger.Errorf("invalid lxc network settings in config %q", configFile)
			return false, errors.Errorf(
				"cannot have line(s) %q without lxc.network.type in config %q",
				strings.Join(networkLines, "\n"), configFile,
			)
		}
		// No networking settings to reorder.
		return false, nil
	}
	// Reset the original file and overwrite it atomically.
	if err := utils.AtomicWriteFile(configFile, output.Bytes(), 0644); err != nil {
		return false, errors.Annotatef(err, "cannot write new config %q", configFile)
	}
	logger.Tracef("reordered network settings in config %q", configFile)
	return true, nil
}

func appendToContainerConfig(name, line string) error {
	configPath := containerConfigFilename(name)
	file, err := os.OpenFile(configPath, os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line)
	if err != nil {
		return err
	}
	logger.Tracef("appended %q to config %q", line, configPath)
	return nil
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
	// Ensure that the logDir actually exists.
	if err := os.MkdirAll(logDir, 0777); err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("make the mount dir for the shared logs: %s", internalDir)
	if err := os.MkdirAll(internalDir, 0755); err != nil {
		logger.Errorf("failed to create internal /var/log/juju mount dir: %v", err)
		return err
	}
	line := fmt.Sprintf(
		"lxc.mount.entry = %s var/log/juju none defaults,bind 0 0\n",
		logDir)
	return appendToContainerConfig(name, line)
}

func allowLoopbackBlockDevices(name string) error {
	const allowLoopDevicesCfg = `
lxc.aa_profile = lxc-container-default-with-mounting
lxc.cgroup.devices.allow = b 7:* rwm
lxc.cgroup.devices.allow = c 10:237 rwm
`
	return appendToContainerConfig(name, allowLoopDevicesCfg)
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

	// Detach loop devices backed by files inside the container's rootfs.
	rootfs := filepath.Join(LxcContainerDir, name, "rootfs")
	if err := manager.loopDeviceManager.DetachLoopDevices(rootfs, "/"); err != nil {
		logger.Errorf("failed to detach loop devices from lxc container: %v", err)
		return err
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

const singleNICTemplate = `
# network config
# interface "eth0"
lxc.network.type = {{.Type}}
lxc.network.link = {{.Link}}
lxc.network.flags = up{{if .MTU}}
lxc.network.mtu = {{.MTU}}{{end}}

`

const multipleNICsTemplate = `
# network config{{$mtu := .MTU}}{{range $nic := .Interfaces}}
{{$nic.Name | printf "# interface %q"}}
lxc.network.type = {{$nic.Type}}{{if $nic.VLANTag}}
lxc.network.vlan.id = {{$nic.VLANTag}}{{end}}
lxc.network.link = {{$nic.Link}}{{if not $nic.NoAutoStart}}
lxc.network.flags = up{{end}}
lxc.network.name = {{$nic.Name}}{{if $nic.MACAddress}}
lxc.network.hwaddr = {{$nic.MACAddress}}{{end}}{{if $nic.IPv4Address}}
lxc.network.ipv4 = {{$nic.IPv4Address}}{{end}}{{if $nic.IPv4Gateway}}
lxc.network.ipv4.gateway = {{$nic.IPv4Gateway}}{{end}}{{if $mtu}}
lxc.network.mtu = {{$mtu}}{{end}}
{{end}}{{/* range */}}

`

func networkConfigTemplate(config container.NetworkConfig) string {
	type nicData struct {
		Name        string
		NoAutoStart bool
		Type        string
		Link        string
		VLANTag     int
		MACAddress  string
		IPv4Address string
		IPv4Gateway string
	}
	type configData struct {
		Type       string
		Link       string
		MTU        int
		Interfaces []nicData
	}
	data := configData{
		Link: config.Device,
		MTU:  config.MTU,
	}
	if config.MTU > 0 {
		logger.Infof("setting MTU to %v for all LXC network interfaces", config.MTU)
	}

	switch config.NetworkType {
	case container.PhysicalNetwork:
		data.Type = "phys"
	case container.BridgeNetwork:
		data.Type = "veth"
	default:
		logger.Warningf(
			"unknown network type %q, using the default %q config",
			config.NetworkType, container.BridgeNetwork,
		)
		data.Type = "veth"
	}
	for _, iface := range config.Interfaces {
		nic := nicData{
			Type:        data.Type,
			Link:        config.Device,
			Name:        iface.InterfaceName,
			NoAutoStart: iface.NoAutoStart,
			VLANTag:     iface.VLANTag,
			MACAddress:  iface.MACAddress,
			IPv4Address: iface.Address.Value,
			IPv4Gateway: iface.GatewayAddress.Value,
		}
		if iface.VLANTag > 0 {
			nic.Type = "vlan"
		}
		if nic.IPv4Address != "" {
			// LXC expects IPv4 addresses formatted like a CIDR:
			// 1.2.3.4/5 (but without masking the least significant
			// octets). Because statically configured IP addresses use
			// the netmask 255.255.255.255, we always use /32 for
			// here.
			nic.IPv4Address += "/32"
		}
		if nic.NoAutoStart && nic.IPv4Gateway != "" {
			// LXC refuses to add an ipv4 gateway when the NIC is not
			// brought up.
			logger.Warningf(
				"not setting IPv4 gateway %q for non-auto start interface %q",
				nic.IPv4Gateway, nic.Name,
			)
			nic.IPv4Gateway = ""
		}

		data.Interfaces = append(data.Interfaces, nic)
	}
	templateName := multipleNICsTemplate
	if len(config.Interfaces) == 0 {
		logger.Tracef("generating default single NIC network config")
		templateName = singleNICTemplate
	} else {
		logger.Tracef("generating network config with %d NIC(s)", len(config.Interfaces))
	}
	tmpl, err := template.New("config").Parse(templateName)
	if err != nil {
		logger.Errorf("cannot parse container config template: %v", err)
		return ""
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		logger.Errorf("cannot render container config: %v", err)
		return ""
	}
	return buf.String()
}

func generateNetworkConfig(config *container.NetworkConfig) string {
	if config == nil {
		config = DefaultNetworkConfig()
		logger.Warningf("network type missing, using the default %q config", config.NetworkType)
	}
	return networkConfigTemplate(*config)
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
