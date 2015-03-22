// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/s3"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	envstorage "github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
)

const (
	none                        = "none"
	invalidParameterValue       = "InvalidParameterValue"
	privateAddressLimitExceeded = "PrivateIpAddressLimitExceeded"
)

// Use shortAttempt to poll for short-term events or for retrying API calls.
var shortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

var AssignPrivateIPAddress = assignPrivateIPAddress

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	// archMutex gates access to supportedArchitectures
	archMutex sync.Mutex
	// supportedArchitectures caches the architectures
	// for which images can be instantiated.
	supportedArchitectures []string

	// ecfgMutex protects the *Unlocked fields below.
	ecfgMutex       sync.Mutex
	ecfgUnlocked    *environConfig
	ec2Unlocked     *ec2.EC2
	s3Unlocked      *s3.S3
	storageUnlocked envstorage.Storage

	availabilityZonesMutex sync.Mutex
	availabilityZones      []common.AvailabilityZone

	// cachedDefaultVpc caches the id of the ec2 default vpc
	cachedDefaultVpc *defaultVpc
}

// Ensure EC2 provider supports environs.NetworkingEnviron.
var _ environs.NetworkingEnviron = (*environ)(nil)
var _ simplestreams.HasRegion = (*environ)(nil)
var _ state.Prechecker = (*environ)(nil)
var _ state.InstanceDistributor = (*environ)(nil)

type defaultVpc struct {
	hasDefaultVpc bool
	id            network.Id
}

func assignPrivateIPAddress(ec2Inst *ec2.EC2, netId string, addr network.Address) error {
	_, err := ec2Inst.AssignPrivateIPAddresses(netId, []string{addr.Value}, 0, false)
	return err
}

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
}

func awsClients(cfg *config.Config) (*ec2.EC2, *s3.S3, *environConfig, error) {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	auth := aws.Auth{ecfg.accessKey(), ecfg.secretKey()}
	region := aws.Regions[ecfg.region()]

	// TODO(katco-): Eventually we want to migrate to v4, but this
	// change is designed to be least impactful. We are currently only
	// trying to support the China region.
	signer := aws.SignV2
	if region == aws.CNNorth {
		signer = aws.SignV4Factory(region.Name, "ec2")
	}
	return ec2.New(auth, region, signer), s3.New(auth, region), ecfg, nil
}

func (e *environ) SetConfig(cfg *config.Config) error {
	ec2Client, s3Client, ecfg, err := awsClients(cfg)
	if err != nil {
		return err
	}
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.ecfgUnlocked = ecfg
	e.ec2Unlocked = ec2Client
	e.s3Unlocked = s3Client

	bucket, err := e.s3Unlocked.Bucket(ecfg.controlBucket())
	if err != nil {
		return err
	}

	// create new storage instances, existing instances continue
	// to reference their existing configuration.
	e.storageUnlocked = &ec2storage{bucket: bucket}
	return nil
}

func (e *environ) defaultVpc() (network.Id, bool, error) {
	if e.cachedDefaultVpc != nil {
		defaultVpc := e.cachedDefaultVpc
		return defaultVpc.id, defaultVpc.hasDefaultVpc, nil
	}
	ec2 := e.ec2()
	resp, err := ec2.AccountAttributes("default-vpc")
	if err != nil {
		return "", false, errors.Trace(err)
	}

	hasDefault := true
	defaultVpcId := ""

	if len(resp.Attributes) == 0 || len(resp.Attributes[0].Values) == 0 {
		hasDefault = false
		defaultVpcId = ""
	} else {

		defaultVpcId := resp.Attributes[0].Values[0]
		if defaultVpcId == none {
			hasDefault = false
			defaultVpcId = ""
		}
	}
	defaultVpc := &defaultVpc{
		id:            network.Id(defaultVpcId),
		hasDefaultVpc: hasDefault}
	e.cachedDefaultVpc = defaultVpc
	return defaultVpc.id, defaultVpc.hasDefaultVpc, nil
}

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) ec2() *ec2.EC2 {
	e.ecfgMutex.Lock()
	ec2 := e.ec2Unlocked
	e.ecfgMutex.Unlock()
	return ec2
}

func (e *environ) s3() *s3.S3 {
	e.ecfgMutex.Lock()
	s3 := e.s3Unlocked
	e.ecfgMutex.Unlock()
	return s3
}

func (e *environ) Name() string {
	return e.name
}

func (e *environ) Storage() envstorage.Storage {
	e.ecfgMutex.Lock()
	stor := e.storageUnlocked
	e.ecfgMutex.Unlock()
	return stor
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	return common.Bootstrap(ctx, e, args)
}

func (e *environ) StateServerInstances() ([]instance.Id, error) {
	return common.ProviderStateInstances(e, e.Storage())
}

