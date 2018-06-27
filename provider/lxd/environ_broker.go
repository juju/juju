// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/arch"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
)

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(
	ctx context.ProviderCallContext, args environs.StartInstanceParams,
) (*environs.StartInstanceResult, error) {
	series := args.Tools.OneSeries()
	logger.Debugf("StartInstance: %q, %s", args.InstanceConfig.MachineId, series)

	arch, err := env.finishInstanceConfig(args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) Handle constraints?

	raw, err := env.newRawInstance(args, arch)
	if err != nil {
		if args.StatusCallback != nil {
			args.StatusCallback(status.ProvisioningError, err.Error(), nil)
		}
		return nil, errors.Trace(err)
	}
	logger.Infof("started instance %q", raw.Name)
	inst := newInstance(raw, env)

	// Build the result.
	hwc := env.getHardwareCharacteristics(args, inst)
	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwc,
	}
	return &result, nil
}

func (env *environ) finishInstanceConfig(args environs.StartInstanceParams) (string, error) {
	// TODO(natefinch): This is only correct so long as the lxd is running on
	// the local machine.  If/when we support a remote lxd environment, we'll
	// need to change this to match the arch of the remote machine.
	arch := arch.HostArch()
	tools, err := args.Tools.Match(tools.Filter{Arch: arch})
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := args.InstanceConfig.SetTools(tools); err != nil {
		return "", errors.Trace(err)
	}
	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.ecfg.Config); err != nil {
		return "", errors.Trace(err)
	}
	return arch, nil
}

func (env *environ) getImageSources() ([]lxd.RemoteServer, error) {
	metadataSources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	remotes := make([]lxd.RemoteServer, 0)
	for _, source := range metadataSources {
		url, err := source.URL("")
		if err != nil {
			logger.Debugf("failed to get the URL for metadataSource: %s", err)
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
		// https://github.com/lxc/lxd/issues/1763
		remotes = append(remotes, lxd.RemoteServer{
			Name:     source.Description(),
			Host:     lxd.EnsureHTTPS(url),
			Protocol: lxd.SimpleStreamsProtocol,
		})
	}
	return remotes, nil
}

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(
	args environs.StartInstanceParams,
	arch string,
) (*lxd.Container, error) {
	hostname, err := env.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Note: other providers have the ImageMetadata already read for them
	// and passed in as args.ImageMetadata. However, lxd provider doesn't
	// use datatype: image-ids, it uses datatype: image-download, and we
	// don't have a registered cloud/region.
	imageSources, err := env.getImageSources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO: support args.Constraints.Arch, we'll want to map from

	// Keep track of StatusCallback output so we may clean up later.
	// This is implemented here, close to where the StatusCallback calls
	// are made, instead of at a higher level in the package, so as not to
	// assume that all providers will have the same need to be implemented
	// in the same way.
	longestMsg := 0
	statusCallback := func(currentStatus status.Status, msg string, data map[string]interface{}) error {
		if args.StatusCallback != nil {
			args.StatusCallback(currentStatus, msg, nil)
		}
		if len(msg) > longestMsg {
			longestMsg = len(msg)
		}
		return nil
	}
	cleanupCallback := func() {
		if args.CleanupCallback != nil {
			args.CleanupCallback(strings.Repeat(" ", longestMsg))
		}
	}
	defer cleanupCallback()

	series := args.InstanceConfig.Series
	image, err := env.raw.FindImage(series, arch, imageSources, true, statusCallback)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cleanupCallback() // Clean out any long line of completed download status

	cloudCfg, err := cloudinit.New(series)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg, err := getContainerConfig(cloudCfg, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cSpec := lxd.ContainerSpec{
		Name:     hostname,
		Profiles: []string{"default", env.profileName()},
		Image:    image,
		Config:   cfg,
		// TODO (manadart 2018-05-30): This is where we need to set network devices from incoming config.
		Devices: nil,
	}

	statusCallback(status.Allocating, "Creating container", nil)
	container, err := env.raw.CreateContainerFromSpec(cSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	statusCallback(status.Running, "Container started", nil)
	return container, nil
}

// getContainerConfig builds the raw "user-defined" metadata for the new
// instance (relative to the provided args) and returns it.
func getContainerConfig(cloudcfg cloudinit.CloudConfig, args environs.StartInstanceParams) (map[string]string, error) {
	renderer := lxdRenderer{}
	uncompressed, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, renderer)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("LXD user data; %d bytes", len(uncompressed))

	// TODO(ericsnow) Looks like LXD does not handle gzipped userdata
	// correctly.  It likely has to do with the HTTP transport, much
	// as we have to b64encode the userdata for GCE.  Until that is
	// resolved we simply pass the plain text.
	//compressed := utils.Gzip(compressed)
	userData := string(uncompressed)
	metadata := map[string]string{
		// Store the cloud-config user data for cloud-init.
		lxd.UserDataKey: userData,
	}
	for k, v := range args.InstanceConfig.Tags {
		if !strings.HasPrefix(k, tags.JujuTagPrefix) {
			// Since some metadata is interpreted by LXD, we cannot allow
			// arbitrary tags to be passed in by the user.
			// We currently only pass through Juju-defined tags.
			logger.Debugf("ignoring non-juju tag: %s=%s", k, v)
			continue
		}
		metadata[lxd.UserNamespacePrefix+k] = v
	}

	return metadata, nil
}

// getHardwareCharacteristics compiles hardware-related details about
// the given instance and relative to the provided spec and returns it.
func (env *environ) getHardwareCharacteristics(
	args environs.StartInstanceParams, inst *environInstance,
) *instance.HardwareCharacteristics {
	container := inst.container

	archStr := container.Arch()
	if archStr == "unknown" || !arch.IsSupportedArch(archStr) {
		// TODO(ericsnow) This special-case should be improved.
		archStr = arch.HostArch()
	}
	cores := uint64(container.CPUs())
	mem := uint64(container.Mem())
	return &instance.HardwareCharacteristics{
		Arch:     &archStr,
		CpuCores: &cores,
		Mem:      &mem,
	}
}

// AllInstances implements environs.InstanceBroker.
func (env *environ) AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error) {
	environInstances, err := env.allInstances()
	instances := make([]instance.Instance, len(environInstances))
	for i, inst := range environInstances {
		if inst == nil {
			continue
		}
		instances[i] = inst
	}
	return instances, errors.Trace(err)
}

// StopInstances implements environs.InstanceBroker.
func (env *environ) StopInstances(ctx context.ProviderCallContext, instances ...instance.Id) error {
	prefix := env.namespace.Prefix()
	var names []string
	for _, id := range instances {
		name := string(id)
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		} else {
			logger.Warningf("ignoring request to stop container %q - not in namespace %q", name, prefix)
		}
	}

	return errors.Trace(env.raw.RemoveContainers(names))
}
