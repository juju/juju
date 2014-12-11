// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
)

const (
	driverScopes = "https://www.googleapis.com/auth/compute " +
		"https://www.googleapis.com/auth/devstorage.full_control"

	tokenURL = "https://accounts.google.com/o/oauth2/token"

	authURL = "https://accounts.google.com/o/oauth2/auth"

	storageScratch    = "SCRATCH"
	storagePersistent = "PERSISTENT"

	statusUp = "UP"
)

var signedImageDataOnly = false

// This file contains the core of the gce Environ implementation. You will
// probably not need to change this file very much to begin with; and if you
// never need to add any more fields, you may never need to touch it.
//
// The rest of the implementation is split into environ_instance.go (which
// must be implemented ) and environ_firewall.go (which can be safely
// ignored until you've got an environment bootstrapping successfully).

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	lock    sync.Mutex
	ecfg    *environConfig
	storage storage.Storage

	gce       *compute.Service
	region    string
	projectID string
}

//TODO (wwitzel3): Investigate simplestreams.HasRegion for this provider
var _ environs.Environ = (*environ)(nil)

func (env *environ) Name() string {
	return env.name
}

func (*environ) Provider() environs.EnvironProvider {
	return providerInstance
}

var newToken = func(ecfg *environConfig, scopes string) (*oauth.Token, error) {
	jtok := jwt.NewToken(ecfg.clientEmail(), scopes, []byte(ecfg.privateKey()))
	jtok.ClaimSet.Aud = tokenURL

	token, err := jtok.Assert(&http.Client{})
	return token, errors.Trace(err)
}

var newService = func(transport *oauth.Transport) (*compute.Service, error) {
	return compute.New(transport.Client())
}

func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	var oldCfg *config.Config
	if env.ecfg != nil {
		oldCfg = env.ecfg.Config
	}
	ecfg, err := validateConfig(cfg, oldCfg)
	if err != nil {
		return err
	}
	storage, err := newStorage(ecfg)
	if err != nil {
		return err
	}
	env.ecfg = ecfg
	env.storage = storage

	token, err := newToken(ecfg, driverScopes)
	if err != nil {
		return errors.Annotate(err, "can't retrieve auth token")
	}

	transport := &oauth.Transport{
		Config: &oauth.Config{
			ClientId: ecfg.clientID(),
			Scope:    driverScopes,
			TokenURL: tokenURL,
			AuthURL:  authURL,
		},
		Token: token,
	}

	service, err := compute.New(transport.Client())
	if err != nil {
		return err
	}

	env.gce = service
	return nil
}

func (env *environ) getSnapshot() *environ {
	env.lock.Lock()
	clone := *env
	env.lock.Unlock()
	clone.lock = sync.Mutex{}
	return &clone
}

func (env *environ) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

func (env *environ) Storage() storage.Storage {
	return env.getSnapshot().storage
}

func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	// You can probably ignore this method; the common implementation should work.
	return common.Bootstrap(ctx, env, params)
}

func (env *environ) Destroy() error {
	// You can probably ignore this method; the common implementation should work.
	return common.Destroy(env)
}

func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	return nil, errNotImplemented
}

func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return errNotImplemented
}

// firewall stuff

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	return errNotImplemented
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	return errNotImplemented
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	return nil, errNotImplemented
}

// instance stuff

func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// Please note that in order to fulfil the demands made of Instances and
	// AllInstances, it is imperative that some environment feature be used to
	// keep track of which instances were actually started by juju.
	env = env.getSnapshot()

	// Start a new raw instance.

	if args.MachineConfig.HasNetworks() {
		return nil, errors.New("starting instances with networks is not supported yet")
	}

	spec, err := env.finishMachineConfig(args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	raw, err := env.newRawInstance(args, spec)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := &environInstance{
		id:   instance.Id(raw.Name),
		env:  env,
		zone: raw.Zone,
	}
	inst.update(env, raw)
	logger.Infof("started instance %q in %q", inst.Id(), raw.Zone)

	// Handle the new instance.

	env.handleStateMachine(args, raw)

	// Build the result.

	hwc := env.getHardwareCharacteristics(spec, raw)

	result := environs.StartInstanceResult{
		Instance: inst,
		Hardware: hwc,
	}
	return &result, nil
}