// SupportedArchitectures is specified on the EnvironCapability interface.
func (e *environ) SupportedArchitectures() ([]string, error) {
	e.archMutex.Lock()
	defer e.archMutex.Unlock()
	if e.supportedArchitectures != nil {
		return e.supportedArchitectures, nil
	}
	// Create a filter to get all images from our region and for the correct stream.
	cloudSpec, err := e.Region()
	if err != nil {
		return nil, err
	}
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: cloudSpec,
		Stream:    e.Config().ImageStream(),
	})
	e.supportedArchitectures, err = common.SupportedArchitectures(e, imageConstraint)
	return e.supportedArchitectures, err
}

// SupportsAddressAllocation is specified on environs.Networking.
func (e *environ) SupportsAddressAllocation(_ network.Id) (bool, error) {
	_, hasDefaultVpc, err := e.defaultVpc()
	if err != nil {
		return false, errors.Trace(err)
	}
	return hasDefaultVpc, nil
}

var unsupportedConstraints = []string{
	constraints.Tags,
}

// ConstraintsValidator is defined on the Environs interface.
func (e *environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterConflicts(
		[]string{constraints.InstanceType},
		[]string{constraints.Mem, constraints.CpuCores, constraints.CpuPower})
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := e.SupportedArchitectures()
	if err != nil {
		return nil, err
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)
	instTypeNames := make([]string, len(allInstanceTypes))
	for i, itype := range allInstanceTypes {
		instTypeNames[i] = itype.Name
	}
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)
	return validator, nil
}

func archMatches(arches []string, arch *string) bool {
	if arch == nil {
		return true
	}
	for _, a := range arches {
		if a == *arch {
			return true
		}
	}
	return false
}

var ec2AvailabilityZones = (*ec2.EC2).AvailabilityZones

type ec2AvailabilityZone struct {
	ec2.AvailabilityZoneInfo
}

func (z *ec2AvailabilityZone) Name() string {
	return z.AvailabilityZoneInfo.Name
}

func (z *ec2AvailabilityZone) Available() bool {
	return z.AvailabilityZoneInfo.State == "available"
}

// AvailabilityZones returns a slice of availability zones
// for the configured region.
func (e *environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	e.availabilityZonesMutex.Lock()
	defer e.availabilityZonesMutex.Unlock()
	if e.availabilityZones == nil {
		filter := ec2.NewFilter()
		filter.Add("region-name", e.ecfg().region())
		resp, err := ec2AvailabilityZones(e.ec2(), filter)
		if err != nil {
			return nil, err
		}
		logger.Debugf("availability zones: %+v", resp)
		e.availabilityZones = make([]common.AvailabilityZone, len(resp.Zones))
		for i, z := range resp.Zones {
			e.availabilityZones[i] = &ec2AvailabilityZone{z}
		}
	}
	return e.availabilityZones, nil
}

// InstanceAvailabilityZoneNames returns the availability zone names for each
// of the specified instances.
func (e *environ) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	instances, err := e.Instances(ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}
	zones := make([]string, len(instances))
	for i, inst := range instances {
		if inst == nil {
			continue
		}
		zones[i] = inst.(*ec2Instance).AvailZone
	}
	return zones, err
}

type ec2Placement struct {
	availabilityZone ec2.AvailabilityZoneInfo
}

func (e *environ) parsePlacement(placement string) (*ec2Placement, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, fmt.Errorf("unknown placement directive: %v", placement)
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		availabilityZone := value
		zones, err := e.AvailabilityZones()
		if err != nil {
			return nil, err
		}
		for _, z := range zones {
			if z.Name() == availabilityZone {
				return &ec2Placement{
					z.(*ec2AvailabilityZone).AvailabilityZoneInfo,
				}, nil
			}
		}
		return nil, fmt.Errorf("invalid availability zone %q", availabilityZone)
	}
	return nil, fmt.Errorf("unknown placement directive: %v", placement)
}

// PrecheckInstance is defined on the state.Prechecker interface.
func (e *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" {
		if _, err := e.parsePlacement(placement); err != nil {
			return err
		}
	}
	if !cons.HasInstanceType() {
		return nil
	}
	// Constraint has an instance-type constraint so let's see if it is valid.
	for _, itype := range allInstanceTypes {
		if itype.Name != *cons.InstanceType {
			continue
		}
		if archMatches(itype.Arches, cons.Arch) {
			return nil
		}
	}
	if cons.Arch == nil {
		return fmt.Errorf("invalid AWS instance type %q specified", *cons.InstanceType)
	}
	return fmt.Errorf("invalid AWS instance type %q and arch %q specified", *cons.InstanceType, *cons.Arch)
}

// MetadataLookupParams returns parameters which are used to query simplestreams metadata.
func (e *environ) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		region = e.ecfg().region()
	}
	cloudSpec, err := e.cloudSpec(region)
	if err != nil {
		return nil, err
	}
	return &simplestreams.MetadataLookupParams{
		Series:        config.PreferredSeries(e.ecfg()),
		Region:        cloudSpec.Region,
		Endpoint:      cloudSpec.Endpoint,
		Architectures: arch.AllSupportedArches,
	}, nil
}

