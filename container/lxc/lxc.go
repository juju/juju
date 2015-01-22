// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
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
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/version"
	"github.com/juju/utils/keyvalues"
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
	return container.BridgeNetworkConfig(DefaultLxcBridge, nil)
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
	imageURLGetter    container.ImageURLGetter
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

// NewContainerManager returns a manager object that can start and
// stop lxc containers. The containers that are created are namespaced
// by the name parameter inside the given ManagerConfig.
func NewContainerManager(conf container.ManagerConfig, imageURLGetter container.ImageURLGetter) (container.Manager, error) {
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
		imageURLGetter:    imageURLGetter,
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
			manager.imageURLGetter,
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
		if err = createContainer(lxcContainer, network, directory, nil, templateParams, caCert); err != nil {
			return nil, nil, err
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

func createContainer(lxcContainer golxc.Container, network *container.NetworkConfig, containerDirectory string,
	extraCreateArgs, templateParams []string, caCert []byte,
) error {
	logger.Tracef("write the lxc.conf file")
	configFile, err := writeLxcConfig(network, containerDirectory)
	if err != nil {
		return errors.Annotatef(err, "failed to write config file")
	}

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
	logger.Debugf("create the lxc container")
	logger.Debugf("lxc-create template params: %v", templateParams)
	if err := lxcContainer.Create(configFile, defaultTemplate, extraCreateArgs, templateParams, execEnv); err != nil {
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
		return nil, nil, err
	}
	// Create a wget bash script in a temporary directory.
	tmpDir, err := ioutil.TempDir("", "wget")
	if err != nil {
		return nil, nil, err
	}
	closer = func() {
		os.RemoveAll(tmpDir)
	}
	// Write the ca cert.
	caCertPath := filepath.Join(tmpDir, "ca-cert.pem")
	err = ioutil.WriteFile(caCertPath, caCert, 0755)
	if err != nil {
		defer closer()
		return nil, nil, err
	}

	// Write the wget script.
	wgetTmpl := `#!/bin/bash
/usr/bin/wget --ca-certificate=%s $*
`
	wget := fmt.Sprintf(wgetTmpl, caCertPath)
	err = ioutil.WriteFile(filepath.Join(tmpDir, "wget"), []byte(wget), 0755)
	if err != nil {
		defer closer()
		return nil, nil, err
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
	}

	primaryNIC, err := discoverHostNIC()
	if err != nil {
		logger.Warningf("cannot determine primary NIC MTU, not setting for container: %v", err)
	} else if primaryNIC.MTU > 0 {
		logger.Infof("setting MTU to %d for all container network interfaces", primaryNIC.MTU)
		data.MTU = primaryNIC.MTU
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
			// octets). So we need to extract the mask part of the
			// iface.CIDR and append it. If CIDR is empty or invalid
			// "/24" is used as a sane default.
			_, ipNet, err := net.ParseCIDR(iface.CIDR)
			if err != nil {
				logger.Warningf(
					"invalid CIDR %q for interface %q, using /24 as fallback",
					iface.CIDR, nic.Name,
				)
				nic.IPv4Address += "/24"
			} else {
				ones, _ := ipNet.Mask.Size()
				nic.IPv4Address += fmt.Sprintf("/%d", ones)
			}
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
		templateName = singleNICTemplate
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

// discoverHostNIC detects and returns the primary network interface
// on the machine. Out of all interfaces, the first non-loopback
// device which is up and has address is considered the primary.
var discoverHostNIC = func() (net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, errors.Annotatef(err, "cannot get network interfaces")
	}
	logger.Tracef("trying to discover primary network interface")
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			// Skip the loopback.
			logger.Tracef("not using loopback interface %q", iface.Name)
			continue
		}
		if iface.Flags&net.FlagUp != 0 {
			// Possibly the primary, but ensure it has an address as
			// well.
			logger.Tracef("verifying interface %q has addresses", iface.Name)
			addrs, err := iface.Addrs()
			if err != nil {
				return net.Interface{}, errors.Annotatef(err, "cannot get %q addresses", iface.Name)
			}
			if len(addrs) > 0 {
				// We found it.
				logger.Tracef("primary network interface is %q", iface.Name)
				return iface, nil
			}
		}
	}
	return net.Interface{}, errors.Errorf("cannot detect the primary network interface")
}

func generateNetworkConfig(config *container.NetworkConfig) string {
	if config == nil {
		config = DefaultNetworkConfig()
		logger.Warningf("network type missing, using the default %q config", config.NetworkType)
	}
	return networkConfigTemplate(*config)
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
