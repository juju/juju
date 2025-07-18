// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"

	jujuarch "github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/containerinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/network"
)

var (
	logger = internallogger.GetLogger("juju.container.lxd")
)

const lxdDefaultProfileName = "default"

type containerManager struct {
	newServer func() (*Server, error)
	server    *Server

	modelUUID        string
	namespace        instance.Namespace
	availabilityZone string

	imageMetadataURL              string
	imageStream                   string
	imageMetadataDefaultsDisabled bool
	imageMutex                    sync.Mutex

	serverInitMutex sync.Mutex
	profileMutex    sync.Mutex
}

// containerManager implements container.Manager.
var _ container.Manager = (*containerManager)(nil)

// NewContainerManager creates the entity that knows how to create and manage
// LXD containers.
func NewContainerManager(cfg container.ManagerConfig, newServer func() (*Server, error)) (container.Manager, error) {
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
		logger.Infof(context.TODO(), "Availability zone will be empty for this container manager")
	}

	imageMetaDataURL := cfg.PopValue(config.ContainerImageMetadataURLKey)
	imageStream := cfg.PopValue(config.ContainerImageStreamKey)
	imageMetadataDefaultsDisabled := false
	if cfg.PopValue(config.ContainerImageMetadataDefaultsDisabledKey) == "true" {
		imageMetadataDefaultsDisabled = true
	}

	// This value is also popped by the provisioner worker; the following
	// dummy pop operation ensures that we don't get a spurious warning
	// for it when calling WarnAboutUnused() below.
	_ = cfg.PopValue(config.LXDSnapChannel)

	cfg.WarnAboutUnused()
	return &containerManager{
		newServer:                     newServer,
		modelUUID:                     modelUUID,
		namespace:                     namespace,
		availabilityZone:              availabilityZone,
		imageMetadataURL:              imageMetaDataURL,
		imageStream:                   imageStream,
		imageMetadataDefaultsDisabled: imageMetadataDefaultsDisabled,
	}, nil
}

// Namespace implements container.Manager.
func (m *containerManager) Namespace() instance.Namespace {
	return m.namespace
}

// DestroyContainer implements container.Manager.
func (m *containerManager) DestroyContainer(id instance.Id) error {
	if err := m.ensureInitialized(); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(m.server.RemoveContainer(string(id)))
}