// Region is specified in the HasRegion interface.
func (e *environ) Region() (simplestreams.CloudSpec, error) {
	return e.cloudSpec(e.ecfg().region())
}

func (e *environ) cloudSpec(region string) (simplestreams.CloudSpec, error) {
	ec2Region, ok := allRegions[region]
	if !ok {
		return simplestreams.CloudSpec{}, fmt.Errorf("unknown region %q", region)
	}
	return simplestreams.CloudSpec{
		Region:   region,
		Endpoint: ec2Region.EC2Endpoint,
	}, nil
}

const (
	ebsStorage = "ebs"
	ssdStorage = "ssd"
)

// DistributeInstances implements the state.InstanceDistributor policy.
func (e *environ) DistributeInstances(candidates, distributionGroup []instance.Id) ([]instance.Id, error) {
	return common.DistributeInstances(e, candidates, distributionGroup)
}

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	var availabilityZones []string
	if args.Placement != "" {
		placement, err := e.parsePlacement(args.Placement)
		if err != nil {
			return nil, err
		}
		if placement.availabilityZone.State != "available" {
			return nil, errors.Errorf("availability zone %q is %s", placement.availabilityZone.Name, placement.availabilityZone.State)
		}
		availabilityZones = append(availabilityZones, placement.availabilityZone.Name)
	}

	// If no availability zone is specified, then automatically spread across
	// the known zones for optimal spread across the instance distribution
	// group.
	if len(availabilityZones) == 0 {
		var group []instance.Id
		var err error
		if args.DistributionGroup != nil {
			group, err = args.DistributionGroup()
			if err != nil {
				return nil, err
			}
		}
		zoneInstances, err := availabilityZoneAllocations(e, group)
		if err != nil {
			return nil, err
		}
		for _, z := range zoneInstances {
			availabilityZones = append(availabilityZones, z.ZoneName)
		}
		if len(availabilityZones) == 0 {
			return nil, errors.New("failed to determine availability zones")
		}
	}

	if args.MachineConfig.HasNetworks() {
		return nil, errors.New("starting instances with networks is not supported yet")
	}
	arches := args.Tools.Arches()
	sources, err := environs.ImageMetadataSources(e)
	if err != nil {
		return nil, err
	}

	series := args.Tools.OneSeries()
	spec, err := findInstanceSpec(sources, e.Config().ImageStream(), &instances.InstanceConstraint{
		Region:      e.ecfg().region(),
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
		Storage:     []string{ssdStorage, ebsStorage},
	})
	if err != nil {
		return nil, err
	}
	tools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, errors.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	args.MachineConfig.Tools = tools[0]
	if err := environs.FinishMachineConfig(args.MachineConfig, e.Config()); err != nil {
		return nil, err
	}

	userData, err := environs.ComposeUserData(args.MachineConfig, nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("ec2 user data; %d bytes", len(userData))
	cfg := e.Config()
	groups, err := e.setUpGroups(args.MachineConfig.MachineId, cfg.APIPort())
	if err != nil {
		return nil, errors.Annotate(err, "cannot set up groups")
	}
	var instResp *ec2.RunInstancesResp

	blockDeviceMappings, volumes, volumeAttachments, err := getBlockDeviceMappings(
		*spec.InstanceType.VirtType, &args,
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create block device mappings")
	}
	rootDiskSize := uint64(blockDeviceMappings[0].VolumeSize) * 1024

	for _, availZone := range availabilityZones {
		instResp, err = runInstances(e.ec2(), &ec2.RunInstances{
			AvailZone:           availZone,
			ImageId:             spec.Image.Id,
			MinCount:            1,
			MaxCount:            1,
			UserData:            userData,
			InstanceType:        spec.InstanceType.Name,
			SecurityGroups:      groups,
			BlockDeviceMappings: blockDeviceMappings,
		})
		if isZoneConstrainedError(err) {
			logger.Infof("%q is constrained, trying another availability zone", availZone)
		} else {
			break
		}
	}
	if err != nil {
		return nil, errors.Annotate(err, "cannot run instances")
	}
	if len(instResp.Instances) != 1 {
		return nil, errors.Errorf("expected 1 started instance, got %d", len(instResp.Instances))
	}

	inst := &ec2Instance{
		e:        e,
		Instance: &instResp.Instances[0],
	}
	logger.Infof("started instance %q in %q", inst.Id(), inst.Instance.AvailZone)

	// TODO(axw) tag all resources (instances and volumes), for accounting
	// and identification.

	if err := assignVolumeIds(inst, volumes, volumeAttachments); err != nil {
		if err := e.StopInstances(inst.Id()); err != nil {
			logger.Errorf("failed to stop instance: %v", err)
		}
		return nil, err
	}

	if multiwatcher.AnyJobNeedsState(args.MachineConfig.Jobs...) {
		if err := common.AddStateInstance(e.Storage(), inst.Id()); err != nil {
			logger.Errorf("could not record instance in provider-state: %v", err)
		}
	}

	hc := instance.HardwareCharacteristics{
		Arch:     &spec.Image.Arch,
		Mem:      &spec.InstanceType.Mem,
		CpuCores: &spec.InstanceType.CpuCores,
		CpuPower: spec.InstanceType.CpuPower,
		RootDisk: &rootDiskSize,
		// Tags currently not supported by EC2
		AvailabilityZone: &inst.Instance.AvailZone,
	}
	return &environs.StartInstanceResult{
		Instance:          inst,
		Hardware:          &hc,
		Volumes:           volumes,
		VolumeAttachments: volumeAttachments,
	}, nil
}