func (env *environ) parseAvailabilityZones(args environs.StartInstanceParams) ([]string, error) {
	if args.Placement != "" {
		// args.Placement will always be a zone name or empty.
		gceZone, err := env.parsePlacement(args.Placement)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !gceZone.Available() {
			return nil, errors.Errorf("availability zone %q is %s", gceZone.Name(), gceZone.zone.Status)
		}
		return []string{gceZone.Name()}, nil
	}

	// If no availability zone is specified, then automatically spread across
	// the known zones for optimal spread across the instance distribution
	// group.
	var group []instance.Id
	var err error
	if args.DistributionGroup != nil {
		group, err = args.DistributionGroup()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	zoneInstances, err := availabilityZoneAllocations(env, group)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var zoneNames []string
	for _, z := range zoneInstances {
		zoneNames = append(zoneNames, z.ZoneName)
	}
	if len(zoneNames) == 0 {
		return nil, errors.New("failed to determine availability zones")
	}

	return zoneNames, nil
}

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

func (env *environ) parsePlacement(placement string) (*gceAvailabilityZone, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, errors.Errorf("unknown placement directive: %v", placement)
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		zoneName := value
		zones, err := env.AvailabilityZones()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, z := range zones {
			if z.Name() == zoneName {
				return z.(*gceAvailabilityZone), nil
			}
		}
		return nil, errors.Errorf("invalid availability zone %q", zoneName)
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

type gceAvailabilityZone struct {
	zone *compute.Zone
}

func (z *gceAvailabilityZone) Name() string {
	return z.zone.Name
}

func (z *gceAvailabilityZone) Available() bool {
	// https://cloud.google.com/compute/docs/reference/latest/zones#status
	return z.zone.Status == statusUp
}

func (env *environ) finishMachineConfig(args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	// (TODO) Finish this.

	arches := args.Tools.Arches()
	series := args.Tools.OneSeries()
	spec, err := env.findInstanceSpec(env.Config().ImageStream(), &instances.InstanceConstraint{
		Region:      env.region,
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
		// TODO(ericsnow) What should go here?
		Storage: []string{storageScratch, storagePersistent},
	})
	if err != nil {
		return nil, err
	}

	envTools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, errors.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	args.MachineConfig.Tools = envTools[0]
	err = environs.FinishMachineConfig(args.MachineConfig, env.Config())
	return spec, errors.Trace(err)
}

func (env *environ) findInstanceSpec(stream string, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	sources, err := environs.ImageMetadataSources(env)
	if err != nil {
		return nil, errors.Trace(err)
	}

	regionURL := env.getRegionURL()
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{ic.Region, regionURL},
		Series:    []string{ic.Series},
		Arches:    ic.Arches,
		Stream:    stream,
	})

	matchingImages, _, err := imagemetadata.Fetch(sources, imageConstraint, signedImageDataOnly)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instanceTypes, err := env.listInstanceTypes(ic)
	if err != nil {
		return nil, errors.Trace(err)
	}

	images := instances.ImageMetadataToImages(matchingImages)
	spec, err := instances.FindInstanceSpec(images, ic, instanceTypes)
	return spec, errors.Trace(err)
}

func (env *environ) getRegionURL() string {
	// TODO(ericsnow) finish this!
	return ""
}

func (env *environ) listInstanceTypes(ic *instances.InstanceConstraint) ([]instances.InstanceType, error) {
	return nil, errNotImplemented
}

func (env *environ) getRawInstance(zone string, id string) (*compute.Instance, error) {
	call := env.gce.Instances.Get(env.projectID, zone, id)
	gInst, err := call.Do()
	return gInst, errors.Trace(err)
}

