// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuarch "github.com/juju/utils/arch"
	jujuos "github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"
	lxd "github.com/lxc/lxd/client"
	lxdshared "github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
)

var (
	logger = loggo.GetLogger("juju.container.lxd")
)

const lxdDefaultProfileName = "default"
const UserDataKey = "user.user-data"
const NetworkConfigKey = "user.network-config"
const JujuModelKey = "user.juju-model"
const AutoStartKey = "boot.autostart"

// XXX: should we allow managing containers on other hosts? this is
// functionality LXD gives us and from discussion juju would use eventually for
// the local provider, so the APIs probably need to be changed to pass extra
// args around. I'm punting for now.
type containerManager struct {
	modelUUID string
	namespace instance.Namespace
	server    lxd.ContainerServer
	// a host machine's availability zone
	availabilityZone string
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

/* The "releases" stream for images. This consists of blessed releases by the
 * Canonical team.
 */
var CloudImagesRemote = RemoteServer{
	Name:     "cloud-images.ubuntu.com",
	Host:     "https://cloud-images.ubuntu.com/releases",
	Protocol: SimplestreamsProtocol,
}

/* The "daily" stream. This consists of images that are built from the daily
 * package builds. These images have not been independently tested, but in
 * theory "should" be good, since they're build from packages from the released
 * archive.
 */
var CloudImagesDailyRemote = RemoteServer{
	Name:     "cloud-images.ubuntu.com",
	Host:     "https://cloud-images.ubuntu.com/daily",
	Protocol: SimplestreamsProtocol,
}

var generateCertificate = func() ([]byte, []byte, error) { return lxdshared.GenerateMemCert(true) }
var DefaultImageSources = []RemoteServer{CloudImagesRemote, CloudImagesDailyRemote}

type Protocol string

const (
	LXDProtocol           Protocol = "lxd"
	SimplestreamsProtocol Protocol = "simplestreams"
)

type RemoteServer struct {
	Name     string
	Host     string
	Protocol Protocol
	lxd.ConnectionArgs
}

var ConnectLocal = connectLocal

func connectLocal() (lxd.ContainerServer, error) {
	client, err := lxd.ConnectLXDUnix(lxdSocketPath(), &lxd.ConnectionArgs{})

	if err != nil {
		return nil, errors.Trace(err)
	}

	return client, nil
}

// NewContainerManager creates the entity that knows how to create and manage
// LXD containers.
// TODO(jam): This needs to grow support for things like LXC's ImageURLGetter
// functionality.
func NewContainerManager(conf container.ManagerConfig) (container.Manager, error) {
	modelUUID := conf.PopValue(container.ConfigModelUUID)
	if modelUUID == "" {
		return nil, errors.Errorf("model UUID is required")
	}
	namespace, err := instance.NewNamespace(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	availabilityZone := conf.PopValue(container.ConfigAvailabilityZone)
	if availabilityZone == "" {
		logger.Infof("Availability zone will be empty for this container manager")
	}

	conf.WarnAboutUnused()
	return &containerManager{
		modelUUID:        modelUUID,
		namespace:        namespace,
		availabilityZone: availabilityZone,
	}, nil
}

// Namespace implements container.Manager.
func (manager *containerManager) Namespace() instance.Namespace {
	return manager.namespace
}

// getImageWithServer returns an ImageServer and Image that has the image
// for series and architecture that we're looking for. If the server
// is remote the image will be cached by LXD, we don't need to cache
// it.
func (manager *containerManager) getImageWithServer(
	series, arch string,
	sources []RemoteServer) (lxd.ImageServer, *api.Image, string, error) {
	// First we check if we have the image locally.
	lastErr := fmt.Errorf("Image not found")
	imageName := seriesLocalAlias(series, arch)
	var target string
	entry, _, err := manager.server.GetImageAlias(imageName)
	if entry != nil {
		// We already have an image with the given alias,
		// so just use that.
		target = entry.Target
		image, _, err := manager.server.GetImage(target)
		if err != nil {
			logger.Debugf("Found image locally - %q %q", image, target)
			return manager.server.(lxd.ImageServer), image, target, nil
		}
	}

	// We don't have an image locally with the juju-specific alias,
	// so look in each of the provided remote sources for any of
	// the expected aliases. We don't need to copy this image as
	// it will be cached by LXD.
	aliases, err := seriesRemoteAliases(series, arch)
	if err != nil {
		return nil, nil, "", errors.Trace(err)
	}
	for _, remote := range sources {
		source, err := manager.connectToSource(remote)
		if err != nil {
			logger.Infof("failed to connect to %q: %s", remote.Host, err)
			lastErr = err
			continue
		}
		for _, alias := range aliases {
			if result, _, err := source.GetImageAlias(alias); err == nil && result.Target != "" {
				target = result.Target
				break
			}
		}
		if target != "" {
			image, _, err := source.GetImage(target)
			if err == nil {
				logger.Debugf("Found image remotely - %q %q %q", source, image, target)
				return source, image, target, nil
			} else {
				lastErr = err
			}
		}
	}
	return nil, nil, "", lastErr
}

// DestroyContainer implements container.Manager.
func (manager *containerManager) DestroyContainer(id instance.Id) error {
	if err := manager.stopInstance(string(id)); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(manager.removeInstance(string(id)))
}

// CreateContainer implements container.Manager.
func (manager *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	callback(status.Provisioning, "Creating container", nil)
	name, err := manager.createInstance(instanceConfig, cons, series, networkConfig, storageConfig, callback)
	if err != nil {
		callback(status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		return nil, nil, errors.Trace(err)
	}

	callback(status.Provisioning, "Container created, starting", nil)
	err = manager.startInstance(name)
	if err != nil {
		if err := manager.removeInstance(name); err != nil {
			logger.Errorf("Cannot remove failed instance: %s", err)
		}
		callback(status.ProvisioningError, fmt.Sprintf("Starting container: %v", err), nil)
		return nil, nil, err
	}

	callback(status.Running, "Container started", nil)
	return &lxdInstance{name, manager.server}, &instance.HardwareCharacteristics{AvailabilityZone: &manager.availabilityZone}, nil
}

// ListContainers implements container.Manager.
func (manager *containerManager) ListContainers() (result []instance.Instance, err error) {
	result = []instance.Instance{}
	if manager.server == nil {
		manager.server, err = ConnectLocal()
		if err != nil {
			return
		}
	}
	lxdInstances, err := manager.server.GetContainers()

	if err != nil {
		return
	}

	for _, i := range lxdInstances {
		if strings.HasPrefix(i.Name, manager.namespace.Prefix()) {
			result = append(result, &lxdInstance{i.Name, manager.server})
		}
	}

	return result, nil
}

// IsInitialized implements container.Manager.
func (manager *containerManager) IsInitialized() bool {
	if manager.server != nil {
		return true
	}

	// NewClient does a roundtrip to the server to make sure it understands
	// the versions, so all we need to do is connect above and we're done.
	var err error
	manager.server, err = ConnectLocal()
	return err == nil
}

func lxdSocketPath() string {
	// LXD socket is different depending on installation method
	// We prefer upstream's preference of snap installed LXD
	debianSocket := filepath.FromSlash("/var/lib/lxd")
	snapSocket := filepath.FromSlash("/var/snap/lxd/common/lxd")
	path := os.Getenv("LXD_DIR")
	if path != "" {
		logger.Debugf("Using environment LXD_DIR as socket path: %q", path)
	} else {
		if _, err := os.Stat(snapSocket); err == nil {
			logger.Debugf("Using LXD snap socket: %q", snapSocket)
			path = snapSocket
		} else {
			logger.Debugf("LXD snap socket not found, falling back to debian socket: %q", debianSocket)
			path = debianSocket
		}
	}
	return filepath.Join(path, "unix.socket")
}

// seriesLocalAlias returns the alias to assign to images for the
// specified series. The alias is juju-specific, to support the
// user supplying a customised image (e.g. CentOS with cloud-init).
func seriesLocalAlias(series, arch string) string {
	return fmt.Sprintf("juju/%s/%s", series, arch)
}

// seriesRemoteAliases returns the aliases to look for in remotes.
func seriesRemoteAliases(series, arch string) ([]string, error) {
	seriesOS, err := jujuseries.GetOSFromSeries(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch seriesOS {
	case jujuos.Ubuntu:
		return []string{path.Join(series, arch)}, nil
	case jujuos.CentOS:
		if series == "centos7" && arch == jujuarch.AMD64 {
			return []string{"centos/7/amd64"}, nil
		}
	case jujuos.OpenSUSE:
		if series == "opensuseleap" && arch == jujuarch.AMD64 {
			return []string{"opensuse/42.2/amd64"}, nil
		}
	}
	return nil, errors.NotSupportedf("series %q", series)
}

// connectToSource connects to remote ImageServer using specified protocol.
func (manager *containerManager) connectToSource(remote RemoteServer) (lxd.ImageServer, error) {
	switch remote.Protocol {
	case LXDProtocol:
		return lxd.ConnectPublicLXD(remote.Host, &remote.ConnectionArgs)
	case SimplestreamsProtocol:
		return lxd.ConnectSimpleStreams(remote.Host, &remote.ConnectionArgs)
	}
	return nil, fmt.Errorf("Wrong protocol %s", remote.Protocol)
}

// startInstance starts previously created instance.
func (manager *containerManager) startInstance(name string) error {
	req := api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	}
	op, err := manager.server.UpdateContainerState(name, req, "")
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

// stopInstance stops instance if it's not stopped.
func (manager *containerManager) stopInstance(name string) error {
	state, etag, err := manager.server.GetContainerState(name)
	if err != nil {
		return err
	}

	if state.StatusCode == api.Stopped {
		return nil
	}

	req := api.ContainerStatePut{
		Action:  "stop",
		Timeout: -1,
	}
	op, err := manager.server.UpdateContainerState(name, req, etag)
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

// createInstance creates a stopped instance from given config. It finds the proper image, either
// locally or remotely, and then creates a container using it.
func (manager *containerManager) createInstance(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (string, error) {
	var err error
	if manager.server == nil {
		manager.server, err = ConnectLocal()
		if err != nil {
			return "", errors.Annotatef(err, "failed to connect to local LXD")
		}
	}

	imageServer, image, imageName, err := manager.getImageWithServer(
		series,
		jujuarch.HostArch(),
		DefaultImageSources,
	)
	if err != nil {
		return "", errors.Annotatef(err, "failed to ensure LXD image")
	}

	name, err := manager.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return "", errors.Trace(err)
	}

	// CloudInitUserData creates our own ENI/netplan, we need to disable cloud-init networking
	// to make it work.
	userData, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		return "", errors.Trace(err)
	}

	metadata := map[string]string{
		UserDataKey:      string(userData),
		NetworkConfigKey: containerinit.CloudInitNetworkConfigDisabled,
		// An extra piece of info to let people figure out where this
		// thing came from.
		JujuModelKey: manager.modelUUID,
		// Make sure these come back up on host reboot.
		AutoStartKey: "true",
	}

	nics, err := networkDevices(networkConfig)
	if err != nil {
		return "", errors.Trace(err)
	}

	// TODO(macgreagoir) This might be dead code. Do we always get
	// len(nics) > 0?
	profiles := []string{}

	if len(nics) == 0 {
		logger.Infof("instance %q configured with %q profile", name, lxdDefaultProfileName)
		profiles = append(profiles, lxdDefaultProfileName)
	} else {
		logger.Infof("instance %q configured with %v network devices", name, nics)
	}

	spec := api.ContainersPost{
		Name: name,
		ContainerPut: api.ContainerPut{
			Profiles: profiles,
			Devices:  nics,
			Config:   metadata,
		},
	}

	logger.Infof("starting instance %q (image %q)...", spec.Name, imageName)
	callback(status.Provisioning, "Creating container", nil)
	op, err := manager.server.CreateContainerFromImage(imageServer, *image, spec)
	if err != nil {
		logger.Errorf("CreateContainer failed with %s", err)
		return "", errors.Trace(err)
	}

	progress := func(op api.Operation) {
		if op.Metadata == nil {
			return
		}
		for _, key := range []string{"fs_progress", "download_progress"} {
			value, ok := op.Metadata[key]
			if ok {
				callback(status.Provisioning, fmt.Sprintf("Retrieving image: %s", value.(string)), nil)
				return
			}
		}
	}
	_, err = op.AddHandler(progress)
	if err != nil {
		return "", errors.Trace(err)
	}

	op.Wait()
	opInfo, err := op.GetTarget()
	if err != nil {
		return "", errors.Trace(err)
	}
	if opInfo.StatusCode != api.Success {
		return "", fmt.Errorf("LXD error: %s", opInfo.Err)
	}
	return name, nil
}

func (manager *containerManager) removeInstance(name string) error {
	op, err := manager.server.DeleteContainer(name)
	if err != nil {
		return err
	}
	err = op.Wait()
	return err
}

func nicDevice(deviceName, parentDevice, hwAddr string, mtu int) (map[string]string, error) {
	device := make(map[string]string)

	device["type"] = "nic"
	device["nictype"] = "bridged"

	if deviceName == "" {
		return nil, errors.Errorf("invalid device name")
	}
	device["name"] = deviceName

	if parentDevice == "" {
		return nil, errors.Errorf("invalid parent device name")
	}
	device["parent"] = parentDevice

	if hwAddr != "" {
		device["hwaddr"] = hwAddr
	}

	if mtu > 0 {
		device["mtu"] = fmt.Sprintf("%v", mtu)
	}

	return device, nil
}

func networkDevices(networkConfig *container.NetworkConfig) (map[string]map[string]string, error) {
	nics := make(map[string]map[string]string)

	if len(networkConfig.Interfaces) > 0 {
		for _, v := range networkConfig.Interfaces {
			if v.InterfaceType == network.LoopbackInterface {
				continue
			}
			if v.InterfaceType != network.EthernetInterface {
				return nil, errors.Errorf("interface type %q not supported", v.InterfaceType)
			}
			parentDevice := v.ParentInterfaceName
			device, err := nicDevice(v.InterfaceName, parentDevice, v.MACAddress, v.MTU)
			if err != nil {
				return nil, errors.Trace(err)
			}
			nics[v.InterfaceName] = device
		}
	} else if networkConfig.Device != "" {
		device, err := nicDevice("eth0", networkConfig.Device, "", networkConfig.MTU)
		if err != nil {
			return nil, errors.Trace(err)
		}
		nics["eth0"] = device
	}

	return nics, nil
}