var runInstances = _runInstances

// runInstances calls ec2.RunInstances for a fixed number of attempts until
// RunInstances returns an error code that does not indicate an error that
// may be caused by eventual consistency.
func _runInstances(e *ec2.EC2, ri *ec2.RunInstances) (resp *ec2.RunInstancesResp, err error) {
	for a := shortAttempt.Start(); a.Next(); {
		resp, err = e.RunInstances(ri)
		if err == nil || ec2ErrCode(err) != "InvalidGroup.NotFound" {
			break
		}
	}
	return resp, err
}

func (e *environ) StopInstances(ids ...instance.Id) error {
	if err := e.terminateInstances(ids); err != nil {
		return errors.Trace(err)
	}
	return common.RemoveStateInstances(e.Storage(), ids...)
}

// groupInfoByName returns information on the security group
// with the given name including rules and other details.
func (e *environ) groupInfoByName(groupName string) (ec2.SecurityGroupInfo, error) {
	// Non-default VPC does not support name-based group lookups, can
	// use a filter by group name instead when support is needed.
	limitToGroups := []ec2.SecurityGroup{{Name: groupName}}
	resp, err := e.ec2().SecurityGroups(limitToGroups, nil)
	if err != nil {
		return ec2.SecurityGroupInfo{}, err
	}
	if len(resp.Groups) != 1 {
		return ec2.SecurityGroupInfo{}, fmt.Errorf("expected one security group named %q, got %v", groupName, resp.Groups)
	}
	return resp.Groups[0], nil
}

// groupByName returns the security group with the given name.
func (e *environ) groupByName(groupName string) (ec2.SecurityGroup, error) {
	groupInfo, err := e.groupInfoByName(groupName)
	return groupInfo.SecurityGroup, err
}

// addGroupFilter sets a limit an instance filter so only those machines
// with the juju environment wide security group associated will be listed.
//
// An EC2 API call is required to resolve the group name to an id, as VPC
// enabled accounts do not support name based filtering.
// TODO: Detect classic accounts and just filter by name for those.
//
// Callers must handle InvalidGroup.NotFound errors to mean the same as no
// matching instances.
func (e *environ) addGroupFilter(filter *ec2.Filter) error {
	groupName := e.jujuGroupName()
	group, err := e.groupByName(groupName)
	if err != nil {
		return err
	}
	// EC2 should support filtering with and without the 'instance.'
	// prefix, but only the form with seems to work with default VPC.
	filter.Add("instance.group-id", group.Id)
	return nil
}

// gatherInstances tries to get information on each instance
// id whose corresponding insts slot is nil.
// It returns environs.ErrPartialInstances if the insts
// slice has not been completely filled.
func (e *environ) gatherInstances(ids []instance.Id, insts []instance.Instance) error {
	var need []string
	for i, inst := range insts {
		if inst == nil {
			need = append(need, string(ids[i]))
		}
	}
	if len(need) == 0 {
		return nil
	}
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	err := e.addGroupFilter(filter)
	if err != nil {
		if ec2ErrCode(err) == "InvalidGroup.NotFound" {
			return environs.ErrPartialInstances
		}
		return err
	}
	filter.Add("instance-id", need...)
	resp, err := e.ec2().Instances(nil, filter)
	if err != nil {
		return err
	}
	n := 0
	// For each requested id, add it to the returned instances
	// if we find it in the response.
	for i, id := range ids {
		if insts[i] != nil {
			continue
		}
		for j := range resp.Reservations {
			r := &resp.Reservations[j]
			for k := range r.Instances {
				if r.Instances[k].InstanceId == string(id) {
					inst := r.Instances[k]
					// TODO(wallyworld): lookup the details to fill in the instance type data
					insts[i] = &ec2Instance{e: e, Instance: &inst}
					n++
				}
			}
		}
	}
	if n < len(ids) {
		return environs.ErrPartialInstances
	}
	return nil
}

func (e *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]instance.Instance, len(ids))
	// Make a series of requests to cope with eventual consistency.
	// Each request will attempt to add more instances to the requested
	// set.
	var err error
	for a := shortAttempt.Start(); a.Next(); {
		err = e.gatherInstances(ids, insts)
		if err == nil || err != environs.ErrPartialInstances {
			break
		}
	}
	if err == environs.ErrPartialInstances {
		for _, inst := range insts {
			if inst != nil {
				return insts, environs.ErrPartialInstances
			}
		}
		return nil, environs.ErrNoInstances
	}
	if err != nil {
		return nil, err
	}
	return insts, nil
}