func (env *environ) newRawInstance(args environs.StartInstanceParams, spec *instances.InstanceSpec) (*compute.Instance, error) {
	userData, err := environs.ComposeUserData(args.MachineConfig, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("GCE user data; %d bytes", len(userData))

	machineID := env.machineFullName(args.MachineConfig.MachineId)
	disks := getDisks(spec, args.Constraints)
	instance := &compute.Instance{
		// TODO(ericsnow) populate/verify these values.
		Name: machineID,
		// TODO(ericsnow) The GCE instance types need to be registered.
		MachineType: spec.InstanceType.Name,
		Disks:       disks,
		// TODO(ericsnow) Do we really need this?
		Metadata: &compute.Metadata{Items: []*compute.MetadataItems{{
			Key:   "metadata.cloud-init:user-data",
			Value: string(userData),
		}}},
	}

	availabilityZones, err := env.parseAvailabilityZones(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, availZone := range availabilityZones {
		call := env.gce.Instances.Insert(
			env.projectID,
			availZone,
			instance,
		)
		operation, err := call.Do()
		if err != nil {
			// XXX Handle zone-is-full error.
			return nil, errors.Annotate(err, "sending new instance request")
		}
		if err := env.waitOperation(operation); err != nil {
			// TODO(ericsnow) Handle zone-is-full error here?
			return nil, errors.Annotate(err, "waiting for new instance operation to finish")
		}

		// Get the instance here.
		// TODO(ericsnow) Do we really need to get it?
		instance, err = env.getRawInstance(availZone, machineID)
		return instance, errors.Trace(err)
	}
	return nil, errors.Errorf("not able to provision in any zone")
}

func (env *environ) machineFullName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", env.Config().Name(), names.NewMachineTag(machineId))
}

// minDiskSize is the minimum/default size (in megabytes) for GCE root disks.
// TODO(ericsnow) Is there a minimum? What is the default?
const minDiskSize uint64 = 0

func getDisks(spec *instances.InstanceSpec, cons constraints.Value) []*compute.AttachedDisk {
	rootDiskSize := minDiskSize
	if cons.RootDisk != nil {
		if *cons.RootDisk >= minDiskSize {
			rootDiskSize = *cons.RootDisk
		} else {
			logger.Infof(
				"Ignoring root-disk constraint of %dM because it is smaller than the GCE image size of %dM",
				*cons.RootDisk,
				minDiskSize,
			)
		}
	}

	// TODO(ericsnow) what happens if there is not attached disk?
	disk := compute.AttachedDisk{
		// TODO(ericsnow) Set other fields too?
		Type: "SCRATCH",    // Could be "PERSISTENT".
		Boot: true,         // not needed?
		Mode: "READ_WRITE", // not needed?
		InitializeParams: &compute.AttachedDiskInitializeParams{
			// DiskName (defaults to instance name)
			DiskSizeGb: int64(roundVolumeSize(rootDiskSize)),
			// DiskType (???)
			SourceImage: spec.Image.Id, // correct?
		},
		// Interface (???)
		// DeviceName (persistent disk only)
		// Source (persistent disk only)
	}

	return []*compute.AttachedDisk{&disk}
}

// AWS expects GiB, we work in MiB; round up to nearest G.
// TODO(ericsnow) Move this to providers.common (also for ec2).
func roundVolumeSize(m uint64) uint64 {
	return (m + 1023) / 1024
}

func (env *environ) handleStateMachine(args environs.StartInstanceParams, raw *compute.Instance) {
	if multiwatcher.AnyJobNeedsState(args.MachineConfig.Jobs...) {
		if err := common.AddStateInstance(env.Storage(), instance.Id(raw.Name)); err != nil {
			logger.Errorf("could not record instance in provider-state: %v", err)
		}
	}
}

func (env *environ) getHardwareCharacteristics(spec *instances.InstanceSpec, raw *compute.Instance) *instance.HardwareCharacteristics {
	rawSize := raw.Disks[0].InitializeParams.DiskSizeGb
	rootDiskSize := uint64(rawSize) * 1024
	hwc := instance.HardwareCharacteristics{
		Arch:     &spec.Image.Arch,
		Mem:      &spec.InstanceType.Mem,
		CpuCores: &spec.InstanceType.CpuCores,
		CpuPower: spec.InstanceType.CpuPower,
		RootDisk: &rootDiskSize,
		// TODO(ericsnow) Add Tags here?
		// Tags *compute.Tags
		AvailabilityZone: &raw.Zone,
	}
	return &hwc
}

func (env *environ) AllInstances() ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	// We're okay here as long as env.ProjectID is exclusive to this juju
	// environment.
	e := env.getSnapshot()

	results, err := e.gce.Instances.AggregatedList(env.projectID).Do()
	if err != nil {
		return nil, err
	}

	ids := []instance.Id{}
	for _, item := range results.Items {
		for _, inst := range item.Instances {
			ids = append(ids, instance.Id(inst.Name))
		}
	}
	return env.Instances(ids)
}

