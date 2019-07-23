// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuarch "github.com/juju/utils/arch"
	"github.com/lxc/lxd/shared/api"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/containerinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/network"
)

var (
	logger = loggo.GetLogger("juju.container.lxd")
)

const lxdDefaultProfileName = "default"

type containerManager struct {
	server *Server

	modelUUID        string
	namespace        instance.Namespace
	availabilityZone string

	imageMetadataURL string
	imageStream      string
	imageMutex       sync.Mutex

	profileMutex sync.Mutex
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

// NewContainerManager creates the entity that knows how to create and manage
// LXD containers.
// TODO(jam): This needs to grow support for things like LXC's ImageURLGetter
// functionality.
func NewContainerManager(cfg container.ManagerConfig, svr *Server) (container.Manager, error) {
	modelUUID := cfg.PopValue(container.ConfigModelUUID)
	if modelUUID == "" {
		return nil, errors.Errorf("model UUID is required")
	}
	namespace, err := instance.NewNamespace(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	availabilityZone := cfg.PopValue(container.ConfigAvailabilityZone)
	if availabilityZone == "" {
		logger.Infof("Availability zone will be empty for this container manager")
	}

	imageMetaDataURL := cfg.PopValue(config.ContainerImageMetadataURLKey)
	imageStream := cfg.PopValue(config.ContainerImageStreamKey)

	cfg.WarnAboutUnused()
	return &containerManager{
		server:           svr,
		modelUUID:        modelUUID,
		namespace:        namespace,
		availabilityZone: availabilityZone,
		imageMetadataURL: imageMetaDataURL,
		imageStream:      imageStream,
	}, nil
}

// Namespace implements container.Manager.
func (m *containerManager) Namespace() instance.Namespace {
	return m.namespace
}

// DestroyContainer implements container.Manager.
func (m *containerManager) DestroyContainer(id instance.Id) error {
	return errors.Trace(m.server.RemoveContainer(string(id)))
}

// CreateContainer implements container.Manager.
func (m *containerManager) CreateContainer(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (instances.Instance, *instance.HardwareCharacteristics, error) {
	callback(status.Provisioning, "Creating container spec", nil)
	spec, err := m.getContainerSpec(instanceConfig, cons, series, networkConfig, storageConfig, callback)
	if err != nil {
		callback(status.ProvisioningError, fmt.Sprintf("Creating container spec: %v", err), nil)
		return nil, nil, errors.Trace(err)
	}

	callback(status.Provisioning, "Creating container", nil)
	c, err := m.server.CreateContainerFromSpec(spec)
	if err != nil {
		callback(status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		return nil, nil, errors.Trace(err)
	}
	callback(status.Running, "Container started", nil)

	return &lxdInstance{c.Name, m.server.ContainerServer},
		&instance.HardwareCharacteristics{AvailabilityZone: &m.availabilityZone}, nil
}

// ListContainers implements container.Manager.
func (m *containerManager) ListContainers() ([]instances.Instance, error) {
	containers, err := m.server.FilterContainers(m.namespace.Prefix())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []instances.Instance
	for _, i := range containers {
		result = append(result, &lxdInstance{i.Name, m.server.ContainerServer})
	}
	return result, nil
}

// IsInitialized implements container.Manager.
func (m *containerManager) IsInitialized() bool {
	return m.server != nil
}

// getContainerSpec generates a spec for creating a new container.
// It sources an image based on the input series, and transforms the input
// config objects into LXD configuration, including cloud init user data.
func (m *containerManager) getContainerSpec(
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	networkConfig *container.NetworkConfig,
	storageConfig *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (ContainerSpec, error) {
	imageSources, err := m.getImageSources()
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
	}

	// Lock around finding an image.
	// The provisioner works concurrently to create containers.
	// If an image needs to be copied from a remote, we don't want many
	// goroutines attempting to do it at once.
	m.imageMutex.Lock()
	found, err := m.server.FindImage(series, jujuarch.HostArch(), imageSources, true, callback)
	m.imageMutex.Unlock()
	if err != nil {
		return ContainerSpec{}, errors.Annotatef(err, "acquiring LXD image")
	}

	name, err := m.namespace.Hostname(instanceConfig.MachineId)
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
	}

	nics, unknown, err := m.networkDevicesFromConfig(networkConfig)
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
	}

	logger.Debugf("configuring container %q with network devices: %v", name, nics)

	// If the default LXD bridge was supplied in network config,
	// but without a CIDR, attempt to ensure it is configured for IPv4.
	// If there are others with incomplete info, log a warning.
	if len(unknown) > 0 {
		if len(unknown) == 1 && unknown[0] == network.DefaultLXDBridge && m.server.networkAPISupport {
			mod, err := m.server.EnsureIPv4(network.DefaultLXDBridge)
			if err != nil {
				return ContainerSpec{}, errors.Annotate(err, "ensuring default bridge IPv4 config")
			}
			if mod {
				logger.Infof(`added "auto" IPv4 configuration to default LXD bridge`)
			}
		} else {
			logger.Warningf("no CIDR was detected for the following networks: %v", unknown)
		}
	}

	// If there was no incoming interface info, then at this point we know
	// that nics were generated by falling back to either a single "eth0",
	// or devices from the profile.
	// Ensure that the devices are represented in the cloud-init user-data.
	if len(networkConfig.Interfaces) == 0 {
		interfaces, err := InterfaceInfoFromDevices(nics)
		if err != nil {
			return ContainerSpec{}, errors.Trace(err)
		}
		networkConfig.Interfaces = interfaces
	}

	// CloudInitUserData creates our own ENI/netplan.
	// We need to disable cloud-init networking to make it work.
	userData, err := containerinit.CloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
	}

	cfg := map[string]string{
		UserDataKey:      string(userData),
		NetworkConfigKey: cloudinit.CloudInitNetworkConfigDisabled,
		AutoStartKey:     "true",
		// Extra info to indicate the origin of this container.
		JujuModelKey: m.modelUUID,
	}

	spec := ContainerSpec{
		Name:     name,
		Image:    found,
		Config:   cfg,
		Profiles: instanceConfig.Profiles,
		Devices:  nics,
	}
	spec.ApplyConstraints(m.server.serverVersion, cons)

	return spec, nil
}

// getImageSources returns a list of LXD remote image sources based on the
// configuration that was passed into the container manager.
func (m *containerManager) getImageSources() ([]ServerSpec, error) {
	imURL := m.imageMetadataURL

	// Unless the configuration explicitly requests the daily stream,
	// an empty image metadata URL results in a search of the default sources.
	if imURL == "" && m.imageStream != "daily" {
		logger.Debugf("checking default image metadata sources")
		return []ServerSpec{CloudImagesRemote, CloudImagesDailyRemote}, nil
	}
	// Otherwise only check the daily stream.
	if imURL == "" {
		return []ServerSpec{CloudImagesDailyRemote}, nil
	}

	imURL, err := imagemetadata.ImageMetadataURL(imURL, m.imageStream)
	if err != nil {
		return nil, errors.Annotatef(err, "generating image metadata source")
	}
	imURL = EnsureHTTPS(imURL)
	remote := ServerSpec{
		Name:     strings.Replace(imURL, "https://", "", 1),
		Host:     imURL,
		Protocol: SimpleStreamsProtocol,
	}

	// If the daily stream was configured with custom image metadata URL,
	// only use the Ubuntu daily as a fallback.
	if m.imageStream == "daily" {
		return []ServerSpec{remote, CloudImagesDailyRemote}, nil
	}
	return []ServerSpec{remote, CloudImagesRemote, CloudImagesDailyRemote}, nil
}

// networkDevicesFromConfig uses the input container network configuration to
// create a map of network device configuration in the LXD format.
// If there are no interfaces in the input config, but there is a bridge device
// name, return a single "eth0" device with the bridge as its parent.
// The last fall-back is to return the NIC devices from the default profile.
// Names for any networks without a known CIDR are returned in a slice.
func (m *containerManager) networkDevicesFromConfig(netConfig *container.NetworkConfig) (map[string]device, []string, error) {
	if len(netConfig.Interfaces) > 0 {
		return DevicesFromInterfaceInfo(netConfig.Interfaces)
	} else if netConfig.Device != "" {
		return map[string]device{
			"eth0": newNICDevice("eth0", netConfig.Device, corenetwork.GenerateVirtualMACAddress(), netConfig.MTU),
		}, nil, nil
	}

	nics, err := m.server.GetNICsFromProfile(lxdDefaultProfileName)
	return nics, nil, errors.Trace(err)
}

// TODO: HML 2-apr-2019
// When provisioner_task processProfileChanges() is
// removed, maybe change to take an lxdprofile.ProfilePost as
// an arg.
// MaybeWriteLXDProfile implements container.LXDProfileManager.
func (m *containerManager) MaybeWriteLXDProfile(pName string, put *charm.LXDProfile) error {
	m.profileMutex.Lock()
	defer m.profileMutex.Unlock()
	hasProfile, err := m.server.HasProfile(pName)
	if err != nil {
		return errors.Trace(err)
	}
	if hasProfile {
		logger.Debugf("lxd profile %q already exists, not written again", pName)
		return nil
	}
	post := api.ProfilesPost{
		Name:       pName,
		ProfilePut: api.ProfilePut(*put),
	}
	if err = m.server.CreateProfile(post); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("wrote lxd profile %q", pName)
	return nil
}

// LXDProfileNames implements container.LXDProfileManager
func (m *containerManager) LXDProfileNames(containerName string) ([]string, error) {
	return m.server.GetContainerProfiles(containerName)
}

// AssignLXDProfiles implements environs.LXDProfiler.
func (m *containerManager) AssignLXDProfiles(instId string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) (current []string, err error) {
	report := func(err error) ([]string, error) {
		// Always return the current profiles assigned to the instance.
		currentProfiles, err2 := m.LXDProfileNames(instId)
		if err != nil && err2 != nil {
			logger.Errorf("secondary error, retrieving profile names: %s", err2)
		}
		return currentProfiles, err
	}

	// Write any new profilePosts and gather a slice of profile
	// names to be deleted, after removal.
	var deleteProfiles []string
	for _, p := range profilePosts {
		if p.Profile != nil {
			pr := charm.LXDProfile(*p.Profile)
			if err := m.MaybeWriteLXDProfile(p.Name, &pr); err != nil {
				return report(err)
			}
		} else {
			deleteProfiles = append(deleteProfiles, p.Name)
		}
	}

	if err := m.server.UpdateContainerProfiles(instId, profilesNames); err != nil {
		return report(errors.Trace(err))
	}

	for _, name := range deleteProfiles {
		if err := m.server.DeleteProfile(name); err != nil {
			// Most likely the failure is because the profile is already in use.
			logger.Debugf("failed to delete profile %q: %s", name, err)
		}
	}
	return report(nil)
}