func (e *environ) fetchNetworkInterfaceId(ec2Inst *ec2.EC2, instId instance.Id) (string, error) {
	var err error
	var instancesResp *ec2.InstancesResp
	for a := shortAttempt.Start(); a.Next(); {
		instancesResp, err = ec2Inst.Instances([]string{string(instId)}, nil)
		if err == nil {
			break
		}
		logger.Tracef("Instances(%q) returned: %v", instId, err)
	}
	if err != nil {
		// either the instance doesn't exist or we couldn't get through to
		// the ec2 api
		return "", err
	}

	if len(instancesResp.Reservations) == 0 {
		return "", errors.New("unexpected AWS response: reservation not found")
	}
	if len(instancesResp.Reservations[0].Instances) == 0 {
		return "", errors.New("unexpected AWS response: instance not found")
	}
	if len(instancesResp.Reservations[0].Instances[0].NetworkInterfaces) == 0 {
		return "", errors.New("unexpected AWS response: network interface not found")
	}
	networkInterfaceId := instancesResp.Reservations[0].Instances[0].NetworkInterfaces[0].Id
	return networkInterfaceId, nil
}

// AllocateAddress requests an address to be allocated for the given
// instance on the given subnet. Implements NetworkingEnviron.AllocateAddress.
func (e *environ) AllocateAddress(instId instance.Id, _ network.Id, addr network.Address) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to allocate address %q for instance %q", addr, instId)

	var nicId string
	ec2Inst := e.ec2()
	nicId, err = e.fetchNetworkInterfaceId(ec2Inst, instId)
	if err != nil {
		return errors.Trace(err)
	}
	for a := shortAttempt.Start(); a.Next(); {
		err = AssignPrivateIPAddress(ec2Inst, nicId, addr)
		logger.Tracef("AssignPrivateIPAddresses(%v, %v) returned: %v", nicId, addr, err)
		if err == nil {
			logger.Tracef("allocated address %v for instance %v, NIC %v", addr, instId, nicId)
			break
		}
		if ec2Err, ok := err.(*ec2.Error); ok {
			if ec2Err.Code == invalidParameterValue {
				// Note: this Code is also used if we specify
				// an IP address outside the subnet. Take care!
				logger.Tracef("address %q not available for allocation", addr)
				return environs.ErrIPAddressUnavailable
			} else if ec2Err.Code == privateAddressLimitExceeded {
				logger.Tracef("no more addresses available on the subnet")
				return environs.ErrIPAddressesExhausted
			}
		}

	}
	return err
}

// ReleaseAddress releases a specific address previously allocated with
// AllocateAddress. Implements NetworkingEnviron.ReleaseAddress.
func (e *environ) ReleaseAddress(instId instance.Id, _ network.Id, addr network.Address) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to release address %q from instance %q", addr, instId)

	var nicId string
	ec2Inst := e.ec2()
	nicId, err = e.fetchNetworkInterfaceId(ec2Inst, instId)
	if err != nil {
		return errors.Trace(err)
	}
	for a := shortAttempt.Start(); a.Next(); {
		_, err = ec2Inst.UnassignPrivateIPAddresses(nicId, []string{addr.Value})
		logger.Tracef("UnassignPrivateIPAddresses(%q, %q) returned: %v", nicId, addr, err)
		if err == nil {
			logger.Tracef("released address %q from instance %q, NIC %q", addr, instId, nicId)
			break
		}
	}
	return err
}