func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	// This advice applies even if an instance id passed in corresponds to a
	// real instance that's not part of the environment -- the Environ should
	// treat that no differently to a request for one that does not exist.
	_ = env.getSnapshot()
	return nil, errNotImplemented
}

func (env *environ) StopInstances(instances ...instance.Id) error {
	_ = env.getSnapshot()
	return errNotImplemented
}

func (env *environ) StateServerInstances() ([]instance.Id, error) {
	return nil, errNotImplemented
}

func (env *environ) SupportedArchitectures() ([]string, error) {
	return arch.AllSupportedArches, nil
}

// Networks

// SupportAddressAllocation takes a network.Id and returns a bool
// and an error. The bool indicates whether that network supports
// static ip address allocation.
func (env *environ) SupportAddressAllocation(netId network.Id) (bool, error) {
	return false, nil
}

// AllocateAddress requests a specific address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errNotImplemented
}

func (env *environ) ReleaseAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return errNotImplemented
}

func (env *environ) Subnets(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, errNotImplemented
}

func (env *environ) ListNetworks(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, errNotImplemented
}

// SupportNetworks returns whether the environment has support to
// specify networks for services and machines.
func (env *environ) SupportNetworks() bool {
	return false
}

// SupportsUnitAssignment returns an error which, if non-nil, indicates
// that the environment does not support unit placement. If the environment
// does not support unit placement, then machines may not be created
// without units, and units cannot be placed explcitly.
func (env *environ) SupportsUnitPlacement() error {
	return errNotImplemented
}

// AvailabilityZones returns all availability zones in the environment.
func (env *environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	return nil, errNotImplemented
}

// InstanceAvailabilityZoneNames returns the names of the availability
// zones for the specified instances. The error returned follows the same
// rules as Environ.Instances.
func (env *environ) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	return nil, errNotImplemented
}

var (
	globalOperationTimeout = 60 * time.Second
	globalOperationDelay   = 10 * time.Second
)

func (env *environ) waitOperation(operation *compute.Operation) error {
	env = env.getSnapshot()

	attempts := utils.AttemptStrategy{
		Total: globalOperationTimeout,
		Delay: globalOperationDelay,
	}
	for a := attempts.Start(); a.Next(); {
		var err error
		if operation.Status == "DONE" {
			return nil
		}
		// TODO(ericsnow) should projectID come from inst?
		call := env.gce.GlobalOperations.Get(env.projectID, operation.ClientOperationId)
		operation, err = call.Do()
		if err != nil {
			return errors.Annotate(err, "while waiting for operation to complete")
		}
	}
	return errors.Errorf("timed out after %d seconds waiting for GCE operation to finish", globalOperationTimeout)
}
