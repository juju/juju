// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/arch"
	lxdshared "github.com/lxc/lxd/shared"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/tools/lxdclient"
)

func isController(icfg *instancecfg.InstanceConfig) bool {
	return multiwatcher.AnyJobNeedsState(icfg.Jobs...)
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*environ) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance implements environs.InstanceBroker.
func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// Start a new instance.

	series := args.Tools.OneSeries()
	logger.Debugf("StartInstance: %q, %s", args.InstanceConfig.MachineId, series)

	if err := env.finishInstanceConfig(args); err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) Handle constraints?

	raw, err := env.newRawInstance(args)
	if err != nil {
		if args.StatusCallback != nil {
			args.StatusCallback(status.StatusProvisioningError, err.Error(), nil)
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

func (env *environ) finishInstanceConfig(args environs.StartInstanceParams) error {
	// TODO(natefinch): This is only correct so long as the lxd is running on
	// the local machine.  If/when we support a remote lxd environment, we'll
	// need to change this to match the arch of the remote machine.
	tools, err := args.Tools.Match(tools.Filter{Arch: arch.HostArch()})
	if err != nil {
		return errors.Trace(err)
	}
	if err := args.InstanceConfig.SetTools(tools); err != nil {
		return errors.Trace(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.ecfg.Config); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (env *environ) getImageSources() ([]lxdclient.Remote, error) {
	metadataSources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}
	remotes := make([]lxdclient.Remote, 0)
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
		if strings.HasPrefix(url, "http://") {
			url = strings.TrimPrefix(url, "http://")
			url = "https://" + url
			logger.Debugf("LXD requires https://, using: %s", url)
		}
		remotes = append(remotes, lxdclient.Remote{
			Name:          source.Description(),
			Host:          url,
			Protocol:      lxdclient.SimplestreamsProtocol,
			Cert:          nil,
			ServerPEMCert: "",
		})
	}
	return remotes, nil
}

// newRawInstance is where the new physical instance is actually
// provisioned, relative to the provided args and spec. Info for that
// low-level instance is returned.
func (env *environ) newRawInstance(args environs.StartInstanceParams) (*lxdclient.Instance, error) {
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

	series := args.InstanceConfig.Series
	// TODO(jam): We should get this information from EnsureImageExists, or
	// something given to us from 'raw', not assume it ourselves.
	image := "ubuntu-" + series
	// TODO: support args.Constraints.Arch, we'll want to map from

	// Keep track of StatusCallback output so we may clean up later.
	// This is implemented here, close to where the StatusCallback calls
	// are made, instead of at a higher level in the package, so as not to
	// assume that all providers will have the same need to be implemented
	// in the same way.
	longestMsg := 0
	statusCallback := func(currentStatus status.Status, msg string) {
		if args.StatusCallback != nil {
			args.StatusCallback(currentStatus, msg, nil)
		}
		if len(msg) > longestMsg {
			longestMsg = len(msg)
		}
	}
	cleanupCallback := func() {
		if args.CleanupCallback != nil {
			args.CleanupCallback(strings.Repeat(" ", longestMsg))
		}
	}
	defer cleanupCallback()

	imageCallback := func(copyProgress string) {
		statusCallback(status.StatusAllocating, copyProgress)
	}
	if err := env.raw.EnsureImageExists(series, imageSources, imageCallback); err != nil {
		return nil, errors.Trace(err)
	}
	cleanupCallback() // Clean out any long line of completed download status

	cloudcfg, err := cloudinit.New(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if args.InstanceConfig.Controller != nil {
		// For controller machines, generate a certificate pair and write
		// them to the instance's disk in a well-defined location, along
		// with the server's certificate.
		certPEM, keyPEM, err := lxdshared.GenerateMemCert()
		if err != nil {
			return nil, errors.Trace(err)
		}
		cert := lxdclient.NewCert(certPEM, keyPEM)
		cert.Name = hostname
		// TODO(axw) 2016-08-24 #1616346
		// We need to remove this cert when removing
		// the machine and/or destroying the controller.
		if err := env.raw.AddCert(cert); err != nil {
			return nil, errors.Annotatef(err, "adding certificate %q", cert.Name)
		}
		serverState, err := env.raw.ServerStatus()
		if err != nil {
			return nil, errors.Annotate(err, "getting server status")
		}
		cloudcfg.AddRunTextFile(clientCertPath, string(certPEM), 0600)
		cloudcfg.AddRunTextFile(clientKeyPath, string(keyPEM), 0600)
		cloudcfg.AddRunTextFile(serverCertPath, serverState.Environment.Certificate, 0600)
	}

	metadata, err := getMetadata(cloudcfg, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	//tags := []string{
	//	env.globalFirewallName(),
	//	machineID,
	//}
	// TODO(ericsnow) Use the env ID for the network name (instead of default)?
	// TODO(ericsnow) Make the network name configurable?
	// TODO(ericsnow) Support multiple networks?
	// TODO(ericsnow) Use a different net interface name? Configurable?
	instSpec := lxdclient.InstanceSpec{
		Name:  hostname,
		Image: image,
		//Type:              spec.InstanceType.Name,
		//Disks:             getDisks(spec, args.Constraints),
		//NetworkInterfaces: []string{"ExternalNAT"},
		Metadata: metadata,
		Profiles: []string{
			//TODO(wwitzel3) allow the user to specify lxc profiles to apply. This allows the
			// user to setup any custom devices order config settings for their environment.
			// Also we must ensure that a device with the parent: lxcbr0 exists in at least
			// one of the profiles.
			"default",
			env.profileName(),
		},
		//Tags:              tags,
		// Network is omitted (left empty).
	}

	logger.Infof("starting instance %q (image %q)...", instSpec.Name, instSpec.Image)

	statusCallback(status.StatusAllocating, "preparing image")
	inst, err := env.raw.AddInstance(instSpec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	statusCallback(status.StatusRunning, "container started")
	return inst, nil
}

// getMetadata builds the raw "user-defined" metadata for the new
// instance (relative to the provided args) and returns it.
func getMetadata(cloudcfg cloudinit.CloudConfig, args environs.StartInstanceParams) (map[string]string, error) {
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
	userdata := string(uncompressed)

	metadata := map[string]string{
		// store the cloud-config userdata for cloud-init.
		metadataKeyCloudInit: userdata,
	}
	for k, v := range args.InstanceConfig.Tags {
		if !strings.HasPrefix(k, tags.JujuTagPrefix) {
			// Since some metadata is interpreted by LXD,
			// we cannot allow arbitrary tags to be passed
			// in by the user. We currently only pass through
			// Juju-defined tags.
			//
			// TODO(axw) 2016-04-11 #1568666
			// We should reject non-juju tags in config validation.
			logger.Debugf("ignoring non-juju tag: %s=%s", k, v)
			continue
		}
		metadata[k] = v
	}

	return metadata, nil
}

// getHardwareCharacteristics compiles hardware-related details about
// the given instance and relative to the provided spec and returns it.
func (env *environ) getHardwareCharacteristics(args environs.StartInstanceParams, inst *environInstance) *instance.HardwareCharacteristics {
	raw := inst.raw.Hardware

	archStr := raw.Architecture
	if archStr == "unknown" || !arch.IsSupportedArch(archStr) {
		// TODO(ericsnow) This special-case should be improved.
		archStr = arch.HostArch()
	}

	hwc, err := instance.ParseHardware(
		"arch="+archStr,
		fmt.Sprintf("cpu-cores=%d", raw.NumCores),
		fmt.Sprintf("mem=%dM", raw.MemoryMB),
		//"root-disk=",
		//"tags=",
	)
	if err != nil {
		logger.Errorf("unexpected problem parsing hardware info: %v", err)
		// Keep moving...
	}
	return &hwc
}

// AllInstances implements environs.InstanceBroker.
func (env *environ) AllInstances() ([]instance.Instance, error) {
	environInstances, err := env.allInstances()
	instances := make([]instance.Instance, len(environInstances))
	for i, inst := range environInstances {
		if inst == nil {
			continue
		}
		instances[i] = inst
	}
	return instances, err
}

// StopInstances implements environs.InstanceBroker.
func (env *environ) StopInstances(instances ...instance.Id) error {
	var ids []string
	for _, id := range instances {
		ids = append(ids, string(id))
	}

	prefix := env.namespace.Prefix()
	err := env.raw.RemoveInstances(prefix, ids...)
	return errors.Trace(err)
}