// NetworkInterfaces implements NetworkingEnviron.NetworkInterfaces.
func (e *environ) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	ec2Client := e.ec2()
	var err error
	var networkInterfacesResp *ec2.NetworkInterfacesResp
	for a := shortAttempt.Start(); a.Next(); {
		logger.Tracef("retrieving NICs for instance %q", instId)
		filter := ec2.NewFilter()
		filter.Add("attachment.instance-id", string(instId))
		networkInterfacesResp, err = ec2Client.NetworkInterfaces(nil, filter)
		logger.Tracef("instance %q NICs: %#v (err: %v)", instId, networkInterfacesResp, err)
		if err != nil {
			logger.Warningf("failed to get instance %q interfaces: %v (retrying)", instId, err)
			continue
		}
		if len(networkInterfacesResp.Interfaces) == 0 {
			logger.Tracef("instance %q has no NIC attachment yet, retrying...", instId)
			continue
		}
		logger.Tracef("found instance %q NICS: %#v", instId, networkInterfacesResp.Interfaces)
		break
	}
	if err != nil {
		// either the instance doesn't exist or we couldn't get through to
		// the ec2 api
		return nil, errors.Annotatef(err, "cannot get instance %q network interfaces", instId)
	}
	ec2Interfaces := networkInterfacesResp.Interfaces
	result := make([]network.InterfaceInfo, len(ec2Interfaces))
	for i, iface := range ec2Interfaces {
		resp, err := ec2Client.Subnets([]string{iface.SubnetId}, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to retrieve subnet %q info", iface.SubnetId)
		}
		if len(resp.Subnets) != 1 {
			return nil, errors.Errorf("expected 1 subnet, got %d", len(resp.Subnets))
		}
		subnet := resp.Subnets[0]
		cidr := subnet.CIDRBlock

		result[i] = network.InterfaceInfo{
			DeviceIndex:      iface.Attachment.DeviceIndex,
			MACAddress:       iface.MACAddress,
			CIDR:             cidr,
			NetworkName:      "", // Not needed for now.
			ProviderId:       network.Id(iface.Id),
			ProviderSubnetId: network.Id(iface.SubnetId),
			VLANTag:          0, // Not supported on EC2.
			// Getting the interface name is not supported on EC2, so fake it.
			InterfaceName: fmt.Sprintf("unsupported%d", iface.Attachment.DeviceIndex),
			Disabled:      false,
			NoAutoStart:   false,
			ConfigType:    network.ConfigDHCP,
			Address:       network.NewScopedAddress(iface.PrivateIPAddress, network.ScopeCloudLocal),
		}
	}
	return result, nil
}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance. subnetIds must not be
// empty. Implements NetworkingEnviron.Subnets.
func (e *environ) Subnets(_ instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	// At some point in the future an empty netIds may mean "fetch all subnets"
	// but until that functionality is needed it's an error.
	if len(subnetIds) == 0 {
		return nil, errors.Errorf("subnetIds must not be empty")
	}
	ec2Inst := e.ec2()
	// We can't filter by instance id here, unfortunately.
	resp, err := ec2Inst.Subnets(nil, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to retrieve subnets")
	}

	subIdSet := make(map[string]bool)
	for _, subId := range subnetIds {
		subIdSet[string(subId)] = false
	}

	var results []network.SubnetInfo
	for _, subnet := range resp.Subnets {
		_, ok := subIdSet[subnet.Id]
		if !ok {
			logger.Tracef("subnet %q not in %v, skipping", subnet.Id, subnetIds)
			continue
		}
		subIdSet[subnet.Id] = true

		cidr := subnet.CIDRBlock
		ip, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			logger.Warningf("skipping subnet %q, invalid CIDR: %v", cidr, err)
			continue
		}
		// ec2 only uses IPv4 addresses for subnets
		start, err := network.IPv4ToDecimal(ip)
		if err != nil {
			logger.Warningf("skipping subnet %q, invalid IP: %v", cidr, err)
			continue
		}
		// First four addresses in a subnet are reserved, see
		// http://goo.gl/rrWTIo
		allocatableLow := network.DecimalToIPv4(start + 4)

		ones, bits := ipnet.Mask.Size()
		zeros := bits - ones
		numIPs := uint32(1) << uint32(zeros)
		highIP := start + numIPs - 1
		// The last address in a subnet is also reserved (see same ref).
		allocatableHigh := network.DecimalToIPv4(highIP - 1)

		info := network.SubnetInfo{
			CIDR:              cidr,
			ProviderId:        network.Id(subnet.Id),
			VLANTag:           0, // Not supported on EC2
			AllocatableIPLow:  allocatableLow,
			AllocatableIPHigh: allocatableHigh,
		}
		logger.Tracef("found subnet with info %#v", info)
		results = append(results, info)
	}

	notFound := []string{}
	for subId, found := range subIdSet {
		if !found {
			notFound = append(notFound, subId)
		}
	}
	if len(notFound) != 0 {
		return nil, errors.Errorf("failed to find the following subnet ids: %v", notFound)
	}

	return results, nil
}

func (e *environ) AllInstances() ([]instance.Instance, error) {
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "pending", "running")
	err := e.addGroupFilter(filter)
	if err != nil {
		if ec2ErrCode(err) == "InvalidGroup.NotFound" {
			return nil, nil
		}
		return nil, err
	}
	resp, err := e.ec2().Instances(nil, filter)
	if err != nil {
		return nil, err
	}
	var insts []instance.Instance
	for _, r := range resp.Reservations {
		for i := range r.Instances {
			inst := r.Instances[i]
			// TODO(wallyworld): lookup the details to fill in the instance type data
			insts = append(insts, &ec2Instance{e: e, Instance: &inst})
		}
	}
	return insts, nil
}

func (e *environ) Destroy() error {
	if err := common.Destroy(e); err != nil {
		return errors.Trace(err)
	}
	return e.Storage().RemoveAll()
}

func portsToIPPerms(ports []network.PortRange) []ec2.IPPerm {
	ipPerms := make([]ec2.IPPerm, len(ports))
	for i, p := range ports {
		ipPerms[i] = ec2.IPPerm{
			Protocol:  p.Protocol,
			FromPort:  p.FromPort,
			ToPort:    p.ToPort,
			SourceIPs: []string{"0.0.0.0/0"},
		}
	}
	return ipPerms
}

