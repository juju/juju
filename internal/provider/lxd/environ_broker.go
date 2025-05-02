// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
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
	statusCallback := func(ctx context.Context, currentStatus status.Status, msg string, data map[string]interface{}) error {
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
// Cloud-init config is generated based on the network devices in the default
// profile and included in the spec config.
func (env *environ) getContainerSpec(
	ctx context.Context,
	image lxd.SourcedImage, serverVersion string, args environs.StartInstanceParams,
) (lxd.ContainerSpec, error) {
	hostname, err := env.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return lxd.ContainerSpec{}, errors.Trace(err)
	}
	cSpec := lxd.ContainerSpec{
		Name:     hostname,
		Profiles: append([]string{"default", env.profileName()}, args.CharmLXDProfiles...),
		Image:    image,
		Config:   make(map[string]string),
	}
	cSpec.ApplyConstraints(serverVersion, args.Constraints)

	cloudCfg, err := cloudinit.New(args.InstanceConfig.Base.OS)
	if err != nil {
		return cSpec, errors.Trace(err)
	}

	// Assemble the list of NICs that need to be added to the container.
	// This includes all NICs from the default profile as well as any
	// additional NICs required to satisfy any subnets that were requested
	// due to space constraints.
	//
	// If additional non-eth0 NICs are to be added, we need to ensure that
	// cloud-init correctly configures them.
	nics, err := env.assignContainerNICs(args)
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

		cSpec.Devices = nics
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
	//cfg[lxd.UserDataKey] = utils.Gzip(userData)
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

func (env *environ) assignContainerNICs(instStartParams environs.StartInstanceParams) (map[string]map[string]string, error) {
	// First, include any nics explicitly requested by the default profile.
	assignedNICs, err := env.server().GetNICsFromProfile("default")
	if err != nil {
		return nil, errors.Trace(err)
	}

	// No additional NICs required.
	if len(instStartParams.SubnetsToZones) == 0 {
		return assignedNICs, nil
	}

	if assignedNICs == nil {
		assignedNICs = make(map[string]map[string]string)
	}

	// We use two sets to de-dup the required NICs and ensure that each
	// additional NIC gets assigned a sequential ethX name.
	requestedHostBridges := set.NewStrings()
	requestedNICNames := set.NewStrings()
	for nicName, details := range assignedNICs {
		requestedNICNames.Add(nicName)
		if len(details) != 0 {
			requestedHostBridges.Add(details["parent"])
		}
	}

	// Assign any extra NICs required to satisfy the subnet requirements
	// for this instance.
	var nextIndex int
	for _, subnetList := range instStartParams.SubnetsToZones {
		for providerSubnetID := range subnetList {
			subnetID := string(providerSubnetID)

			// Sanity check: make sure we are using the correct subnet
			// naming conventions (subnet-$hostBridgeName-$CIDR).
			if !strings.HasPrefix(subnetID, "subnet-") {
				continue
			}

			// Let's be paranoid here and assume that the bridge
			// name may also contain dashes. So trim the "subnet-"
			// prefix and anything from the right-most dash to
			// recover the bridge name.
			subnetID = strings.TrimPrefix(subnetID, "subnet-")
			lastDashIndex := strings.LastIndexByte(subnetID, '-')
			if lastDashIndex == -1 {
				continue
			}
			hostBridge := subnetID[:lastDashIndex]

			// We have already requested a device on this subnet
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

			assignedNICs[devName] = map[string]string{
				"name":    devName,
				"type":    "nic",
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
	p, err := env.parsePlacement(ctx, args.Placement)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if p.nodeName == "" {
		return env.server(), nil
	}
	return env.server().UseTargetServer(p.nodeName)
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
	return &instance.HardwareCharacteristics{
		Arch:     &archStr,
		CpuCores: &cores,
		Mem:      &mem,
		VirtType: &container.Type,
	}
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
