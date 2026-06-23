// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/internal/tools"
)

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(
	ctx context.Context, args environs.StartInstanceParams,
) (*environs.StartInstanceResult, error) {
	logger.Debugf(ctx, "StartInstance: %q, %s", args.InstanceConfig.MachineId, args.InstanceConfig.Base)

	arch, virtType, err := env.finishInstanceConfig(args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	container, err := env.newContainer(ctx, args, arch, virtType)
	if err != nil {
		err = env.HandleCredentialError(ctx, err)
		if args.StatusCallback != nil {
			_ = args.StatusCallback(ctx, status.ProvisioningError, err.Error(), nil)
		}
		return nil, errors.Trace(err)
	}
	logger.Infof(ctx, "started instance %q", container.Name)
	inst := newInstance(container, env)

	// Build the result.
	hwc := env.getHardwareCharacteristics(args, inst)
	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwc,
	}
	return &result, nil
}

func (env *environ) finishInstanceConfig(args environs.StartInstanceParams) (string, instance.VirtType, error) {
	// Use the HostArch to determine the tools to use.
	arch := env.server().HostArch()
	tools, err := args.Tools.Match(tools.Filter{Arch: arch})
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if err := args.InstanceConfig.SetTools(tools); err != nil {
		return "", "", errors.Trace(err)
	}

	// Parse the virt-type from the constraints, so we can pass it to the
	// findImage function.
	virtType := instance.DefaultInstanceType
	if args.Constraints.HasVirtType() {
		if virtType, err = instance.ParseVirtType(*args.Constraints.VirtType); err != nil {
			return "", "", errors.Trace(err)
		}
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.Config()); err != nil {
		return "", "", errors.Trace(err)
	}
	return arch, virtType, nil
}