func (e *environ) openPortsInGroup(name string, ports []network.PortRange) error {
	if len(ports) == 0 {
		return nil
	}
	// Give permissions for anyone to access the given ports.
	g, err := e.groupByName(name)
	if err != nil {
		return err
	}
	ipPerms := portsToIPPerms(ports)
	_, err = e.ec2().AuthorizeSecurityGroup(g, ipPerms)
	if err != nil && ec2ErrCode(err) == "InvalidPermission.Duplicate" {
		if len(ports) == 1 {
			return nil
		}
		// If there's more than one port and we get a duplicate error,
		// then we go through authorizing each port individually,
		// otherwise the ports that were *not* duplicates will have
		// been ignored
		for i := range ipPerms {
			_, err := e.ec2().AuthorizeSecurityGroup(g, ipPerms[i:i+1])
			if err != nil && ec2ErrCode(err) != "InvalidPermission.Duplicate" {
				return fmt.Errorf("cannot open port %v: %v", ipPerms[i], err)
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot open ports: %v", err)
	}
	return nil
}

func (e *environ) closePortsInGroup(name string, ports []network.PortRange) error {
	if len(ports) == 0 {
		return nil
	}
	// Revoke permissions for anyone to access the given ports.
	// Note that ec2 allows the revocation of permissions that aren't
	// granted, so this is naturally idempotent.
	g, err := e.groupByName(name)
	if err != nil {
		return err
	}
	_, err = e.ec2().RevokeSecurityGroup(g, portsToIPPerms(ports))
	if err != nil {
		return fmt.Errorf("cannot close ports: %v", err)
	}
	return nil
}

func (e *environ) portsInGroup(name string) (ports []network.PortRange, err error) {
	group, err := e.groupInfoByName(name)
	if err != nil {
		return nil, err
	}
	for _, p := range group.IPPerms {
		if len(p.SourceIPs) != 1 {
			logger.Warningf("unexpected IP permission found: %v", p)
			continue
		}
		ports = append(ports, network.PortRange{
			Protocol: p.Protocol,
			FromPort: p.FromPort,
			ToPort:   p.ToPort,
		})
	}
	network.SortPortRanges(ports)
	return ports, nil
}

func (e *environ) OpenPorts(ports []network.PortRange) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on environment",
			e.Config().FirewallMode())
	}
	if err := e.openPortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("opened ports in global group: %v", ports)
	return nil
}

func (e *environ) ClosePorts(ports []network.PortRange) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on environment",
			e.Config().FirewallMode())
	}
	if err := e.closePortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("closed ports in global group: %v", ports)
	return nil
}

func (e *environ) Ports() ([]network.PortRange, error) {
	if e.Config().FirewallMode() != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from environment",
			e.Config().FirewallMode())
	}
	return e.portsInGroup(e.globalGroupName())
}