// CreateContainer implements container.Manager.
func (m *containerManager) CreateContainer(
	ctx context.Context,
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	base corebase.Base,
	networkConfig *container.NetworkConfig,
	_ *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (instances.Instance, *instance.HardwareCharacteristics, error) {
	if err := m.ensureInitialized(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	_ = callback(ctx, status.Provisioning, "Creating container spec", nil)
	spec, err := m.getContainerSpec(ctx, instanceConfig, cons, base, networkConfig, callback)
	if err != nil {
		_ = callback(ctx, status.ProvisioningError, fmt.Sprintf("Creating container spec: %v", err), nil)
		return nil, nil, errors.Trace(err)
	}

	_ = callback(ctx, status.Provisioning, "Creating container", nil)
	c, err := m.server.CreateContainerFromSpec(spec)
	if err != nil {
		_ = callback(ctx, status.ProvisioningError, fmt.Sprintf("Creating container: %v", err), nil)
		return nil, nil, errors.Trace(err)
	}
	_ = callback(ctx, status.Running, "Container started", nil)

	virtType := string(spec.VirtType)
	arch := c.Arch()
	hardware := &instance.HardwareCharacteristics{
		Arch:     &arch,
		VirtType: &virtType,
	}
	if m.availabilityZone != "" {
		hardware.AvailabilityZone = &m.availabilityZone
	}

	return &lxdInstance{
		id:     c.Name,
		server: m.server.InstanceServer,
	}, hardware, nil
}

// ListContainers implements container.Manager.
func (m *containerManager) ListContainers() ([]instances.Instance, error) {
	if err := m.ensureInitialized(); err != nil {
		return nil, errors.Trace(err)
	}

	containers, err := m.server.FilterContainers(m.namespace.Prefix())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []instances.Instance
	for _, i := range containers {
		result = append(result, &lxdInstance{i.Name, m.server.InstanceServer})
	}
	return result, nil
}

// ensureInitialized checks to see if we have already established
// the local LXD server. This is done lazily so that LXD daemons are
// not active until we need to provision containers.
// NOTE: It must be called at the top of public methods that connect
// to the server.
func (m *containerManager) ensureInitialized() error {
	m.serverInitMutex.Lock()
	defer m.serverInitMutex.Unlock()

	if m.server != nil {
		return nil
	}

	var err error
	m.server, err = m.newServer()
	return errors.Annotate(err, "initializing local LXD server")
}

// IsInitialized implements container.Manager.
// It returns true if we can find a LXD socket on this host.
func (m *containerManager) IsInitialized() bool {
	return SocketPath(IsUnixSocket) != ""
}

// getContainerSpec generates a spec for creating a new container.
// It sources an image based on the input series, and transforms the input
// config objects into LXD configuration, including cloud init user data.
func (m *containerManager) getContainerSpec(
	ctx context.Context,
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	base corebase.Base,
	networkConfig *container.NetworkConfig,
	callback environs.StatusCallbackFunc,
) (ContainerSpec, error) {
	imageSources, err := m.getImageSources()
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
	}

	virtType := instance.InstanceTypeContainer
	if cons.HasVirtType() {
		v, err := instance.ParseVirtType(*cons.VirtType)
		if err != nil {
			return ContainerSpec{}, errors.Trace(err)
		}
		virtType = v
	}

	// Lock around finding an image.
	// The provisioner works concurrently to create containers.
	// If an image needs to be copied from a remote, we don't want many
	// goroutines attempting to do it at once.
	m.imageMutex.Lock()
	found, err := m.server.FindImage(ctx, base, jujuarch.HostArch(), virtType, imageSources, true, callback)
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

	logger.Debugf(ctx, "configuring container %q with network devices: %v", name, nics)

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
				logger.Infof(ctx, `added "auto" IPv4 configuration to default LXD bridge`)
			}
		} else {
			logger.Warningf(ctx, "no CIDR was detected for the following networks: %v", unknown)
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

	// We tell Netplan to match by interface name for containers; by MAC for VMs.
	cloudConfig, err := cloudinit.New(
		instanceConfig.Base.OS, cloudinit.WithNetplanMACMatch(virtType == instance.InstanceTypeVM))
	if err != nil {
		return ContainerSpec{}, errors.Trace(err)
	}

	// CloudInitUserData creates our own ENI/netplan.
	// We need to disable cloud-init networking to make it work.
	userData, err := containerinit.CloudInitUserData(cloudConfig, instanceConfig, networkConfig)
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

	serverSpecs := []ServerSpec{}
	if !m.imageMetadataDefaultsDisabled {
		if m.imageStream != "daily" {
			// Unless the configuration explicitly requests the daily stream,
			// we always prefer the default source.
			serverSpecs = append(serverSpecs, CloudImagesRemote)
		}
		serverSpecs = append(serverSpecs, CloudImagesDailyRemote, CloudImagesLinuxContainersRemote)
	}

	if imURL == "" {
		return serverSpecs, nil
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

	serverSpecs = append([]ServerSpec{remote}, serverSpecs...)
	return serverSpecs, nil
}

// networkDevicesFromConfig uses the input container network configuration to
// create a map of network device configuration in the LXD format.
// If there are no interfaces in the input config, but there is a bridge device
// name, return a single "eth0" device with the bridge as its parent.
// The last fall-back is to return the NIC devices from the default profile.
// Names for any networks without a known CIDR are returned in a slice.
// The host interface will be assigned a random unique value by LXD
// (https://bugs.launchpad.net/juju/+bug/1932180/comments/5).
func (m *containerManager) networkDevicesFromConfig(netConfig *container.NetworkConfig) (map[string]device, []string, error) {
	if len(netConfig.Interfaces) > 0 {
		return DevicesFromInterfaceInfo(netConfig.Interfaces)
	}

	// NOTE(achilleasa): the lxd default profile can be edited by the
	// operator to override the host_name setting. To this end, we should
	// avoid patching the host_name ourselves.
	nics, err := m.server.GetNICsFromProfile(lxdDefaultProfileName)
	return nics, nil, errors.Trace(err)
}

// SupportsLXDProfiles indicates whether this provider can interact
// with LXD Profiles.
func (*containerManager) SupportsLXDProfiles() bool {
	return true
}

// MaybeWriteLXDProfile implements container.LXDProfileManager.
// TODO: HML 2-apr-2019
// When provisioner_task processProfileChanges() is removed,
// maybe change to take an lxdprofile.ProfilePost as an arg.
func (m *containerManager) MaybeWriteLXDProfile(pName string, put lxdprofile.Profile) error {
	if err := m.ensureInitialized(); err != nil {
		return errors.Trace(err)
	}

	m.profileMutex.Lock()
	defer m.profileMutex.Unlock()
	hasProfile, err := m.server.HasProfile(pName)
	if err != nil {
		return errors.Trace(err)
	}
	if hasProfile {
		logger.Debugf(context.TODO(), "lxd profile %q already exists, not written again", pName)
		return nil
	}
	logger.Debugf(context.TODO(), "attempting to write lxd profile %q %+v", pName, put)
	post := api.ProfilesPost{
		Name: pName,
		ProfilePut: api.ProfilePut{
			Description: put.Description,
			Config:      put.Config,
			Devices:     put.Devices,
		},
	}
	if err = m.server.CreateProfile(post); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf(context.TODO(), "wrote lxd profile %q", pName)
	if err := m.verifyProfile(pName); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// verifyProfile gets the actual profile from lxd for the name provided
// and logs the result. For informational purposes only. Returns an error
// if the call to GetProfile fails.
func (m *containerManager) verifyProfile(pName string) error {
	// As there are configs where we do not have the option of looking at
	// the profile on the machine to verify, verify here that what we thought
	// was written, is what was written.
	profile, _, err := m.server.GetProfile(pName)
	if err != nil {
		return err
	}
	logger.Debugf(context.TODO(), "lxd profile %q: received %+v ", pName, profile)
	return nil
}

// LXDProfileNames implements container.LXDProfileManager
func (m *containerManager) LXDProfileNames(containerName string) ([]string, error) {
	if err := m.ensureInitialized(); err != nil {
		return nil, errors.Trace(err)
	}

	return m.server.GetContainerProfiles(containerName)
}

// AssignLXDProfiles implements environs.LXDProfiler.
func (m *containerManager) AssignLXDProfiles(
	instID string, profilesNames []string, profilePosts []lxdprofile.ProfilePost,
) (current []string, err error) {
	if err := m.ensureInitialized(); err != nil {
		return nil, errors.Trace(err)
	}

	report := func(err error) ([]string, error) {
		// Always return the current profiles assigned to the instance.
		currentProfiles, err2 := m.LXDProfileNames(instID)
		if err != nil && err2 != nil {
			logger.Errorf(context.TODO(), "secondary error, retrieving profile names: %s", err2)
		}
		return currentProfiles, err
	}

	// Write any new profilePosts and gather a slice of profile
	// names to be deleted, after removal.
	var deleteProfiles []string
	for _, p := range profilePosts {
		if p.Profile != nil {
			if err := m.MaybeWriteLXDProfile(p.Name, *p.Profile); err != nil {
				return report(err)
			}
		} else {
			deleteProfiles = append(deleteProfiles, p.Name)
		}
	}

	if err := m.server.UpdateContainerProfiles(instID, profilesNames); err != nil {
		return report(errors.Trace(err))
	}

	for _, name := range deleteProfiles {
		if err := m.server.DeleteProfile(name); err != nil {
			// Most likely the failure is because the profile is already in use.
			logger.Debugf(context.TODO(), "failed to delete profile %q: %s", name, err)
		}
	}
	return report(nil)
}