// newContainer is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newContainer(
	ctx context.Context,
	args environs.StartInstanceParams,
	arch string,
	virtType instance.VirtType,
) (*lxd.Container, error) {
	// Note: other providers have the ImageMetadata already read for them
	// and passed in as args.ImageMetadata. However, lxd provider doesn't
	// use datatype: image-ids, it uses datatype: image-download, and we
	// don't have a registered cloud/region.
	imageSources, err := env.getImageSources(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Keep track of StatusCallback output so we may clean up later.
	// This is implemented here, close to where the StatusCallback calls
	// are made, instead of at a higher level in the package, so as not to
	// assume that all providers will have the same need to be implemented
	// in the same way.
	statusCallback := func(ctx context.Context, currentStatus status.Status, msg string, data map[string]any) error {
		if args.StatusCallback != nil {
			_ = args.StatusCallback(ctx, currentStatus, msg, nil)
		}
		return nil
	}
	cleanupCallback := func() {
		if args.CleanupCallback != nil {
			_ = args.CleanupCallback()
		}
	}
	defer cleanupCallback()

	target, err := env.getTargetServer(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	image, err := target.FindImage(ctx, args.InstanceConfig.Base, arch, virtType, imageSources, true, statusCallback)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cleanupCallback() // Clean out any long line of completed download status

	cSpec, err := env.getContainerSpec(ctx, image, target.ServerVersion(), args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	_ = statusCallback(ctx, status.Allocating, "Creating container", nil)
	container, err := target.CreateContainerFromSpec(cSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	_ = statusCallback(ctx, status.Running, "Container started", nil)
	return container, nil
}

func (env *environ) getImageSources(ctx context.Context) ([]lxd.ServerSpec, error) {
	// TODO (stickupkid): Allow the passing in of the factory.
	factory := simplestreams.DefaultDataSourceFactory()
	metadataSources, err := environs.ImageMetadataSources(env, factory)
	if err != nil {
		return nil, errors.Trace(err)
	}
	remotes := make([]lxd.ServerSpec, 0)
	for _, source := range metadataSources {
		url, err := source.URL("")
		if err != nil {
			logger.Debugf(ctx, "failed to get the URL for metadataSource: %s", err)
			continue
		}
		// NOTE(jam) LXD only allows you to pass HTTPS URLs. So strip
		// off http:// and replace it with https://
		// Arguably we could give the user a direct error if
		// env.ImageMetadataURL is http instead of https, but we also
		// get http from the DefaultImageSources, which is why we
		// replace it.
		// TODO(jam) Maybe we could add a Validate step that ensures
		// image-metadata-url is an "https://" URL, so that Users get a
		// "your configuration is wrong" error, rather than silently
		// changing it and having them get confused.
		// https://github.com/canonical/lxd/issues/1763
		remotes = append(remotes, lxd.MakeSimpleStreamsServerSpec(source.Description(), url))
	}
	// Required for CentOS images.
	remotes = append(remotes, lxd.CloudImagesLinuxContainersRemote)
	return remotes, nil
}

// getContainerSpec builds a container spec from the input container image and
// start-up parameters.
// Cloud-init config is generated based on the network devices in the applied
// profiles and included in the spec config.
func (env *environ) getContainerSpec(
	ctx context.Context, image lxd.SourcedImage, serverVersion string, args environs.StartInstanceParams,
) (lxd.ContainerSpec, error) {
	hostname, err := env.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return lxd.ContainerSpec{}, errors.Trace(err)
	}
	cSpec := lxd.ContainerSpec{
		Name:     hostname,
		Profiles: env.containerProfileNames(),
		Image:    image,
		Config:   make(map[string]string),
	}
	cSpec.ApplyConstraints(serverVersion, args.Constraints)

	virtType := instance.InstanceTypeContainer
	if args.Constraints.HasVirtType() {
		v, err := instance.ParseVirtType(*args.Constraints.VirtType)
		if err != nil {
			return lxd.ContainerSpec{}, errors.Trace(err)
		}
		virtType = v
	}
	cloudCfg, err := cloudinit.New(
		args.InstanceConfig.Base.OS, cloudinit.WithNetplanMACMatch(virtType == instance.InstanceTypeVM))
	if err != nil {
		return cSpec, errors.Trace(err)
	}

	// Assemble the list of NICs that need to be added to the container.
	// This includes all NICs from the applied profiles as well as any
	// additional NICs required to satisfy any subnets that were requested
	// due to space constraints.
	//
	// If additional non-eth0 NICs are to be added, we need to ensure that
	// cloud-init correctly configures them.
	nics, err := env.assignContainerNICs(ctx, args)
	if err != nil {
		return cSpec, errors.Trace(err)
	}

	if !(len(nics) == 1 && nics["eth0"] != nil) {
		logger.Debugf(ctx, "generating custom cloud-init networking")

		cSpec.Config[lxd.NetworkConfigKey] = cloudinit.CloudInitNetworkConfigDisabled

		info, err := lxd.InterfaceInfoFromDevices(nics)
		if err != nil {
			return cSpec, errors.Trace(err)
		}
		if err := cloudCfg.AddNetworkConfig(info); err != nil {
			return cSpec, errors.Trace(err)
		}

		if cSpec.Devices == nil {
			cSpec.Devices = make(map[string]map[string]string)
		}
		maps.Copy(cSpec.Devices, nics)
	}

	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudCfg, lxdRenderer{})
	if err != nil {
		return cSpec, errors.Annotate(err, "composing user data")
	}
	logger.Debugf(ctx, "LXD user data; %d bytes", len(userData))

	// TODO(ericsnow) Looks like LXD does not handle gzipped userdata
	// correctly.  It likely has to do with the HTTP transport, much
	// as we have to b64encode the userdata for GCE.  Until that is
	// resolved we simply pass the plain text.
	// cfg[lxd.UserDataKey] = utils.Gzip(userData)
	cSpec.Config[lxd.UserDataKey] = string(userData)

	for k, v := range args.InstanceConfig.Tags {
		if !strings.HasPrefix(k, tags.JujuTagPrefix) {
			// Since some metadata is interpreted by LXD, we cannot allow
			// arbitrary tags to be passed in by the user.
			// We currently only pass through Juju-defined tags.
			logger.Debugf(ctx, "ignoring non-juju tag: %s=%s", k, v)
			continue
		}
		cSpec.Config[lxd.UserNamespacePrefix+k] = v
	}

	return cSpec, nil
}

func (env *environ) containerProfileNames() []string {
	return []string{"default", env.profileName()}
}

func (env *environ) assignContainerNICs(ctx context.Context, instStartParams environs.StartInstanceParams) (map[string]map[string]string, error) {
	// First, include any NICs explicitly requested by the applied profiles.
	// Later profiles override earlier profiles, matching LXD profile
	// precedence.
	assignedNICs := make(map[string]map[string]string)
	for _, profileName := range env.containerProfileNames() {
		profileNICs, err := env.server().GetNICsFromProfile(profileName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		maps.Copy(assignedNICs, profileNICs)
	}

	// No additional NICs required.
	if len(instStartParams.SubnetsToZones) == 0 {
		return assignedNICs, nil
	}

	// Map each requested subnet to the LXD network (host bridge) that hosts it.
	// The subnet's ProviderNetworkId identifies the bridge to attach for a
	// subnet requested by space constraints.
	requestedSubnetIDs := make([]corenetwork.Id, 0)
	for _, subnetList := range instStartParams.SubnetsToZones {
		for providerSubnetID := range subnetList {
			requestedSubnetIDs = append(requestedSubnetIDs, providerSubnetID)
		}
	}
	subnets, err := env.Subnets(ctx, requestedSubnetIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetBridges := make(map[corenetwork.Id]string, len(subnets))
	for _, subnet := range subnets {
		subnetBridges[subnet.ProviderId] = string(subnet.ProviderNetworkId)
	}

	// We use two sets to de-dup the required NICs and ensure that each
	// additional NIC gets assigned a sequential ethX name.
	requestedHostBridges := set.NewStrings()
	requestedNICNames := set.NewStrings()
	for nicName, details := range assignedNICs {
		requestedNICNames.Add(nicName)
		netName := lxd.NetworkName(details)
		requestedHostBridges.Add(netName)
	}

	// Assign any extra NICs required to satisfy the subnet requirements
	// for this instance.
	var nextIndex int
	for _, subnetList := range instStartParams.SubnetsToZones {
		for providerSubnetID := range subnetList {
			// Recover the host bridge for this subnet. Unknown subnets
			// (e.g. ones no longer reported by the provider) are skipped.
			hostBridge, ok := subnetBridges[providerSubnetID]
			if !ok || hostBridge == "" {
				continue
			}

			// A profile or generated device already attaches the container to
			// the bridge hosting this subnet. Profile NICs are included in
			// assignedNICs above, so they can satisfy space subnet requirements
			// without adding a duplicate device.
			if requestedHostBridges.Contains(hostBridge) {
				continue
			}

			// Allocate a new device entry and ensure it doesn't
			// clash with any existing ones
			var devName string
			for {
				devName = fmt.Sprintf("eth%d", nextIndex)
				if requestedNICNames.Contains(devName) {
					nextIndex++
					continue
				}
				break
			}
			hwaddr := corenetwork.GenerateVirtualMACAddress()
			assignedNICs[devName] = map[string]string{
				"name":    devName,
				"type":    "nic",
				"hwaddr":  hwaddr,
				"nictype": "bridged",
				"parent":  hostBridge,
			}

			requestedHostBridges.Add(hostBridge)
			requestedNICNames.Add(devName)
		}
	}

	return assignedNICs, nil
}

// getTargetServer checks to see if a valid zone was passed as a placement
// directive in the start-up start-up arguments. If so, a server for the
// specific node is returned.
func (env *environ) getTargetServer(
	ctx context.Context, args environs.StartInstanceParams,
) (Server, error) {
	zone, err := env.deriveAvailabilityZone(ctx, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if zone == "" || !env.server().IsClustered() {
		if zone != "" && zone != env.server().Name() {
			return nil, errors.NotFoundf("LXD server %q", zone)
		}
		return env.server(), nil

	}
	return env.server().UseTargetServer(ctx, zone)
}

type lxdPlacement struct {
	nodeName string
}

func (env *environ) parsePlacement(ctx context.Context, placement string) (*lxdPlacement, error) {
	if placement == "" {
		return &lxdPlacement{}, nil
	}

	var node string
	pos := strings.IndexRune(placement, '=')
	// Assume that a plain string is a node name.
	if pos == -1 {
		node = placement
	} else {
		if placement[:pos] != "zone" {
			return nil, fmt.Errorf("unknown placement directive: %v", placement)
		}
		node = placement[pos+1:]
	}

	if node == "" {
		return &lxdPlacement{}, nil
	}

	zones, err := env.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := zones.Validate(node); err != nil {
		return nil, errors.Trace(err)
	}

	return &lxdPlacement{nodeName: node}, nil
}

// getHardwareCharacteristics compiles hardware-related details about
// the given instance and relative to the provided spec and returns it.
func (env *environ) getHardwareCharacteristics(
	args environs.StartInstanceParams, inst *environInstance,
) *instance.HardwareCharacteristics {
	container := inst.container

	archStr := container.Arch()
	if archStr == "unknown" || !arch.IsSupportedArch(archStr) {
		archStr = env.server().HostArch()
	}
	cores := uint64(container.CPUs())
	mem := uint64(container.Mem())
	location := container.Location
	// In non-cluster mode, the container location has value as "none".
	// Use the single server name to make it human friendly.
	if location == "none" {
		location = env.server().Name()
	}

	hc := instance.HardwareCharacteristics{
		Arch:             &archStr,
		CpuCores:         &cores,
		Mem:              &mem,
		VirtType:         &container.Type,
		AvailabilityZone: &location,
	}

	if args.Constraints.HasRootDiskSource() {
		hc.RootDiskSource = args.Constraints.RootDiskSource
	}
	return &hc
}

// AllInstances implements environs.InstanceBroker.
func (env *environ) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	environInstances, err := env.allInstances()
	instances := make([]instances.Instance, len(environInstances))
	for i, inst := range environInstances {
		if inst == nil {
			continue
		}
		instances[i] = inst
	}
	return instances, errors.Trace(err)
}

// AllRunningInstances implements environs.InstanceBroker.
func (env *environ) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	// We can only get Alive containers from lxd api which means that "all" is the same as "running".
	return env.AllInstances(ctx)
}

// StopInstances implements environs.InstanceBroker.
func (env *environ) StopInstances(ctx context.Context, instances ...instance.Id) error {
	prefix := env.namespace.Prefix()
	var names []string
	for _, id := range instances {
		name := string(id)
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		} else {
			logger.Warningf(ctx, "ignoring request to stop container %q - not in namespace %q", name, prefix)
		}
	}

	err := env.server().RemoveContainers(names)
	if err != nil {
		return env.HandleCredentialError(ctx, err)
	}
	return nil
}