func (*environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

func (e *environ) terminateInstances(ids []instance.Id) error {
	if len(ids) == 0 {
		return nil
	}
	var err error
	ec2inst := e.ec2()
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = string(id)
	}
	for a := shortAttempt.Start(); a.Next(); {
		_, err = ec2inst.TerminateInstances(strs)
		if err == nil || ec2ErrCode(err) != "InvalidInstanceID.NotFound" {
			return err
		}
	}
	if len(ids) == 1 {
		return err
	}
	// If we get a NotFound error, it means that no instances have been
	// terminated even if some exist, so try them one by one, ignoring
	// NotFound errors.
	var firstErr error
	for _, id := range ids {
		_, err = ec2inst.TerminateInstances([]string{string(id)})
		if ec2ErrCode(err) == "InvalidInstanceID.NotFound" {
			err = nil
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (e *environ) globalGroupName() string {
	return fmt.Sprintf("%s-global", e.jujuGroupName())
}

func (e *environ) machineGroupName(machineId string) string {
	return fmt.Sprintf("%s-%s", e.jujuGroupName(), machineId)
}

func (e *environ) jujuGroupName() string {
	return "juju-" + e.name
}

// setUpGroups creates the security groups for the new machine, and
// returns them.
//
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same EC2 account.  In
// addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
func (e *environ) setUpGroups(machineId string, apiPort int) ([]ec2.SecurityGroup, error) {
	jujuGroup, err := e.ensureGroup(e.jujuGroupName(),
		[]ec2.IPPerm{
			{
				Protocol:  "tcp",
				FromPort:  22,
				ToPort:    22,
				SourceIPs: []string{"0.0.0.0/0"},
			},
			{
				Protocol:  "tcp",
				FromPort:  apiPort,
				ToPort:    apiPort,
				SourceIPs: []string{"0.0.0.0/0"},
			},
			{
				Protocol: "tcp",
				FromPort: 0,
				ToPort:   65535,
			},
			{
				Protocol: "udp",
				FromPort: 0,
				ToPort:   65535,
			},
			{
				Protocol: "icmp",
				FromPort: -1,
				ToPort:   -1,
			},
		})
	if err != nil {
		return nil, err
	}
	var machineGroup ec2.SecurityGroup
	switch e.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = e.ensureGroup(e.machineGroupName(machineId), nil)
	case config.FwGlobal:
		machineGroup, err = e.ensureGroup(e.globalGroupName(), nil)
	}
	if err != nil {
		return nil, err
	}
	return []ec2.SecurityGroup{jujuGroup, machineGroup}, nil
}

// zeroGroup holds the zero security group.
var zeroGroup ec2.SecurityGroup

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
// Any entries in perms without SourceIPs will be granted for
// the named group only.
func (e *environ) ensureGroup(name string, perms []ec2.IPPerm) (g ec2.SecurityGroup, err error) {
	ec2inst := e.ec2()
	resp, err := ec2inst.CreateSecurityGroup("", name, "juju group")
	if err != nil && ec2ErrCode(err) != "InvalidGroup.Duplicate" {
		return zeroGroup, err
	}

	var have permSet
	if err == nil {
		g = resp.SecurityGroup
	} else {
		resp, err := ec2inst.SecurityGroups(ec2.SecurityGroupNames(name), nil)
		if err != nil {
			return zeroGroup, err
		}
		info := resp.Groups[0]
		// It's possible that the old group has the wrong
		// description here, but if it does it's probably due
		// to something deliberately playing games with juju,
		// so we ignore it.
		g = info.SecurityGroup
		have = newPermSetForGroup(info.IPPerms, g)
	}
	want := newPermSetForGroup(perms, g)
	revoke := make(permSet)
	for p := range have {
		if !want[p] {
			revoke[p] = true
		}
	}
	if len(revoke) > 0 {
		_, err := ec2inst.RevokeSecurityGroup(g, revoke.ipPerms())
		if err != nil {
			return zeroGroup, fmt.Errorf("cannot revoke security group: %v", err)
		}
	}

	add := make(permSet)
	for p := range want {
		if !have[p] {
			add[p] = true
		}
	}
	if len(add) > 0 {
		_, err := ec2inst.AuthorizeSecurityGroup(g, add.ipPerms())
		if err != nil {
			return zeroGroup, fmt.Errorf("cannot authorize securityGroup: %v", err)
		}
	}
	return g, nil
}

// permKey represents a permission for a group or an ip address range
// to access the given range of ports. Only one of groupName or ipAddr
// should be non-empty.
type permKey struct {
	protocol string
	fromPort int
	toPort   int
	groupId  string
	ipAddr   string
}

type permSet map[permKey]bool

// newPermSetForGroup returns a set of all the permissions in the
// given slice of IPPerms. It ignores the name and owner
// id in source groups, and any entry with no source ips will
// be granted for the given group only.
func newPermSetForGroup(ps []ec2.IPPerm, group ec2.SecurityGroup) permSet {
	m := make(permSet)
	for _, p := range ps {
		k := permKey{
			protocol: p.Protocol,
			fromPort: p.FromPort,
			toPort:   p.ToPort,
		}
		if len(p.SourceIPs) > 0 {
			for _, ip := range p.SourceIPs {
				k.ipAddr = ip
				m[k] = true
			}
		} else {
			k.groupId = group.Id
			m[k] = true
		}
	}
	return m
}

// ipPerms returns m as a slice of permissions usable
// with the ec2 package.
func (m permSet) ipPerms() (ps []ec2.IPPerm) {
	// We could compact the permissions, but it
	// hardly seems worth it.
	for p := range m {
		ipp := ec2.IPPerm{
			Protocol: p.protocol,
			FromPort: p.fromPort,
			ToPort:   p.toPort,
		}
		if p.ipAddr != "" {
			ipp.SourceIPs = []string{p.ipAddr}
		} else {
			ipp.SourceGroups = []ec2.UserSecurityGroup{{Id: p.groupId}}
		}
		ps = append(ps, ipp)
	}
	return
}

// isZoneConstrainedError reports whether or not the error indicates
// RunInstances failed due to the specified availability zone being
// constrained for the instance type being provisioned, or is
// otherwise unusable for the specific request made.
func isZoneConstrainedError(err error) bool {
	switch err := err.(type) {
	case *ec2.Error:
		switch err.Code {
		case "Unsupported", "InsufficientInstanceCapacity":
			// A big hammer, but we've now seen several different error messages
			// for constrained zones, and who knows how many more there might
			// be. If the message contains "Availability Zone", it's a fair
			// bet that it's constrained or otherwise unusable.
			return strings.Contains(err.Message, "Availability Zone")
		case "InvalidInput":
			// If the region has a default VPC, then we will receive an error
			// if the AZ does not have a default subnet. Until we have proper
			// support for networks, we'll skip over these.
			return strings.HasPrefix(err.Message, "No default subnet for availability zone")
		case "VolumeTypeNotAvailableInZone":
			return true
		}
	}
	return false
}

// If the err is of type *ec2.Error, ec2ErrCode returns
// its code, otherwise it returns the empty string.
func ec2ErrCode(err error) string {
	ec2err, _ := err.(*ec2.Error)
	if ec2err == nil {
		return ""
	}
	return ec2err.Code
}
