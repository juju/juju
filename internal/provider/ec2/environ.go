// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"encoding/base64"
	stderrors "errors"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/kr/pretty"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	corecontext "github.com/juju/juju/core/context"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/storage"
)

const (
	// AWSClientContextKey defines a way to change the aws client func within
	// a context.
	AWSClientContextKey corecontext.ContextKey = "aws-client-func"

	// AWSIAMClientContextKey defines a way to change the aws iam client func
	// within a context.
	AWSIAMClientContextKey corecontext.ContextKey = "aws-iam-client-func"
)

const (
	invalidParameterValue = "InvalidParameterValue"

	// tagName is the AWS-specific tag key that populates resources'
	// name columns in the console.
	tagName = "Name"
)

var (
	// Use shortRetryStrategy to poll for short-term events or for retrying API calls.
	shortRetryStrategy = retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: 60 * time.Second,
		Delay:       200 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		BackoffFunc: retry.DoubleDelay,
	}

	// aliveInstanceStates are the states which we filter by when listing
	// instances in an environment.
	aliveInstanceStates = []string{"pending", "running"}

	// Ensure that environ implements FirewallFeatureQuerier.
	_ environs.FirewallFeatureQuerier = (*environ)(nil)
)

var _ Client = (*ec2.Client)(nil)

type environ struct {
	environs.NoSpaceDiscoveryEnviron
	environs.NoContainerAddressesEnviron
	common.CredentialInvalidator

	name           string
	cloud          environscloudspec.CloudSpec
	controllerUUID string

	iamClient     IAMClient
	iamClientFunc IAMClientFunc

	ec2Client     Client
	ec2ClientFunc ClientFunc

	// ecfgMutex protects the *Unlocked fields below.
	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig

	// namespace is used to create the machine and device hostnames.
	namespace instance.Namespace

	instTypesCache      *instanceTypeCache
	instTypesCacheMutex sync.RWMutex

	defaultVPCMutex   sync.Mutex
	defaultVPCChecked bool
	defaultVPC        *types.Vpc

	ensureGroupMutex sync.Mutex
}

func newEnviron(name, controllerUUID string, invalidator environs.CredentialInvalidator) *environ {
	return &environ{
		CredentialInvalidator: newCredentialInvalidator(common.NewCredentialInvalidator(invalidator, isAuthorizationError)),

		name:           name,
		controllerUUID: controllerUUID,

		ec2ClientFunc: clientFunc,
		iamClientFunc: iamClientFunc,
	}
}

var _ environs.Environ = (*environ)(nil)

var _ environs.Networking = (*environ)(nil)

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
}

func (e *environ) SetConfig(ctx context.Context, cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(ctx, cfg)
	if err != nil {
		return errors.Trace(err)
	}
	e.ecfgMutex.Lock()
	e.ecfgUnlocked = ecfg
	e.ecfgMutex.Unlock()
	return nil
}

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) Name() string {
	return e.name
}

// PrepareForBootstrap is part of the Environ interface.
func (e *environ) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(ctx, e.CredentialInvalidator, e.ec2Client); err != nil {
			return err
		}
	}
	ecfg := e.ecfg()
	vpcID, forceVPCID := ecfg.vpcID(), ecfg.forceVPCID()
	if err := validateBootstrapVPC(ctx, e.ec2Client, e.cloud.Region, vpcID, forceVPCID, ctx); err != nil {
		return errors.Trace(e.HandleCredentialError(ctx, err))
	}
	return nil
}

// ValidateProviderForNewModel is part of the [environs.ModelResources] interface.
func (e *environ) ValidateProviderForNewModel(ctx context.Context) error {
	if err := verifyCredentials(ctx, e.CredentialInvalidator, e.ec2Client); err != nil {
		return err
	}
	vpcID := e.ecfg().vpcID()
	if err := validateModelVPC(ctx, e.ec2Client, e.name, vpcID); err != nil {
		return errors.Trace(e.HandleCredentialError(ctx, err))
	}
	return nil
}

// CreateModelResources is part of the [environs.ModelResources] interface.
func (e *environ) CreateModelResources(ctx context.Context, args environs.CreateParams) error {
	return nil
}

// FinaliseBootstrapCredential is responsible for performing and finalisation
// steps to a credential being passwed to a newly bootstrapped controller. This
// was introduced to help with the transformation to instance roles.
func (e *environ) FinaliseBootstrapCredential(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
	cred *cloud.Credential,
) (*cloud.Credential, error) {
	if !args.BootstrapConstraints.HasInstanceRole() {
		return cred, nil
	}

	instanceRoleName := *args.BootstrapConstraints.InstanceRole
	newCred := cloud.NewCredential(cloud.InstanceRoleAuthType, map[string]string{
		"instance-profile-name": instanceRoleName,
	})
	return &newCred, nil
}

// Bootstrap is part of the Environ interface.
func (e *environ) Bootstrap(ctx environs.BootstrapContext, callCtx envcontext.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	// We are going to take a look at the Bootstrap constraints and see if we have to make an instance profile
	r, err := common.Bootstrap(ctx, e, callCtx, args)
	return r, e.HandleCredentialError(ctx, err)
}

func (e *environ) CreateAutoInstanceRole(
	ctx envcontext.ProviderCallContext,
	args environs.BootstrapParams,
) (string, error) {
	_, exists := args.ControllerConfig[controller.ControllerName]
	if !exists {
		return "", errors.NewNotValid(nil, "cannot find controller name in config")
	}
	controllerName, ok := args.ControllerConfig[controller.ControllerName].(string)
	if !ok {
		return "", errors.NewNotValid(nil, "controller name in config is not a valid string")
	}
	controllerUUID := args.ControllerConfig[controller.ControllerUUIDKey].(string)
	instProfile, cleanups, err := ensureControllerInstanceProfile(
		ctx,
		e.iamClient,
		controllerName,
		controllerUUID)
	if err != nil {
		for _, c := range cleanups {
			c()
		}
		return "", err
	}
	return *instProfile.InstanceProfileName, nil
}

func (e *environ) SupportsInstanceRoles(_ envcontext.ProviderCallContext) bool {
	return true
}

// SupportsSpaces is specified on environs.Networking.
func (e *environ) SupportsSpaces() (bool, error) {
	return true, nil
}

func (e *environ) instanceTypeCache() *instanceTypeCache {
	e.instTypesCacheMutex.RLock()
	defer e.instTypesCacheMutex.RUnlock()
	return e.instTypesCache
}

var unsupportedConstraints = []string{
	constraints.Tags,
	// TODO(anastasiamac 2016-03-16) LP#1557874
	// use virt-type in StartInstances
	constraints.VirtType,
	constraints.AllocatePublicIP,
}

// ConstraintsValidator is defined on the Environs interface.
func (e *environ) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterVocabulary(
		constraints.Arch,
		[]string{arch.AMD64, arch.ARM64},
	)
	validator.RegisterConflicts(
		[]string{constraints.InstanceType},
		[]string{constraints.Arch, constraints.Mem, constraints.Cores, constraints.CpuPower})
	validator.RegisterUnsupported(unsupportedConstraints)

	instanceTypes, err := e.supportedInstanceTypes(ctx, allInstanceTypeFilter())
	if err != nil {
		return nil, errors.Trace(e.HandleCredentialError(ctx, err))
	}
	sort.Sort(instances.ByName(instanceTypes))
	instTypeNames := make([]string, len(instanceTypes))
	for i, itype := range instanceTypes {
		instTypeNames[i] = itype.Name
	}
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)
	validator.RegisterConflictResolver(constraints.InstanceType, constraints.Arch, func(attrValues map[string]interface{}) error {
		instanceTypeName, ok := attrValues[constraints.InstanceType].(string)
		if !ok {
			return nil
		}
		arch, ok := attrValues[constraints.Arch].(string)
		if !ok {
			return nil
		}
		for _, itype := range instanceTypes {
			if itype.Name != instanceTypeName {
				continue
			}
			if itype.Arch != arch {
				return fmt.Errorf("%v=%q expected %v=%q not %q",
					constraints.InstanceType, instanceTypeName,
					constraints.Arch, itype.Arch, arch)
			}
			// The instance-type and arch are a valid combination.
			return nil
		}
		// Should never get here as the instance type value should be already validated to be
		// in instanceTypes.
		return errors.NotFoundf("%v %q", constraints.InstanceType, instanceTypeName)
	})

	return validator, nil
}

var ec2AvailabilityZones = func(client Client, ctx context.Context, in *ec2.DescribeAvailabilityZonesInput, opts ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return client.DescribeAvailabilityZones(ctx, in, opts...)
}

type ec2AvailabilityZone struct {
	types.AvailabilityZone
}

func (z *ec2AvailabilityZone) Name() string {
	return aws.ToString(z.AvailabilityZone.ZoneName)
}

func (z *ec2AvailabilityZone) Available() bool {
	return z.AvailabilityZone.State == types.AvailabilityZoneStateAvailable
}

// AvailabilityZones returns a slice of availability zones
// for the configured region.
func (e *environ) AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	filter := makeFilter("region-name", e.cloud.Region)
	resp, err := ec2AvailabilityZones(e.ec2Client, ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []types.Filter{filter},
	})
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	zones := make(network.AvailabilityZones, len(resp.AvailabilityZones))
	for i, z := range resp.AvailabilityZones {
		zones[i] = &ec2AvailabilityZone{AvailabilityZone: z}
	}
	return zones, nil
}

// InstanceAvailabilityZoneNames returns the availability zone names for each
// of the specified instances.
func (e *environ) InstanceAvailabilityZoneNames(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
	instances, err := e.Instances(ctx, ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, errors.Trace(err)
	}

	return gatherAvailabilityZones(instances), nil
}

// AvailabilityZoner defines a institute interface for getting an az from an
// instance.
type AvailabilityZoner interface {
	AvailabilityZone() (string, bool)
}

func gatherAvailabilityZones(instances []instances.Instance) map[instance.Id]string {
	zones := make(map[instance.Id]string)
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		t, ok := inst.(AvailabilityZoner)
		if !ok {
			continue
		}
		az, ok := t.AvailabilityZone()
		if !ok {
			continue
		}
		zones[inst.Id()] = az
	}
	return zones
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (e *environ) DeriveAvailabilityZones(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	availabilityZone, err := e.deriveAvailabilityZone(ctx, args)
	if availabilityZone != "" {
		return []string{availabilityZone}, errors.Trace(err)
	}
	return nil, errors.Trace(err)
}

type ec2Placement struct {
	availabilityZone types.AvailabilityZone
	subnet           *types.Subnet
}

func (e *environ) parsePlacement(ctx envcontext.ProviderCallContext, placement string) (*ec2Placement, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, fmt.Errorf("unknown placement directive: %v", placement)
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		availabilityZone := value
		zones, err := e.AvailabilityZones(ctx)
		if err != nil {
			return nil, err
		}
		for _, z := range zones {
			if z.Name() == availabilityZone {
				ec2AZ := z.(*ec2AvailabilityZone)
				return &ec2Placement{
					availabilityZone: ec2AZ.AvailabilityZone,
				}, nil
			}
		}
		return nil, fmt.Errorf("invalid availability zone %q", availabilityZone)
	case "subnet":
		logger.Debugf(ctx, "searching for subnet matching placement directive %q", value)
		matcher := CreateSubnetMatcher(value)
		// Get all known subnets, look for a match
		subnets, vpcID, err := e.subnetsForVPC(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// we'll also need info about this zone, we don't have a way right now to ask about a single AZ, so punt
		zones, err := e.AvailabilityZones(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subnetIDs := make([]string, 0, len(subnets))
		for _, subnet := range subnets {
			subnetIDs = append(subnetIDs, *subnet.SubnetId)
			if matcher.Match(subnet) {
				// We found the CIDR, now see if we can find the AZs.
				for _, zone := range zones {
					if zone.Name() == aws.ToString(subnet.AvailabilityZone) {
						ec2AZ := zone.(*ec2AvailabilityZone)
						return &ec2Placement{
							availabilityZone: ec2AZ.AvailabilityZone,
							subnet:           &subnet,
						}, nil
					}
				}
				logger.Debugf(ctx, "found a matching subnet (%v) but couldn't find the AZ", subnet)
			}
		}
		return nil, fmt.Errorf("unable to find subnet %q in %v for vpi-id %q%w", value, subnetIDs, vpcID, errors.Hide(errors.NotFound))
	}
	return nil, fmt.Errorf("unknown placement directive: %v", placement)
}

// PrecheckInstance is defined on the environs.InstancePrechecker interface.
func (e *environ) PrecheckInstance(ctx envcontext.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if _, _, err := e.deriveAvailabilityZoneAndSubnetID(ctx,
		environs.StartInstanceParams{
			Placement:         args.Placement,
			VolumeAttachments: args.VolumeAttachments,
		},
	); err != nil {
		return errors.Trace(err)
	}
	if !args.Constraints.HasInstanceType() {
		return nil
	}
	// Constraint has an instance-type constraint so let's see if it is valid.
	instanceTypes, err := e.supportedInstanceTypes(ctx, exactInstanceTypeFilter(types.InstanceType(*args.Constraints.InstanceType)))
	if err != nil {
		return errors.Trace(e.HandleCredentialError(ctx, err))
	}
	for _, itype := range instanceTypes {
		if !args.Constraints.HasArch() || *args.Constraints.Arch == itype.Arch {
			return nil
		}
	}
	if args.Constraints.Arch == nil {
		return fmt.Errorf("invalid AWS instance type %q specified", *args.Constraints.InstanceType)
	}
	return fmt.Errorf("invalid AWS instance type %q and arch %q specified", *args.Constraints.InstanceType, *args.Constraints.Arch)
}

// AgentMetadataLookupParams returns parameters which are used to query agent simple-streams metadata.
func (e *environ) AgentMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	base := config.PreferredBase(e.ecfg())
	return e.metadataLookupParams(region, base.OS)
}

// ImageMetadataLookupParams returns parameters which are used to query image simple-streams metadata.
func (e *environ) ImageMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	base := config.PreferredBase(e.ecfg())
	return e.metadataLookupParams(region, base.Channel.Track)
}

// MetadataLookupParams returns parameters which are used to query simple-streams metadata.
func (e *environ) metadataLookupParams(region, release string) (*simplestreams.MetadataLookupParams, error) {
	var endpoint string
	if region == "" {
		region = e.cloud.Region
		endpoint = e.cloud.Endpoint
	} else {
		// TODO(axw) 2016-10-04 #1630089
		// MetadataLookupParams needs to be updated so that providers
		// are not expected to know how to map regions to endpoints.
		resolver := ec2.NewDefaultEndpointResolver()
		ep, err := resolver.ResolveEndpoint(region, ec2.EndpointResolverOptions{})
		if err != nil {
			return nil, errors.Annotatef(err, "unknown region %q", region)
		}
		endpoint = ep.URL
	}
	return &simplestreams.MetadataLookupParams{
		Release:  release,
		Region:   region,
		Endpoint: endpoint,
	}, nil
}

// Region is specified in the HasRegion interface.
func (e *environ) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   e.cloud.Region,
		Endpoint: e.cloud.Endpoint,
	}, nil
}

const (
	ebsStorage    = "ebs"
	ssdGP3Storage = "ssd-gp3"
	ssdStorage    = "ssd"
)

// resourceName returns the string to use for a resource's Name tag,
// to help users identify Juju-managed resources in the AWS console.
func resourceName(tag names.Tag, envName string) string {
	return fmt.Sprintf("juju-%s-%s", envName, tag)
}

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(
	ctx envcontext.ProviderCallContext, args environs.StartInstanceParams,
) (_ *environs.StartInstanceResult, resultErr error) {
	var inst *sdkInstance
	callback := args.StatusCallback
	defer func() {
		if resultErr == nil || inst == nil {
			return
		}
		if err := e.StopInstances(ctx, inst.Id()); err != nil {
			_ = callback(ctx, status.Error, fmt.Sprintf("error stopping failed instance: %v", err), nil)
			logger.Errorf(ctx, "error stopping failed instance: %v", err)
		}
	}()

	_ = callback(ctx, status.Allocating, "Verifying availability zone", nil)

	annotateWrapError := func(received error, annotation string) error {
		if received == nil {
			return nil
		}
		// If there is a problem with authentication/authorisation,
		// we want a correctly typed error.
		annotatedErr := errors.Annotate(e.HandleCredentialError(ctx, received), annotation)
		if errors.Is(annotatedErr, common.ErrorCredentialNotValid) {
			return annotatedErr
		}
		return environs.ZoneIndependentError(annotatedErr)
	}

	wrapError := func(received error) error {
		return annotateWrapError(received, "")
	}

	// Verify the supplied availability zone to start the instance in.
	// It is provided via Constraints or AvailabilityZone in
	// StartInstanceParams.
	availabilityZone, placementSubnetID, err := e.deriveAvailabilityZoneAndSubnetID(ctx, args)
	if err != nil {
		// An IsNotValid error is returned if the zone is invalid;
		// this is a zone-specific error.
		zoneSpecific := errors.Is(err, errors.NotValid)
		if !zoneSpecific {
			return nil, wrapError(err)
		}
		return nil, errors.Trace(err)
	}

	subnetZones, err := getValidSubnetZoneMap(ctx, args)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	hasVPCID := isVPCIDSet(e.ecfg().vpcID())

	instFilter, err := e.generalPurposeInstanceFilter(ctx, e.instanceTypeCache())
	if err != nil {
		return nil, errors.Trace(err)
	}

	if args.Constraints.HasInstanceType() {
		instFilter = oneOfInstanceTypeFilter(
			instFilter,
			exactInstanceTypeFilter(types.InstanceType(*args.Constraints.InstanceType)),
		)
	}

	instanceTypes, err := e.supportedInstanceTypes(ctx, instFilter)
	if err != nil {
		return nil, wrapError(err)
	}

	subnet, err := e.selectSubnetForInstance(ctx, hasVPCID, subnetZones, placementSubnetID, availabilityZone)
	if err != nil {
		return nil, errors.Trace(err)
	}

	arch, err := args.Tools.OneArch()
	if err != nil {
		return nil, errors.Trace(err)
	}

	spec, err := findInstanceSpec(
		ctx,
		args.InstanceConfig.IsController(),
		args.ImageMetadata,
		instanceTypes,
		&instances.InstanceConstraint{
			Region:      e.cloud.Region,
			Base:        args.InstanceConfig.Base,
			Arch:        arch,
			Constraints: args.Constraints,
			Storage:     []string{ssdStorage, ebsStorage, ssdGP3Storage},
		},
	)
	if err != nil {
		return nil, wrapError(err)
	}

	if err := e.finishInstanceConfig(&args, spec); err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	_ = callback(ctx, status.Allocating, "Making user data", nil)
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, nil, AmazonRenderer{})
	if err != nil {
		return nil, environs.ZoneIndependentError(fmt.Errorf("constructing user data: %w", err))
	}
	logger.Debugf(ctx, "ec2 user data; %d bytes", len(userData))

	_ = callback(ctx, status.Allocating, "Setting up groups", nil)
	groupIDs, err := e.setUpGroups(ctx, args.ControllerUUID, args.InstanceConfig.MachineId)
	if err != nil {
		return nil, annotateWrapError(err, "cannot set up groups")
	}

	blockDeviceMappings, err := getBlockDeviceMappings(
		args.Constraints,
		args.InstanceConfig.Base.OS,
		args.InstanceConfig.IsController(),
		args.RootDisk,
	)
	if err != nil {
		return nil, annotateWrapError(err, "cannot get block device mapping")
	}
	rootDiskSize := uint64(aws.ToInt32(blockDeviceMappings[0].Ebs.VolumeSize)) * 1024

	hostname, err := e.namespace.Hostname(args.InstanceConfig.MachineId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	args.InstanceConfig.Tags[tagName] = hostname

	instanceTags := CreateTagSpecification(types.ResourceTypeInstance, args.InstanceConfig.Tags)

	cfg := e.Config()
	rootVolumeTags := tags.ResourceTags(
		names.NewModelTag(cfg.UUID()),
		names.NewControllerTag(args.ControllerUUID),
		cfg,
	)
	rootVolumeTags[tagName] = hostname + "-root"
	volumeTags := CreateTagSpecification(types.ResourceTypeVolume, rootVolumeTags)

	imageID := aws.String(spec.Image.Id)
	if args.Constraints.HasImageID() {
		imageID = aws.String(*args.Constraints.ImageID)
	}

	var instResp *ec2.RunInstancesOutput
	commonRunArgs := &ec2.RunInstancesInput{
		MinCount:            aws.Int32(1),
		MaxCount:            aws.Int32(1),
		UserData:            aws.String(base64.StdEncoding.EncodeToString(userData)),
		InstanceType:        types.InstanceType(spec.InstanceType.Name),
		SecurityGroupIds:    groupIDs,
		BlockDeviceMappings: blockDeviceMappings,
		ImageId:             imageID,
		MetadataOptions: &types.InstanceMetadataOptionsRequest{
			HttpEndpoint: types.InstanceMetadataEndpointStateEnabled,
			// By forcing HTTP tokens here we move all created instances over to
			// IMDSv2.
			// Fixes lp1960568
			HttpTokens: types.HttpTokensStateRequired,
		},
		TagSpecifications: []types.TagSpecification{
			instanceTags,
			volumeTags,
		},
	}

	runArgs := commonRunArgs
	runArgs.Placement = &types.Placement{
		AvailabilityZone: aws.String(availabilityZone),
	}
	runArgs.SubnetId = subnet.SubnetId

	_ = callback(ctx, status.Allocating,
		fmt.Sprintf("Trying to start instance in availability zone %q", availabilityZone), nil)

	instResp, err = runInstances(ctx, e.ec2Client, runArgs, callback)
	if err != nil {
		err = e.HandleCredentialError(ctx, err)
		if !isZoneOrSubnetConstrainedError(err) {
			err = annotateWrapError(err, "cannot run instances")
		}
		return nil, err
	} else if len(instResp.Instances) != 1 {
		return nil, errors.Errorf("expected 1 started instance, got %d", len(instResp.Instances))
	}

	inst = &sdkInstance{
		e: e,
		i: instResp.Instances[0],
	}
	// In the case the allocate-public-ip constraint is set to false, we
	// need to dissociate the public IP address from the instance.
	allocatePublicIP := args.Constraints.AllocatePublicIP
	if allocatePublicIP != nil && !*allocatePublicIP {
		// We can safely dissociate the public IP address from the
		// first (primary) network interface of the instance, because
		// (from the AWS docs):
		// This option can be enabled for any network interface but will only apply to the
		// primary network interface (eth0).
		_, err := e.ec2Client.ModifyNetworkInterfaceAttribute(ctx, &ec2.ModifyNetworkInterfaceAttributeInput{
			NetworkInterfaceId:       inst.i.NetworkInterfaces[0].NetworkInterfaceId,
			AssociatePublicIpAddress: aws.Bool(false),
		})
		if err != nil {
			return nil, errors.Errorf("dissociating public IP address from instance %q: %v", inst.Id(), err)
		}
	}
	instAZ, _ := inst.AvailabilityZone()
	if hasVPCID {
		instVPC := e.ecfg().vpcID()
		instSubnet := aws.ToString(inst.i.SubnetId)
		logger.Infof(ctx, "started instance %q in AZ %q, subnet %q, VPC %q", inst.Id(), instAZ, instSubnet, instVPC)
	} else {
		logger.Infof(ctx, "started instance %q in AZ %q", inst.Id(), instAZ)
	}

	hc := instance.HardwareCharacteristics{
		Arch:     &spec.Image.Arch,
		Mem:      &spec.InstanceType.Mem,
		CpuCores: &spec.InstanceType.CpuCores,
		CpuPower: spec.InstanceType.CpuPower,
		RootDisk: &rootDiskSize,
		// Tags currently not supported by EC2
	}
	if instAZ != "" {
		hc.AvailabilityZone = &instAZ
	}

	if err := e.maybeAttachInstanceProfile(ctx, callback, inst, args.Constraints); err != nil {
		return nil, err
	}

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: &hc,
	}, nil
}

// maybeAttachInstanceProfile assesses if an instance profile needs to be
// attached to an instance based on it's constraints. If the instance
// constraints do not specify an instance role then this func returns silently.
func (e *environ) maybeAttachInstanceProfile(
	ctx envcontext.ProviderCallContext,
	statusCallback environs.StatusCallbackFunc,
	instance *sdkInstance,
	constraints constraints.Value,
) error {
	if !constraints.HasInstanceRole() {
		return nil
	}

	_ = statusCallback(
		ctx,
		status.Allocating,
		fmt.Sprintf("finding aws instance profile %s", *constraints.InstanceRole),
		nil,
	)
	instProfile, err := findInstanceProfileFromName(ctx, e.iamClient, *constraints.InstanceRole)
	if err != nil {
		return errors.Annotatef(err, "findining instance profile %s", *constraints.InstanceRole)
	}

	_ = statusCallback(
		ctx,
		status.Allocating,
		fmt.Sprintf("attaching aws instance profile %s", *instProfile.Arn),
		nil,
	)
	if err := setInstanceProfileWithWait(ctx, e.ec2Client, instProfile, instance, e); err != nil {
		return errors.Annotatef(
			err,
			"attaching instance profile %s to instance %s",
			*instProfile.Arn,
			instance.Id(),
		)
	}

	return nil
}

func (e *environ) finishInstanceConfig(args *environs.StartInstanceParams, spec *instances.InstanceSpec) error {
	if err := args.InstanceConfig.SetTools(args.Tools); err != nil {
		return errors.Trace(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, e.Config()); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// GetValidSubnetZoneMap ensures that (a single one of) any supplied space
// requirements are congruent and can be met, and that the representative
// subnet-zone map is returned, with Fan networks filtered out.
// The returned map will be nil if there are no space requirements.
func getValidSubnetZoneMap(ctx context.Context, args environs.StartInstanceParams) (map[network.Id][]string, error) {
	spaceCons := args.Constraints.IncludeSpaces()

	bindings := set.NewStrings()
	for _, spaceName := range args.EndpointBindings {
		bindings.Add(spaceName.String())
	}

	conCount := len(spaceCons)
	bindCount := len(bindings)

	// If there are no bindings or space constraints, we have no limitations
	// and should not have even received start arguments with a subnet/zone
	// mapping - just return nil and attempt provisioning in the current AZ.
	if conCount == 0 && bindCount == 0 {
		return nil, nil
	}

	sort.Strings(spaceCons)
	allSpaceReqs := bindings.Union(set.NewStrings(spaceCons...)).SortedValues()

	// We only need to validate if both bindings and constraints are present.
	// If one is supplied without the other, we know that the value for
	// args.SubnetsToZones correctly reflects the set of spaces.
	var indexInCommon int
	if conCount > 0 && bindCount > 0 {
		// If we have spaces in common between bindings and constraints,
		// the union count will be fewer than the sum.
		// If it is not, just error out here.
		if len(allSpaceReqs) == conCount+bindCount {
			return nil, errors.Errorf("unable to satisfy supplied space requirements; spaces: %v, bindings: %v",
				spaceCons, bindings.SortedValues())
		}

		// Now get the first index of the space in common.
		for _, conSpaceName := range spaceCons {
			if !bindings.Contains(conSpaceName) {
				continue
			}

			for i, spaceName := range allSpaceReqs {
				if conSpaceName == spaceName {
					indexInCommon = i
					break
				}
			}
		}
	}

	// TODO (manadart 2020-02-07): We only take a single subnet/zones
	// mapping to create a NIC for the instance.
	// This is behaviour that dates from the original spaces MVP.
	// It will not take too much effort to enable multi-NIC support for EC2
	// if we use them all when constructing the instance creation request.
	if conCount > 1 || bindCount > 1 {
		logger.Warningf(ctx, "only considering the space requirement for %q", allSpaceReqs[indexInCommon])
	}

	// We should always have a mapping if there are space requirements,
	// and it should always have the same length as the union of
	// constraints + bindings.
	// However unlikely, rather than taking this for granted and possibly
	// panicking, log a warning and let the provisioning continue.
	mappingCount := len(args.SubnetsToZones)
	if mappingCount == 0 || mappingCount <= indexInCommon {
		logger.Warningf(ctx,
			"got space requirements, but not a valid subnet-zone map; constraints/bindings not applied")
		return nil, nil
	}

	// Select the subnet-zone mapping at the index we determined minus Fan
	// networks which we can not consider for provisioning non-containers.
	// We know that the index determined from the spaces union corresponds
	// with the right mapping because of consistent sorting by the provisioner.
	subnetZones := make(map[network.Id][]string)
	for id, zones := range args.SubnetsToZones[indexInCommon] {
		if !network.IsInFanNetwork(id) {
			subnetZones[id] = zones
		}
	}

	return subnetZones, nil
}

func (e *environ) selectSubnetForInstance(ctx envcontext.ProviderCallContext,
	hasVPCID bool,
	subnetZones map[network.Id][]string,
	placementSubnetID network.Id,
	availabilityZone string,
) (types.Subnet, error) {
	var (
		subnets []types.Subnet
		err     error
	)
	if hasVPCID {
		subnets, err = e.selectVPCSubnetsForZone(ctx, subnetZones, placementSubnetID, availabilityZone)
		if err != nil {
			return types.Subnet{}, errors.Trace(err)
		}
	} else if availabilityZone != "" && len(subnetZones) > 0 {
		subnets, err = e.selectSubnetsForZone(ctx, subnetZones, placementSubnetID, availabilityZone)
		if err != nil {
			return types.Subnet{}, errors.Trace(err)
		}
	}

	preferredSubnets := []types.Subnet{}
	usableSubnets := []types.Subnet{}
	for _, subnet := range subnets {
		if subnet.State != types.SubnetStateAvailable {
			continue
		}
		usableSubnets = append(usableSubnets, subnet)
		if isDualStackSubnet(subnet) {
			preferredSubnets = append(preferredSubnets, subnet)
		}
	}

	if len(usableSubnets) == 0 {
		return types.Subnet{}, nil
	}

	// With multiple equally suitable subnets, picking one at random
	// will allow for better instance spread within the same zone, and
	// still work correctly if we happen to pick a constrained subnet
	// (we'll just treat this the same way we treat constrained zones
	// and retry).
	var subnet types.Subnet
	if len(preferredSubnets) != 0 {
		subnet = preferredSubnets[rand.Intn(len(preferredSubnets))]
		logger.Debugf(ctx,
			"selecting random preferred subnet %q from %d matching in zone %q",
			*subnet.SubnetId,
			len(preferredSubnets),
			availabilityZone,
		)
	} else {
		subnet = usableSubnets[rand.Intn(len(usableSubnets))]
		logger.Debugf(ctx,
			"selected random subnet %q from %d matching in zone %q",
			*subnet.SubnetId,
			len(usableSubnets),
			availabilityZone,
		)
	}

	return subnet, nil
}

func (e *environ) selectVPCSubnetsForZone(ctx envcontext.ProviderCallContext,
	subnetZones map[network.Id][]string,
	placementSubnetID network.Id,
	availabilityZone string,
) ([]types.Subnet, error) {
	var allowedSubnetIDs []network.Id
	if placementSubnetID != "" {
		allowedSubnetIDs = []network.Id{placementSubnetID}
	} else {
		for subnetID := range subnetZones {
			allowedSubnetIDs = append(allowedSubnetIDs, subnetID)
		}
	}

	subnets, err := getVPCSubnetsForAvailabilityZone(ctx,
		e.ec2Client, e.ecfg().vpcID(), availabilityZone, allowedSubnetIDs)

	switch {
	case isNotFoundError(err):
		return nil, errors.Trace(err)
	case err != nil:
		return nil, errors.Annotatef(e.HandleCredentialError(ctx, err), "getting subnets for zone %q", availabilityZone)
	}
	return subnets, nil
}

// selectSubnetIDsForZone selects a slice of subnets from a placement or
// availabilityZone.
// TODO (stickupkid): This could be lifted into core package as openstack has
// a very similar pattern to this.
func (e *environ) selectSubnetsForZone(
	ctx envcontext.ProviderCallContext,
	subnetZones map[network.Id][]string,
	placementSubnetID network.Id,
	availabilityZone string,
) ([]types.Subnet, error) {
	subnetIds, err := network.FindSubnetIDsForAvailabilityZone(availabilityZone, subnetZones)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(subnetIds) == 0 {
		return nil, errors.Errorf("availability zone %q has no subnets satisfying space constraints", availabilityZone)
	}

	// Use the placement to locate a subnet ID.
	if placementSubnetID != "" {
		asSet := network.MakeIDSet(subnetIds...)
		if !asSet.Contains(placementSubnetID) {
			return nil, errors.NotFoundf("subnets %q in AZ %q", placementSubnetID, availabilityZone)
		}
		subnetIds = []network.Id{placementSubnetID}
	}

	return subnetsForIDs(e.ec2Client, ctx, subnetIds)
}

func (e *environ) deriveAvailabilityZone(
	ctx envcontext.ProviderCallContext, args environs.StartInstanceParams,
) (string, error) {
	availabilityZone, _, err := e.deriveAvailabilityZoneAndSubnetID(ctx, args)
	return availabilityZone, errors.Trace(err)
}

func (e *environ) deriveAvailabilityZoneAndSubnetID(
	ctx envcontext.ProviderCallContext, args environs.StartInstanceParams,
) (string, network.Id, error) {
	// Determine the availability zones of existing volumes that are to be
	// attached to the machine. They must all match, and must be the same
	// as specified zone (if any).
	volumeAttachmentsZone, err := volumeAttachmentsZone(ctx, e.ec2Client, args.VolumeAttachments)
	if err != nil {
		return "", "", e.HandleCredentialError(ctx, err)
	}

	placementZone, placementSubnetID, err := e.instancePlacementZone(ctx, args.Placement, volumeAttachmentsZone)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	var availabilityZone string
	if placementZone != "" {
		availabilityZone = placementZone
	} else if args.AvailabilityZone != "" {
		// Validate and check state of the AvailabilityZone
		zones, err := e.AvailabilityZones(ctx)
		if err != nil {
			return "", "", err
		}
		for _, z := range zones {
			if z.Name() == args.AvailabilityZone {
				ec2AZ := z.(*ec2AvailabilityZone)
				zoneState := ec2AZ.AvailabilityZone.State
				if zoneState != types.AvailabilityZoneStateAvailable {
					return "", "", errors.Errorf(
						"availability zone %q is %q",
						z.Name(),
						zoneState,
					)
				} else {
					availabilityZone = args.AvailabilityZone
				}
				break
			}
		}
		if availabilityZone == "" {
			return "", "", errors.NotValidf("availability zone %q", availabilityZone)
		}
	}
	return availabilityZone, placementSubnetID, nil
}

func (e *environ) instancePlacementZone(ctx envcontext.ProviderCallContext, placement, volumeAttachmentsZone string) (zone string, subnet network.Id, _ error) {
	if placement == "" {
		return volumeAttachmentsZone, "", nil
	}
	var placementSubnetID network.Id
	instPlacement, err := e.parsePlacement(ctx, placement)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	zoneName := aws.ToString(instPlacement.availabilityZone.ZoneName)
	zoneState := instPlacement.availabilityZone.State
	if zoneState != types.AvailabilityZoneStateAvailable {
		return "", "", errors.Errorf(
			"availability zone %q is %q",
			zoneName,
			zoneState,
		)
	}
	if volumeAttachmentsZone != "" && volumeAttachmentsZone != zoneName {
		return "", "", errors.Errorf(
			"cannot create instance with placement %q, as this will prevent attaching the requested EBS volumes in zone %q",
			placement, volumeAttachmentsZone,
		)
	}
	if instPlacement.subnet != nil {
		subnetState := instPlacement.subnet.State
		if subnetState != types.SubnetStateAvailable {
			return "", "", errors.Errorf("subnet %q is %q",
				aws.ToString(instPlacement.subnet.CidrBlock), subnetState)
		}
		placementSubnetID = network.Id(aws.ToString(instPlacement.subnet.SubnetId))
	}
	return zoneName, placementSubnetID, nil
}

// volumeAttachmentsZone determines the availability zone for each volume
// identified in the volume attachment parameters, checking that they are
// all the same, and returns the availability zone name.
func volumeAttachmentsZone(ctx context.Context, e Client, attachments []storage.VolumeAttachmentParams) (string, error) {
	volumeIds := make([]string, 0, len(attachments))
	for _, a := range attachments {
		if a.Provider != EBS_ProviderType {
			continue
		}
		volumeIds = append(volumeIds, a.VolumeId)
	}
	if len(volumeIds) == 0 {
		return "", nil
	}
	resp, err := e.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: volumeIds,
	})
	if err != nil {
		return "", errors.Annotatef(err, "getting volume details (%s)", volumeIds)
	}
	if len(resp.Volumes) == 0 {
		return "", nil
	}
	for i, v := range resp.Volumes[1:] {
		zone1 := aws.ToString(resp.Volumes[i].AvailabilityZone)
		zone2 := aws.ToString(v.AvailabilityZone)
		if zone2 != zone1 {
			return "", errors.Errorf(
				"cannot attach volumes from multiple availability zones: %s is in %s, %s is in %s",
				aws.ToString(resp.Volumes[i].VolumeId), zone1, aws.ToString(v.VolumeId), zone2,
			)
		}
	}
	return aws.ToString(resp.Volumes[0].AvailabilityZone), nil
}

// tagResources calls ec2.CreateTags, tagging each of the specified resources
// with the given tags. tagResources will retry for a short period of time
// if it receives a *.NotFound error response from EC2.
func tagResources(ctx context.Context, e Client, tags map[string]string, resourceIds ...string) error {
	if len(tags) == 0 {
		return nil
	}
	ec2Tags := make([]types.Tag, 0, len(tags))
	for k, v := range tags {
		ec2Tags = append(ec2Tags, types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	retryStrategy := shortRetryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		return !strings.HasSuffix(ec2ErrCode(err), ".NotFound")
	}
	retryStrategy.Func = func() error {
		_, err := e.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: resourceIds,
			Tags:      ec2Tags,
		})
		return err
	}
	err := retry.Call(retryStrategy)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = retry.LastError(err)
	}
	return err
}

var runInstances = _runInstances

// runInstances calls ec2.RunInstances for a fixed number of attempts until
// RunInstances returns an error code that does not indicate an error that
// may be caused by eventual consistency.
func _runInstances(ctx context.Context, e Client, ri *ec2.RunInstancesInput, callback environs.StatusCallbackFunc) (*ec2.RunInstancesOutput, error) {
	var resp *ec2.RunInstancesOutput
	try := 1

	retryStrategy := shortRetryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		return !isNotFoundError(err)
	}
	retryStrategy.Func = func() error {
		_ = callback(ctx, status.Allocating, fmt.Sprintf("Start instance attempt %d", try), nil)
		var err error
		resp, err = e.RunInstances(ctx, ri)
		try++
		return err
	}
	err := retry.Call(retryStrategy)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = retry.LastError(err)
	}

	return resp, err
}

func (e *environ) StopInstances(ctx envcontext.ProviderCallContext, ids ...instance.Id) error {
	return errors.Trace(e.terminateInstances(ctx, ids))
}

// groupByName returns the security group with the given name.
func (e *environ) groupByName(ctx context.Context, groupName string) (types.SecurityGroup, error) {
	groups, err := e.securityGroupsByNameOrID(ctx, groupName)
	if err != nil {
		return types.SecurityGroup{}, e.HandleCredentialError(ctx, err)
	}

	if len(groups) != 1 {
		return types.SecurityGroup{}, errors.NewNotFound(fmt.Errorf(
			"expected one security group named %q, got %v",
			groupName, groups,
		), "")
	}
	return groups[0], nil
}

// isNotFoundError returns whether err is a typed NotFoundError or an EC2 error
// code for "group not found", indicating no matching instances (as they are
// filtered by group).
func isNotFoundError(err error) bool {
	return err != nil && (errors.Is(err, errors.NotFound) || ec2ErrCode(err) == "InvalidGroup.NotFound")
}

// Instances is part of the environs.Environ interface.
func (e *environ) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]instances.Instance, len(ids))
	// Make a series of requests to cope with eventual consistency.
	// Each request will attempt to add more instances to the requested
	// set.
	retryStrategy := shortRetryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		return err != environs.ErrPartialInstances
	}
	retryStrategy.Func = func() error {
		var need []string
		for i, inst := range insts {
			if inst == nil {
				need = append(need, string(ids[i]))
			}
		}
		filters := []types.Filter{
			makeFilter("instance-state-name", aliveInstanceStates...),
			makeFilter("instance-id", need...),
			makeModelFilter(e.uuid()),
		}
		err := e.gatherInstances(ctx, ids, insts, filters)
		if err != nil {
			return e.HandleCredentialError(ctx, err)
		}
		return nil
	}
	err := retry.Call(retryStrategy)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = retry.LastError(err)
	}

	if errors.Is(err, environs.ErrPartialInstances) {
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

// gatherInstances tries to get information on each instance
// id whose corresponding insts slot is nil.
//
// This function returns environs.ErrPartialInstances if the
// insts slice has not been completely filled.
func (e *environ) gatherInstances(
	ctx context.Context,
	ids []instance.Id,
	insts []instances.Instance,
	filters []types.Filter,
) error {
	resp, err := e.ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: filters,
	})
	if err != nil {
		return err
	}
	n := 0
	// For each requested id, add it to the returned instances
	// if we find it in the response.
	for i, id := range ids {
		if insts[i] != nil {
			n++
			continue
		}
		for _, r := range resp.Reservations {
			for _, inst := range r.Instances {
				if *inst.InstanceId != string(id) {
					continue
				}
				insts[i] = &sdkInstance{e: e, i: inst}
				n++
			}
		}
	}
	if n < len(ids) {
		return environs.ErrPartialInstances
	}
	return nil
}

// NetworkInterfaces implements NetworkingEnviron.NetworkInterfaces.
func (e *environ) NetworkInterfaces(ctx envcontext.ProviderCallContext, ids []instance.Id) ([]network.InterfaceInfos, error) {
	switch len(ids) {
	case 0:
		return nil, environs.ErrNoInstances
	case 1: // short-cut for single instance
		ifList, err := e.networkInterfacesForInstance(ctx, ids[0])
		if err != nil {
			return nil, err
		}
		return []network.InterfaceInfos{ifList}, nil
	}

	// Collect all available subnets into a map where keys are subnet IDs
	// and values are subnets. We will use this map to resolve subnets
	// for the bulk network interface info requests below.
	subMap, err := e.subnetMap(ctx)
	if err != nil {
		return nil, errors.Annotate(e.HandleCredentialError(ctx, err), "failed to retrieve subnet info")
	}

	infos := make([]network.InterfaceInfos, len(ids))
	idToInfosIndex := make(map[string]int)
	for idx, id := range ids {
		idToInfosIndex[string(id)] = idx
	}

	// Make a series of requests to cope with eventual consistency.  Each
	// request will attempt to add more network interface queries to the
	// requested set till we eventually obtain the full set of data.
	retryStrategy := shortRetryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		return err != environs.ErrPartialInstances
	}
	retryStrategy.Func = func() error {
		var need []string
		for idx, info := range infos {
			if info == nil {
				need = append(need, string(ids[idx]))
			}
		}

		// Network interfaces are not currently tagged so we cannot
		// use a model filter here.
		filter := makeFilter("attachment.instance-id", need...)
		logger.Tracef(ctx, "retrieving NICs for instances %v", need)
		return e.gatherNetworkInterfaceInfo(ctx, filter, infos, idToInfosIndex, subMap)
	}
	err = retry.Call(retryStrategy)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = retry.LastError(err)
	}

	if err == environs.ErrPartialInstances {
		for _, info := range infos {
			if info != nil {
				return infos, environs.ErrPartialInstances
			}
		}
		return nil, environs.ErrNoInstances
	}
	if err != nil {
		return nil, err
	}
	return infos, nil
}

// subnetMap returns a map with all known ec2.Subnets and their IDs as keys.
func (e *environ) subnetMap(ctx envcontext.ProviderCallContext) (map[string]types.Subnet, error) {
	subnetsResp, err := e.ec2Client.DescribeSubnets(ctx, nil)
	if err != nil {
		return nil, err
	}

	subMap := make(map[string]types.Subnet, len(subnetsResp.Subnets))
	for _, sub := range subnetsResp.Subnets {
		subMap[aws.ToString(sub.SubnetId)] = sub
	}
	return subMap, nil
}

// gatherNetworkInterfaceInfo executes a filtered network interface lookup,
// parses the results and appends them to the correct infos slot based on
// the attachment instance ID information for each result.
//
// This method returns environs.ErrPartialInstances if the infos slice contains
// any nil entries.
func (e *environ) gatherNetworkInterfaceInfo(
	ctx envcontext.ProviderCallContext,
	filter types.Filter,
	infos []network.InterfaceInfos,
	idToInfosIndex map[string]int,
	subMap map[string]types.Subnet,
) error {
	// Check how many queries have already been answered; machines must
	// have at least one network interface attached to them.
	pending := len(infos)
	for _, info := range infos {
		if len(info) != 0 {
			pending--
		}
	}

	// Run query
	networkInterfacesResp, err := e.ec2Client.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
		Filters: []types.Filter{filter},
	})
	if err != nil {
		return e.HandleCredentialError(ctx, err)
	}

	for _, netIfSpec := range networkInterfacesResp.NetworkInterfaces {
		idx, found := idToInfosIndex[aws.ToString(netIfSpec.Attachment.InstanceId)]
		if !found {
			continue
		} else if infos[idx] == nil {
			// This is the first (and perhaps only) interface that
			// we obtained for this instance. Decrement the number
			// of pending queries.
			pending--
		}

		subnet, found := subMap[aws.ToString(netIfSpec.SubnetId)]
		if !found {
			return errors.NotFoundf("info for subnet %q", netIfSpec.SubnetId)
		}

		infos[idx] = append(infos[idx], mapNetworkInterface(netIfSpec, subnet))
	}

	if pending != 0 {
		return environs.ErrPartialInstances
	}
	return nil
}

func (e *environ) networkInterfacesForInstance(ctx envcontext.ProviderCallContext, instId instance.Id) (network.InterfaceInfos, error) {
	var resp *ec2.DescribeNetworkInterfacesOutput

	abortRetries := make(chan struct{}, 1)
	defer close(abortRetries)

	retryStrategy := shortRetryStrategy
	retryStrategy.Stop = abortRetries
	retryStrategy.IsFatalError = func(err error) bool {
		return errors.Is(err, common.ErrorCredentialNotValid)
	}
	retryStrategy.NotifyFunc = func(lastError error, attempt int) {
		logger.Errorf(ctx, "failed to get instance %q interfaces: %v (retrying)", instId, lastError)
	}
	retryStrategy.Func = func() error {
		logger.Tracef(ctx, "retrieving NICs for instance %q", instId)
		filter := makeFilter("attachment.instance-id", string(instId))

		var err error
		resp, err = e.ec2Client.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
			Filters: []types.Filter{filter},
		})
		if err != nil {
			return e.HandleCredentialError(ctx, err)
		}
		if len(resp.NetworkInterfaces) == 0 {
			msg := fmt.Sprintf("instance %q has no NIC attachment yet, retrying...", instId)
			logger.Tracef(ctx, "%s", msg)
			return errors.New(msg)
		}
		if logger.IsLevelEnabled(corelogger.TRACE) {
			logger.Tracef(ctx, "found instance %q NICs: %s", instId, pretty.Sprint(resp.NetworkInterfaces))
		}
		return nil
	}
	err := retry.Call(retryStrategy)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = retry.LastError(err)
	}
	if err != nil {
		// either the instance doesn't exist or we couldn't get through to
		// the ec2 api
		return nil, errors.Annotatef(err, "cannot get instance %q network interfaces", instId)
	}
	ec2Interfaces := resp.NetworkInterfaces
	result := make(network.InterfaceInfos, len(ec2Interfaces))
	for i, iface := range ec2Interfaces {
		resp, err := e.ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			SubnetIds: []string{aws.ToString(iface.SubnetId)},
		})
		if err != nil {
			return nil, errors.Annotatef(e.HandleCredentialError(ctx, err), "failed to retrieve subnet %q info", aws.ToString(iface.SubnetId))
		}
		if len(resp.Subnets) != 1 {
			return nil, errors.Errorf("expected 1 subnet, got %d", len(resp.Subnets))
		}

		result[i] = mapNetworkInterface(iface, resp.Subnets[0])
	}
	return result, nil
}

func mapNetworkInterface(iface types.NetworkInterface, subnet types.Subnet) network.InterfaceInfo {
	privateAddress := aws.ToString(iface.PrivateIpAddress)
	subnetCIDR := aws.ToString(subnet.CidrBlock)
	// Device names and VLAN tags are not returned by EC2.
	ni := network.InterfaceInfo{
		DeviceIndex:       int(aws.ToInt32(iface.Attachment.DeviceIndex)),
		MACAddress:        aws.ToString(iface.MacAddress),
		ProviderId:        network.Id(aws.ToString(iface.NetworkInterfaceId)),
		ProviderSubnetId:  network.Id(aws.ToString(iface.SubnetId)),
		AvailabilityZones: []string{aws.ToString(iface.AvailabilityZone)},
		Disabled:          false,
		NoAutoStart:       false,
		InterfaceType:     network.EthernetDevice,
		// The describe interface responses that we get back from EC2
		// define a *list* of private IP addresses with one entry that
		// is tagged as primary and whose value is encoded in the
		// "PrivateIPAddress" field. The code below arranges so that
		// the primary IP is always added first with any additional
		// private IPs appended after it.
		Addresses: network.ProviderAddresses{network.NewMachineAddress(
			privateAddress,
			network.WithScope(network.ScopeCloudLocal),
			network.WithCIDR(subnetCIDR),
			network.WithConfigType(network.ConfigDHCP),
		).AsProviderAddress()},
		Origin: network.OriginProvider,
	}

	for _, privAddr := range iface.PrivateIpAddresses {
		if privAddr.Association != nil {
			if ip := aws.ToString(privAddr.Association.PublicIp); ip != "" {
				ni.ShadowAddresses = append(ni.ShadowAddresses, network.NewMachineAddress(
					ip,
					network.WithScope(network.ScopePublic),
					network.WithConfigType(network.ConfigDHCP),
				).AsProviderAddress())
			}
		}

		if aws.ToString(privAddr.PrivateIpAddress) == privateAddress {
			continue // primary address has already been added.
		}

		// An EC2 interface is connected to a single subnet,
		// so we assume other addresses are in the same subnet.
		ni.Addresses = append(ni.Addresses, network.NewMachineAddress(
			privateAddress,
			network.WithScope(network.ScopeCloudLocal),
			network.WithCIDR(subnetCIDR),
			network.WithConfigType(network.ConfigDHCP),
		).AsProviderAddress())
	}

	return ni
}

func makeSubnetInfo(
	ctx context.Context,
	cidr string, subnetId, providerNetworkId network.Id, availZones []string,
) (network.SubnetInfo, error) {
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return network.SubnetInfo{}, errors.Annotatef(err, "skipping subnet %q, invalid CIDR", cidr)
	}

	info := network.SubnetInfo{
		CIDR:              cidr,
		ProviderId:        subnetId,
		ProviderNetworkId: providerNetworkId,
		VLANTag:           0, // Not supported on EC2
		AvailabilityZones: availZones,
	}
	logger.Tracef(ctx, "found subnet with info %#v", info)
	return info, nil

}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance or list of ids. subnetIds can be
// empty, in which case all known are returned. Implements
// NetworkingEnviron.Subnets.
func (e *environ) Subnets(
	ctx envcontext.ProviderCallContext, instId instance.Id, subnetIds []network.Id,
) ([]network.SubnetInfo, error) {
	var results []network.SubnetInfo
	subIdSet := make(map[string]bool)
	for _, subId := range subnetIds {
		subIdSet[string(subId)] = false
	}

	if instId != instance.UnknownId {
		interfaces, err := e.networkInterfacesForInstance(ctx, instId)
		if err != nil {
			return results, errors.Trace(err)
		}
		if len(subnetIds) == 0 {
			for _, iface := range interfaces {
				subIdSet[string(iface.ProviderSubnetId)] = false
			}
		}
		for _, iface := range interfaces {
			_, ok := subIdSet[string(iface.ProviderSubnetId)]
			if !ok {
				logger.Tracef(ctx, "subnet %q not in %v, skipping", iface.ProviderSubnetId, subnetIds)
				continue
			}
			subIdSet[string(iface.ProviderSubnetId)] = true
			info, err := makeSubnetInfo(
				ctx,
				iface.PrimaryAddress().CIDR, iface.ProviderSubnetId, iface.ProviderNetworkId, iface.AvailabilityZones)
			if err != nil {
				// Error will already have been logged.
				continue
			}
			results = append(results, info)
		}
	} else {
		subnets, _, err := e.subnetsForVPC(ctx)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to retrieve subnets")
		}
		if len(subnetIds) == 0 {
			for _, subnet := range subnets {
				subIdSet[aws.ToString(subnet.SubnetId)] = false
			}
		}

		for _, subnet := range subnets {
			subnetID := aws.ToString(subnet.SubnetId)
			_, ok := subIdSet[subnetID]
			if !ok {
				logger.Tracef(ctx, "subnet %q not in %v, skipping", subnetID, subnetIds)
				continue
			}
			subIdSet[subnetID] = true
			cidr := aws.ToString(subnet.CidrBlock)
			info, err := makeSubnetInfo(
				ctx,
				cidr, network.Id(subnetID), network.Id(aws.ToString(subnet.VpcId)), []string{aws.ToString(subnet.AvailabilityZone)})
			if err != nil {
				// Error will already have been logged.
				continue
			}
			results = append(results, info)

		}
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

func (e *environ) subnetsForVPC(ctx envcontext.ProviderCallContext) ([]types.Subnet, string, error) {
	vpcId := e.ecfg().vpcID()
	if !isVPCIDSet(vpcId) {
		if hasDefaultVPC, err := e.hasDefaultVPC(ctx); err == nil && hasDefaultVPC {
			vpcId = aws.ToString(e.defaultVPC.VpcId)
		}
	}
	filter := makeFilter("vpc-id", vpcId)
	resp, err := e.ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{filter},
	})
	if err != nil {
		return nil, "", e.HandleCredentialError(ctx, err)
	}
	return resp.Subnets, vpcId, nil
}

// AdoptResources is part of the Environ interface.
func (e *environ) AdoptResources(ctx envcontext.ProviderCallContext, controllerUUID string, fromVersion semversion.Number) error {
	// Gather resource ids for instances, volumes and security groups tagged with this model.
	instances, err := e.AllInstances(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	// We want to update the controller tags on root disks even though
	// they are destroyed automatically with the instance they're
	// attached to.
	volumeIds, err := e.allModelVolumes(ctx, true)
	if err != nil {
		return errors.Trace(err)
	}
	groups, err := e.modelSecurityGroups(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	resourceIds := make([]string, len(instances))
	for i, instance := range instances {
		resourceIds[i] = instance.Id().String()
	}
	groupIds := make([]string, len(groups))
	for i, g := range groups {
		groupIds[i] = aws.ToString(g.GroupId)
	}
	resourceIds = append(resourceIds, volumeIds...)
	resourceIds = append(resourceIds, groupIds...)

	tags := map[string]string{tags.JujuController: controllerUUID}
	if err := tagResources(ctx, e.ec2Client, tags, resourceIds...); err != nil {
		return errors.Annotate(e.HandleCredentialError(ctx, err), "updating tags")
	}
	return nil
}

// AllInstances is part of the environs.InstanceBroker interface.
func (e *environ) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	// We want to return everything we find here except for instances that are
	// "shutting-down" - they are on the way to be terminated - or already "terminated".
	// From https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-lifecycle.html
	return e.allInstancesByState(ctx, activeStates.Values()...)
}

// AllRunningInstances is part of the environs.InstanceBroker interface.
func (e *environ) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	return e.allInstancesByState(ctx, aliveInstanceStates...)
}

// allInstancesByState returns all instances in the environment
// with one of the specified instance states.
func (e *environ) allInstancesByState(ctx context.Context, states ...string) ([]instances.Instance, error) {
	filters := []types.Filter{
		makeFilter("instance-state-name", states...),
		makeModelFilter(e.uuid()),
	}
	return e.allInstances(ctx, filters)
}

// ControllerInstances is part of the environs.Environ interface.
func (e *environ) ControllerInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	filters := []types.Filter{
		makeFilter("instance-state-name", aliveInstanceStates...),
		makeFilter(fmt.Sprintf("tag:%s", tags.JujuIsController), "true"),
		makeControllerFilter(controllerUUID),
	}
	ids, err := e.allInstanceIDs(ctx, filters)
	if err != nil {
		return nil, errors.Trace(e.HandleCredentialError(ctx, err))
	}
	if len(ids) == 0 {
		return nil, environs.ErrNotBootstrapped
	}
	return ids, nil
}

func makeFilter(name string, values ...string) types.Filter {
	filter := types.Filter{
		Name:   &name,
		Values: values,
	}
	return filter
}

// allControllerManagedInstances returns the IDs of all instances managed by
// this environment's controller.
//
// Note that this requires that all instances are tagged; we cannot filter on
// security groups, as we do not know the names of the models.
func (e *environ) allControllerManagedInstances(ctx envcontext.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	filters := []types.Filter{
		makeFilter("instance-state-name", aliveInstanceStates...),
		makeControllerFilter(controllerUUID),
	}
	return e.allInstanceIDs(ctx, filters)
}

func (e *environ) allInstanceIDs(ctx envcontext.ProviderCallContext, filters []types.Filter) ([]instance.Id, error) {
	insts, err := e.allInstances(ctx, filters)
	if err != nil {
		return nil, errors.Trace(e.HandleCredentialError(ctx, err))
	}
	ids := make([]instance.Id, len(insts))
	for i, inst := range insts {
		ids[i] = inst.Id()
	}
	return ids, nil
}

func (e *environ) allInstances(ctx context.Context, filters []types.Filter) ([]instances.Instance, error) {
	resp, err := e.ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: filters,
	})
	if err != nil {
		return nil, errors.Annotate(e.HandleCredentialError(ctx, err), "listing instances")
	}
	var insts []instances.Instance
	for _, r := range resp.Reservations {
		for _, inst := range r.Instances {
			insts = append(insts, &sdkInstance{e: e, i: inst})
		}
	}
	return insts, nil
}

// Destroy is part of the environs.Environ interface.
func (e *environ) Destroy(ctx envcontext.ProviderCallContext) error {
	if err := common.Destroy(e, ctx); err != nil {
		return errors.Trace(e.HandleCredentialError(ctx, err))
	}
	if err := e.cleanModelSecurityGroups(ctx); err != nil {
		return errors.Annotate(e.HandleCredentialError(ctx, err), "cannot delete model security groups")
	}
	return nil
}

// DestroyController implements the Environ interface.
func (e *environ) DestroyController(ctx envcontext.ProviderCallContext, controllerUUID string) error {
	// In case any hosted environment hasn't been cleaned up yet,
	// we also attempt to delete their resources when the controller
	// environment is destroyed.
	if err := e.destroyControllerManagedModels(ctx, controllerUUID); err != nil {
		return errors.Annotate(err, "destroying managed models")
	}
	return e.Destroy(ctx)
}

// destroyControllerManagedModels destroys all models managed by this
// model's controller.
func (e *environ) destroyControllerManagedModels(ctx envcontext.ProviderCallContext, controllerUUID string) error {
	// Terminate all instances managed by the controller.
	instIds, err := e.allControllerManagedInstances(ctx, controllerUUID)
	if err != nil {
		return errors.Annotate(err, "listing instances")
	}
	if err := e.terminateInstances(ctx, instIds); err != nil {
		return errors.Annotate(err, "terminating instances")
	}

	// Delete all volumes managed by the controller. (No need to delete root disks manually.)
	volIds, err := e.allControllerManagedVolumes(ctx, controllerUUID, false)
	if err != nil {
		return errors.Annotate(err, "listing volumes")
	}
	errs := foreachVolume(ctx, e.ec2Client, volIds, destroyVolume)
	for i, err := range errs {
		if err == nil {
			continue
		}
		// (anastasiamac 2018-03-21) This is strange - we do try
		// to destroy all volumes but afterwards, if we have encountered any errors,
		// we will return first one...The same logic happens on detach..?...
		return errors.Annotatef(e.HandleCredentialError(ctx, err), "destroying volume %q", volIds[i])
	}

	// Delete security groups managed by the controller.
	groups, err := e.controllerSecurityGroups(ctx, controllerUUID)
	if err != nil {
		return errors.Trace(err)
	}
	for _, g := range groups {
		if err := deleteSecurityGroupInsistently(ctx, e.ec2Client, g, clock.WallClock); err != nil {
			return errors.Trace(e.HandleCredentialError(ctx, err))
		}
	}

	instanceProfiles, err := listInstanceProfilesForController(ctx, e.iamClient, controllerUUID)
	if errors.Is(err, errors.Unauthorized) {
		logger.Warningf(ctx, "unable to list Instance Profiles for deletion, Instance Profiles may have to be manually cleaned up for controller %q", controllerUUID)
	} else if err != nil {
		return errors.Annotatef(err, "listing instance profiles for controller uuid %q", controllerUUID)
	}

	for _, ip := range instanceProfiles {
		err := deleteInstanceProfile(ctx, e.iamClient, ip)
		if err != nil {
			return errors.Annotatef(err, "deleting instance profile %q for controller uuid %q", *ip.InstanceProfileName, controllerUUID)
		}
	}

	roles, err := listRolesForController(ctx, e.iamClient, controllerUUID)
	if errors.Is(err, errors.Unauthorized) {
		logger.Warningf(ctx, "unable to list Roles for deletion, Roles may have to be manually cleaned up for controller %q", controllerUUID)
	} else if err != nil {
		return errors.Annotatef(err, "listing roles for controller uuid %q", controllerUUID)
	}

	for _, role := range roles {
		err := deleteRole(ctx, e.iamClient, *role.RoleName)
		if err != nil {
			return errors.Annotatef(err, "deleting role %q as part of controller uuid %q", *role.RoleName, controllerUUID)
		}
	}

	return nil
}

func (e *environ) allControllerManagedVolumes(ctx envcontext.ProviderCallContext, controllerUUID string, includeRootDisks bool) ([]string, error) {
	volumes, err := listVolumes(ctx, e.ec2Client, includeRootDisks, makeControllerFilter(controllerUUID))
	if err != nil {
		return nil, errors.Trace(e.HandleCredentialError(ctx, err))
	}
	return volumes, nil
}

func (e *environ) allModelVolumes(ctx envcontext.ProviderCallContext, includeRootDisks bool) ([]string, error) {
	volumes, err := listVolumes(ctx, e.ec2Client, includeRootDisks, makeModelFilter(e.uuid()))
	if err != nil {
		return nil, errors.Trace(e.HandleCredentialError(ctx, err))
	}
	return volumes, nil
}

func rulesToIPPerms(rules firewall.IngressRules) []types.IpPermission {
	ipPerms := make([]types.IpPermission, len(rules))
	for i, r := range rules {
		ipPerms[i] = types.IpPermission{
			IpProtocol: aws.String(r.PortRange.Protocol),
			FromPort:   aws.Int32(int32(r.PortRange.FromPort)),
			ToPort:     aws.Int32(int32(r.PortRange.ToPort)),
		}
		if len(r.SourceCIDRs) == 0 {
			ipPerms[i].IpRanges = []types.IpRange{{CidrIp: aws.String(defaultRouteIpv4CIDRBlock), Description: ipRangeDescription(r, defaultRouteIpv4CIDRBlock)}}
			ipPerms[i].Ipv6Ranges = []types.Ipv6Range{{CidrIpv6: aws.String(defaultRouteIPv6CIDRBlock), Description: ipRangeDescription(r, defaultRouteIPv6CIDRBlock)}}
		} else {
			for _, cidr := range r.SourceCIDRs.SortedValues() {
				// CIDRs are pre-validated; if an invalid CIDR
				// reaches this loop, it will be skipped.
				addrType, _ := network.CIDRAddressType(cidr)
				if addrType == network.IPv4Address {
					ipPerms[i].IpRanges = append(ipPerms[i].IpRanges, types.IpRange{CidrIp: aws.String(cidr), Description: ipRangeDescription(r, cidr)})
				} else if addrType == network.IPv6Address {
					ipPerms[i].Ipv6Ranges = append(ipPerms[i].Ipv6Ranges, types.Ipv6Range{CidrIpv6: aws.String(cidr), Description: ipRangeDescription(r, cidr)})
				}
			}
		}
	}
	return ipPerms
}

func ipRangeDescription(rule firewall.IngressRule, cidr string) *string {
	if cidr == "" || cidr == firewall.AllNetworksIPV4CIDR || cidr == firewall.AllNetworksIPV6CIDR {
		return aws.String(fmt.Sprintf("juju ingress to %s", rule.PortRange))
	}
	return aws.String(fmt.Sprintf("juju ingress to %s from %s", rule.PortRange, cidr))

}

func (e *environ) openPortsInGroup(ctx envcontext.ProviderCallContext, name string, rules firewall.IngressRules) error {
	if len(rules) == 0 {
		return nil
	}
	// Give permissions for anyone to access the given ports.
	g, err := e.groupByName(ctx, name)
	if err != nil {
		return err
	}
	ipPerms := rulesToIPPerms(rules)
	_, err = e.ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       g.GroupId,
		IpPermissions: ipPerms,
	})
	if err != nil && ec2ErrCode(err) == "InvalidPermission.Duplicate" {
		if len(rules) == 1 {
			return nil
		}
		// If there's more than one port and we get a duplicate error,
		// then we go through authorizing each port individually,
		// otherwise the ports that were *not* duplicates will have
		// been ignored
		for i := range ipPerms {
			_, err := e.ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
				GroupId:       g.GroupId,
				IpPermissions: ipPerms[i : i+1],
			})
			if err != nil && ec2ErrCode(err) != "InvalidPermission.Duplicate" {
				return errors.Annotatef(e.HandleCredentialError(ctx, err), "cannot open port %v", ipPerms[i])
			}
		}
		return nil
	}
	if err != nil {
		return errors.Annotate(e.HandleCredentialError(ctx, err), "cannot open ports")
	}
	return nil
}

func (e *environ) closePortsInGroup(ctx envcontext.ProviderCallContext, name string, rules firewall.IngressRules) error {
	if len(rules) == 0 {
		return nil
	}
	// Revoke permissions for anyone to access the given ports.
	// Note that ec2 allows the revocation of permissions that aren't
	// granted, so this is naturally idempotent.
	g, err := e.groupByName(ctx, name)
	if err != nil {
		return err
	}
	_, err = e.ec2Client.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
		GroupId:       g.GroupId,
		IpPermissions: rulesToIPPerms(rules),
	})
	if err != nil {
		return errors.Annotate(e.HandleCredentialError(ctx, err), "cannot close ports")
	}
	return nil
}

func (e *environ) ingressRulesInGroup(ctx envcontext.ProviderCallContext, name string) (rules firewall.IngressRules, err error) {
	group, err := e.groupByName(ctx, name)
	if err != nil {
		return nil, err
	}
	for _, p := range group.IpPermissions {
		if len(p.UserIdGroupPairs) == 1 && aws.ToString(p.UserIdGroupPairs[0].GroupId) == aws.ToString(group.GroupId) {
			// hide internal sec group rules
			continue
		}
		var sourceCIDRs []string
		for _, r := range p.IpRanges {
			sourceCIDRs = append(sourceCIDRs, aws.ToString(r.CidrIp))
		}
		for _, r := range p.Ipv6Ranges {
			sourceCIDRs = append(sourceCIDRs, aws.ToString(r.CidrIpv6))
		}
		if len(sourceCIDRs) == 0 {
			sourceCIDRs = append(sourceCIDRs, defaultRouteIpv4CIDRBlock)
			sourceCIDRs = append(sourceCIDRs, defaultRouteIPv6CIDRBlock)
		}
		portRange := network.PortRange{
			Protocol: aws.ToString(p.IpProtocol),
			FromPort: int(aws.ToInt32(p.FromPort)),
			ToPort:   int(aws.ToInt32(p.ToPort)),
		}
		if portRange.Protocol == "icmpv6" {
			// For now we need to filter out the icmpv6 rules as they are complicated and not
			// represented well in the juju model.
			continue
		}
		rules = append(rules, firewall.NewIngressRule(portRange, sourceCIDRs...))
	}
	if err := rules.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	rules.Sort()
	return rules, nil
}

func (e *environ) OpenPorts(ctx envcontext.ProviderCallContext, rules firewall.IngressRules) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return errors.Errorf("invalid firewall mode %q for opening ports on model", e.Config().FirewallMode())
	}
	if err := e.openPortsInGroup(ctx, e.globalGroupName(), rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof(ctx, "opened ports in global group: %v", rules)
	return nil
}

func (e *environ) ClosePorts(ctx envcontext.ProviderCallContext, rules firewall.IngressRules) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return errors.Errorf("invalid firewall mode %q for closing ports on model", e.Config().FirewallMode())
	}
	if err := e.closePortsInGroup(ctx, e.globalGroupName(), rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof(ctx, "closed ports in global group: %v", rules)
	return nil
}

func (e *environ) IngressRules(ctx envcontext.ProviderCallContext) (firewall.IngressRules, error) {
	if e.Config().FirewallMode() != config.FwGlobal {
		return nil, errors.Errorf("invalid firewall mode %q for retrieving ingress rules from model", e.Config().FirewallMode())
	}
	return e.ingressRulesInGroup(ctx, e.globalGroupName())
}

func (e *environ) OpenModelPorts(ctx envcontext.ProviderCallContext, rules firewall.IngressRules) error {
	if err := e.openPortsInGroup(ctx, e.jujuGroupName(), rules); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (e *environ) CloseModelPorts(ctx envcontext.ProviderCallContext, rules firewall.IngressRules) error {
	if err := e.closePortsInGroup(ctx, e.jujuGroupName(), rules); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (e *environ) ModelIngressRules(ctx envcontext.ProviderCallContext) (firewall.IngressRules, error) {
	return e.ingressRulesInGroup(ctx, e.jujuGroupName())
}

func (*environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

func (e *environ) instanceSecurityGroups(ctx envcontext.ProviderCallContext, instIDs []instance.Id, states ...string) ([]types.GroupIdentifier, error) {
	strInstID := make([]string, len(instIDs))
	for i := range instIDs {
		strInstID[i] = string(instIDs[i])
	}

	var filter []types.Filter
	if len(states) > 0 {
		filter = append(filter, makeFilter("instance-state-name", states...))
	}

	resp, err := e.ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: strInstID,
		Filters:     filter,
	})
	if err != nil {
		return nil, errors.Annotatef(e.HandleCredentialError(ctx, err), "cannot retrieve instance information from aws to delete security groups")
	}

	var securityGroups []types.GroupIdentifier
	for _, res := range resp.Reservations {
		for _, inst := range res.Instances {
			logger.Debugf(ctx, "instance %q has security groups %s", aws.ToString(inst.InstanceId), pretty.Sprint(inst.SecurityGroups))
			securityGroups = append(securityGroups, inst.SecurityGroups...)
		}
	}
	return securityGroups, nil
}

// controllerSecurityGroups returns the details of all security groups managed
// by the environment's controller.
func (e *environ) controllerSecurityGroups(ctx envcontext.ProviderCallContext, controllerUUID string) ([]types.GroupIdentifier, error) {
	return e.querySecurityGroups(ctx, makeControllerFilter(controllerUUID))
}

func (e *environ) modelSecurityGroups(ctx envcontext.ProviderCallContext) ([]types.GroupIdentifier, error) {
	return e.querySecurityGroups(ctx, makeModelFilter(e.uuid()))
}

func (e *environ) querySecurityGroups(ctx envcontext.ProviderCallContext, filter types.Filter) ([]types.GroupIdentifier, error) {
	resp, err := e.ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []types.Filter{filter},
	})
	if err != nil {
		return nil, errors.Annotate(e.HandleCredentialError(ctx, err), "listing security groups")
	}
	groups := make([]types.GroupIdentifier, len(resp.SecurityGroups))
	for i, g := range resp.SecurityGroups {
		groups[i] = types.GroupIdentifier{
			GroupId:   g.GroupId,
			GroupName: g.GroupName,
		}
	}
	return groups, nil
}

// cleanModelSecurityGroups attempts to delete all security groups owned
// by the model. These include any security groups belonging to instances
// in the model which may not have been cleaned up.
func (e *environ) cleanModelSecurityGroups(ctx envcontext.ProviderCallContext) error {
	// Delete security groups managed by the model.
	groups, err := e.modelSecurityGroups(ctx)
	if err != nil {
		return errors.Annotatef(err, "cannot retrieve security groups for model %q", e.uuid())
	}
	for _, g := range groups {
		logger.Debugf(ctx, "deleting model security group %q (%q)", aws.ToString(g.GroupName), aws.ToString(g.GroupId))
		if err := deleteSecurityGroupInsistently(ctx, e.ec2Client, g, clock.WallClock); err != nil {
			return errors.Trace(e.HandleCredentialError(ctx, err))
		}
	}
	return nil
}

var (
	activeStates = set.NewStrings(
		"rebooting", "pending", "running", "stopping", "stopped")
	terminatingStates = set.NewStrings(
		"shutting-down", "terminated")
)

func (e *environ) terminateInstances(ctx envcontext.ProviderCallContext, ids []instance.Id) error {
	if len(ids) == 0 {
		return nil
	}

	// TODO (anastasiamac 2016-04-11) Err if instances still have resources hanging around.
	// LP#1568654
	defer func() {
		e.deleteSecurityGroupsForInstances(ctx, ids)
	}()

	// TODO (anastasiamac 2016-04-7) instance termination would benefit
	// from retry with exponential delay just like security groups
	// in defer. Bug#1567179.
	abortRetries := make(chan struct{}, 1)
	defer close(abortRetries)

	retryStrategy := shortRetryStrategy
	retryStrategy.Stop = abortRetries
	retryStrategy.Func = func() error {
		resp, err := terminateInstancesById(ctx, e.ec2Client, ids...)
		if err == nil {
			for i, sc := range resp {
				if !terminatingStates.Contains(string(sc.CurrentState.Name)) {
					logger.Warningf(ctx, "instance %d has been terminated but is in state %q", ids[i], sc.CurrentState.Name)
				}
			}
		}
		if err == nil || ec2ErrCode(err) != "InvalidInstanceID.NotFound" {
			// This will return either success at terminating all instances (1st condition) or
			// encountered error as long as it's not NotFound (2nd condition).
			abortRetries <- struct{}{}
			return e.HandleCredentialError(ctx, err)
		}
		return err
	}
	err := retry.Call(retryStrategy)
	if err != nil && retry.IsRetryStopped(err) {
		err = retry.LastError(err)
		return err
	}
	// We will get here only if we got a NotFound error.
	// 1. If we attempted to terminate only one instance was, return now.
	if len(ids) == 1 {
		ids = nil
		return nil
	}
	// 2. If we attempted to terminate several instances and got a NotFound error,
	// it means that no instances were terminated.
	// So try each instance individually, ignoring a NotFound error this time.
	deletedIDs := []instance.Id{}
	for _, id := range ids {
		resp, err := terminateInstancesById(ctx, e.ec2Client, id)
		if err == nil {
			scName := string(resp[0].CurrentState.Name)
			if !terminatingStates.Contains(scName) {
				logger.Warningf(ctx, "instance %d has been terminated but is in state %q", id, scName)
			}
			deletedIDs = append(deletedIDs, id)
		}
		if err != nil && ec2ErrCode(err) != "InvalidInstanceID.NotFound" {
			ids = deletedIDs
			return e.HandleCredentialError(ctx, err)
		}
	}
	// We will get here if all of the instances are deleted successfully,
	// or are not found, which implies they were previously deleted.
	ids = deletedIDs
	return nil
}

var terminateInstancesById = func(ctx context.Context, ec2inst Client, ids ...instance.Id) ([]types.InstanceStateChange, error) {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = string(id)
	}
	r, err := ec2inst.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: strs,
	})
	if err != nil {
		return nil, err
	}
	return r.TerminatingInstances, nil
}

func (e *environ) deleteSecurityGroupsForInstances(ctx envcontext.ProviderCallContext, ids []instance.Id) {
	if len(ids) == 0 {
		logger.Debugf(ctx, "no need to delete security groups: no intances were terminated successfully")
		return
	}

	// We only want to attempt deleting security groups for the
	// instances that have been successfully terminated.
	securityGroups, err := e.instanceSecurityGroups(ctx, ids, terminatingStates.Values()...)
	if err != nil {
		logger.Errorf(ctx, "cannot determine security groups to delete: %v", err)
		return
	}

	// TODO(perrito666) we need to tag global security groups to be able
	// to tell them apart from future groups that are neither machine
	// nor environment group.
	// https://bugs.launchpad.net/juju-core/+bug/1534289
	jujuGroup := e.jujuGroupName()

	for _, deletable := range securityGroups {
		if aws.ToString(deletable.GroupName) == jujuGroup {
			continue
		}
		if err := deleteSecurityGroupInsistently(ctx, e.ec2Client, deletable, clock.WallClock); err != nil {
			// In ideal world, we would err out here.
			// However:
			// 1. We do not know if all instances have been terminated.
			// If some instances erred out, they may still be using this security group.
			// In this case, our failure to delete security group is reasonable: it's still in use.
			// 2. Some security groups may be shared by multiple instances,
			// for example, global firewalling. We should not delete these.
			logger.Warningf(ctx, "%v", e.HandleCredentialError(ctx, err))
		}
	}
}

// SecurityGroupCleaner defines provider instance methods needed to delete
// a security group.
type SecurityGroupCleaner interface {
	// DeleteSecurityGroup deletes security group on the provider.
	DeleteSecurityGroup(context.Context, *ec2.DeleteSecurityGroupInput, ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
}

var deleteSecurityGroupInsistently = func(ctx context.Context, client SecurityGroupCleaner, g types.GroupIdentifier, clock clock.Clock) error {
	err := retry.Call(retry.CallArgs{
		Attempts:    30,
		Delay:       time.Second,
		MaxDelay:    time.Minute, // because 2**29 seconds is beyond reasonable
		BackoffFunc: retry.DoubleDelay,
		Clock:       clock,
		Func: func() error {
			_, err := client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
				GroupId: g.GroupId,
			})
			if err == nil || isNotFoundError(err) {
				logger.Debugf(ctx, "deleting security group %q", aws.ToString(g.GroupName))
				return nil
			}
			return errors.Trace(err)
		},
		IsFatalError: func(err error) bool {
			return errors.Is(err, common.ErrorCredentialNotValid)
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf(ctx, "deleting security group %q, attempt %d (%v)", aws.ToString(g.GroupName), attempt, err)
		},
	})
	if err != nil {
		return errors.Annotatef(err, "cannot delete security group %q (%q): consider deleting it manually",
			aws.ToString(g.GroupName), aws.ToString(g.GroupId))
	}
	return nil
}

func makeModelFilter(modelUUID string) types.Filter {
	return makeFilter(fmt.Sprintf("tag:%s", tags.JujuModel), modelUUID)
}

func makeControllerFilter(controllerUUID string) types.Filter {
	return makeFilter(fmt.Sprintf("tag:%s", tags.JujuController), controllerUUID)
}

func (e *environ) uuid() string {
	return e.Config().UUID()
}

func (e *environ) globalGroupName() string {
	return fmt.Sprintf("%s-global", e.jujuGroupName())
}

func (e *environ) machineGroupName(machineId string) string {
	return fmt.Sprintf("%s-%s", e.jujuGroupName(), machineId)
}

func (e *environ) jujuGroupName() string {
	return "juju-" + e.uuid()
}

// setUpGroups creates the security groups for the new machine, and
// returns them.
//
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same EC2 account.  In
// addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
func (e *environ) setUpGroups(ctx envcontext.ProviderCallContext, controllerUUID, machineId string) ([]string, error) {
	jujuGroup, err := e.ensureGroup(ctx, e.jujuGroupName(), true)
	if err != nil {
		return nil, err
	}

	var machineGroup types.SecurityGroup
	switch e.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = e.ensureGroup(ctx, e.machineGroupName(machineId), false)
	case config.FwGlobal:
		machineGroup, err = e.ensureGroup(ctx, e.globalGroupName(), false)
	}
	if err != nil {
		return nil, err
	}
	return []string{aws.ToString(jujuGroup.GroupId), aws.ToString(machineGroup.GroupId)}, nil
}

// securityGroupsByNameOrID calls ec2.SecurityGroups() either with the given
// groupName or with filter by vpc-id and group-name, depending on whether
// vpc-id is empty or not.
func (e *environ) securityGroupsByNameOrID(ctx context.Context, groupName string) ([]types.SecurityGroup, error) {
	var (
		groups  []string
		filters []types.Filter
	)

	if chosenVPCID := e.ecfg().vpcID(); isVPCIDSet(chosenVPCID) {
		// AWS VPC API requires both of these filters (and no
		// group names/ids set) for non-default EC2-VPC groups:
		filters = []types.Filter{
			makeFilter("vpc-id", chosenVPCID),
			makeFilter("group-name", groupName),
		}
	} else {
		// EC2-Classic or EC2-VPC with implicit default VPC need to use
		// the GroupName.X arguments instead of the filters.
		groups = []string{groupName}
	}

	// If the security group was just created, it might not be available
	// yet as EC2 resources are eventually consistent. If we get a NotFound
	// error from EC2 we will retry the request using the shortRetryStrategy
	// strategy before giving up.
	var resp *ec2.DescribeSecurityGroupsOutput

	retryStrategy := shortRetryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		return !strings.HasSuffix(ec2ErrCode(err), ".NotFound")
	}
	retryStrategy.Func = func() error {
		var err error
		resp, err = e.ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
			GroupNames: groups,
			Filters:    filters,
		})
		return err
	}
	err := retry.Call(retryStrategy)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = errors.NewNotFound(retry.LastError(err), fmt.Sprintf("security group %s not found", groupName))
	}
	if err != nil {
		return nil, err
	}
	return resp.SecurityGroups, nil
}

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
// Any entries in perms without SourceIPs will be granted for
// the named group only.
func (e *environ) ensureGroup(ctx envcontext.ProviderCallContext, name string, isModelGroup bool) (types.SecurityGroup, error) {
	// Due to parallelization of the provisioner, it's possible that we try
	// to create the model security group a second time before the first time
	// is complete causing failures.
	e.ensureGroupMutex.Lock()
	defer e.ensureGroupMutex.Unlock()

	// Specify explicit VPC ID if needed (not for default VPC or EC2-classic).
	var vpcIDParam *string
	chosenVPCID := e.ecfg().vpcID()
	inVPCLogSuffix := ""
	if isVPCIDSet(chosenVPCID) {
		inVPCLogSuffix = fmt.Sprintf(" (in VPC %q)", chosenVPCID)
		vpcIDParam = aws.String(chosenVPCID)
	}

	// Tag the created group with the model and controller UUIDs.
	cfg := e.Config()
	tags := tags.ResourceTags(
		names.NewModelTag(cfg.UUID()),
		names.NewControllerTag(e.controllerUUID),
		cfg,
	)
	groupTagSpec := CreateTagSpecification(types.ResourceTypeSecurityGroup, tags)

	_, err := e.ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		VpcId:       vpcIDParam,
		Description: aws.String("juju group"),
		TagSpecifications: []types.TagSpecification{
			groupTagSpec,
		},
	})
	if err != nil && ec2ErrCode(err) != "InvalidGroup.Duplicate" {
		err = errors.Annotatef(e.HandleCredentialError(ctx, err), "creating security group %q%s", name, inVPCLogSuffix)
		return types.SecurityGroup{}, err
	}

	groups, err := e.securityGroupsByNameOrID(ctx, name)
	if err != nil {
		return types.SecurityGroup{}, errors.Annotatef(e.HandleCredentialError(ctx, err), "fetching security group %q%s", name, inVPCLogSuffix)
	}
	if len(groups) == 0 {
		return types.SecurityGroup{}, errors.NotFoundf("security group %q%s", name, inVPCLogSuffix)
	}
	if len(groups) > 1 {
		logger.Debugf(ctx, "more than one security group with name %q", name)
	}
	group := groups[0]

	if isModelGroup {
		if err := e.ensureInternalRules(ctx, group); err != nil {
			return types.SecurityGroup{}, errors.Annotate(err, "failed to enable internal model rules")
		}
	} else {
		if err := e.ensureICMPRules(ctx, group); err != nil {
			return types.SecurityGroup{}, err
		}
	}

	return group, nil
}

var internalPermissionDescription = aws.String("juju internal model rule")

func (e *environ) ensureInternalRules(ctx envcontext.ProviderCallContext, group types.SecurityGroup) error {
	perms := []types.IpPermission{{
		IpProtocol:       aws.String("tcp"),
		FromPort:         aws.Int32(0),
		ToPort:           aws.Int32(65535),
		UserIdGroupPairs: []types.UserIdGroupPair{{GroupId: group.GroupId, Description: internalPermissionDescription}},
	}, {
		IpProtocol:       aws.String("udp"),
		FromPort:         aws.Int32(0),
		ToPort:           aws.Int32(65535),
		UserIdGroupPairs: []types.UserIdGroupPair{{GroupId: group.GroupId, Description: internalPermissionDescription}},
	}, {
		IpProtocol:       aws.String("icmp"),
		FromPort:         aws.Int32(-1),
		ToPort:           aws.Int32(-1),
		UserIdGroupPairs: []types.UserIdGroupPair{{GroupId: group.GroupId, Description: internalPermissionDescription}},
	}}
	for _, perm := range perms {
		_, err := e.ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       group.GroupId,
			IpPermissions: []types.IpPermission{perm},
		})
		if err != nil && ec2ErrCode(err) != "InvalidPermission.Duplicate" {
			return errors.Trace(err)
		}
	}
	return nil
}

// ensureICMPRules here to insure that the security group has the correct icmp
// rules applied to it. Specifically this function will ensure IPv6 ICMP rules
// in accordance with RFC4890. We don't deal with ipv4 icmp rules here as Juju
// has historically blocked these and network operators feel uneasy about the
// port being open.
func (e *environ) ensureICMPRules(ctx envcontext.ProviderCallContext, group types.SecurityGroup) error {
	openAccessV6 := types.Ipv6Range{CidrIpv6: aws.String("::/0")}
	perms := []types.IpPermission{
		{
			IpProtocol: aws.String("icmpv6"),
			FromPort:   aws.Int32(1),
			ToPort:     aws.Int32(-1),
			Ipv6Ranges: []types.Ipv6Range{openAccessV6},
		},
		// Packet Too Big (Type 2)
		{
			IpProtocol: aws.String("icmpv6"),
			FromPort:   aws.Int32(2),
			ToPort:     aws.Int32(-1),
			Ipv6Ranges: []types.Ipv6Range{openAccessV6},
		},
		// Time Exceeded (Type 3) - Code 0 only
		{
			IpProtocol: aws.String("icmpv6"),
			FromPort:   aws.Int32(3),
			ToPort:     aws.Int32(0),
			Ipv6Ranges: []types.Ipv6Range{openAccessV6},
		},
		// Parameter Problem (Type 4) - Codes 1 and 2 only
		{
			IpProtocol: aws.String("icmpv6"),
			FromPort:   aws.Int32(4),
			ToPort:     aws.Int32(1),
			Ipv6Ranges: []types.Ipv6Range{openAccessV6},
		},
		{
			IpProtocol: aws.String("icmpv6"),
			FromPort:   aws.Int32(4),
			ToPort:     aws.Int32(2),
			Ipv6Ranges: []types.Ipv6Range{openAccessV6},
		},

		// >>> Connectivity checking messages
		// Echo Request (Type 128)
		{
			IpProtocol: aws.String("icmpv6"),
			FromPort:   aws.Int32(128),
			ToPort:     aws.Int32(-1),
			Ipv6Ranges: []types.Ipv6Range{openAccessV6},
		},

		// >>> Traffic that normally should not be dropped
		// Time Exceeded (Type 3) - Code 1
		{
			IpProtocol: aws.String("icmpv6"),
			FromPort:   aws.Int32(3),
			ToPort:     aws.Int32(1),
			Ipv6Ranges: []types.Ipv6Range{openAccessV6},
		},

		// Parameter Problem (Type 4) - Code 0
		{
			IpProtocol: aws.String("icmpv6"),
			FromPort:   aws.Int32(4),
			ToPort:     aws.Int32(0),
			Ipv6Ranges: []types.Ipv6Range{openAccessV6},
		},
	}

	_, err := e.ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       group.GroupId,
		IpPermissions: perms,
	})
	if err != nil && ec2ErrCode(err) != "InvalidPermission.Duplicate" {
		return fmt.Errorf("applying icmpv6 rules to security group %q: %w", *group.GroupId, err)
	}
	return nil
}

func isZoneOrSubnetConstrainedError(err error) bool {
	return isZoneConstrainedError(err) || isSubnetConstrainedError(err)
}

// isZoneConstrainedError reports whether or not the error indicates
// RunInstances failed due to the specified availability zone being
// constrained for the instance type being provisioned, or is
// otherwise unusable for the specific request made.
func isZoneConstrainedError(err error) bool {
	var apiErr smithy.APIError
	if stderrors.As(errors.Cause(err), &apiErr) {
		switch apiErr.ErrorCode() {
		case "Unsupported", "InsufficientInstanceCapacity":
			// A big hammer, but we've now seen several different error messages
			// for constrained zones, and who knows how many more there might
			// be. If the message contains "Availability Zone", it's a fair
			// bet that it's constrained or otherwise unusable.
			return strings.Contains(apiErr.ErrorMessage(), "Availability Zone")
		case "InvalidInput":
			// If the region has a default VPC, then we will receive an error
			// if the AZ does not have a default subnet. Until we have proper
			// support for networks, we'll skip over these.
			return strings.HasPrefix(apiErr.ErrorMessage(), "No default subnet for availability zone")
		case "VolumeTypeNotAvailableInZone":
			return true
		}
	}
	return false
}

// isSubnetConstrainedError reports whether or not the error indicates
// RunInstances failed due to the specified VPC subnet ID being constrained for
// the instance type being provisioned, or is otherwise unusable for the
// specific request made.
func isSubnetConstrainedError(err error) bool {
	switch ec2ErrCode(err) {
	case "InsufficientFreeAddressesInSubnet", "InsufficientInstanceCapacity":
		// Subnet and/or VPC general limits reached.
		return true
	case "InvalidSubnetID.NotFound":
		// This shouldn't happen, as we validate the subnet IDs, but it can
		// happen if the user manually deleted the subnet outside of Juju.
		return true
	}
	return false
}

// If the err is of type *ec2.Error, ec2ErrCode returns
// its code, otherwise it returns the empty string.
func ec2ErrCode(err error) string {
	var apiErr smithy.APIError
	if stderrors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}

func (e *environ) hasDefaultVPC(ctx envcontext.ProviderCallContext) (bool, error) {
	e.defaultVPCMutex.Lock()
	defer e.defaultVPCMutex.Unlock()
	if !e.defaultVPCChecked {
		filter := makeFilter("isDefault", "true")
		resp, err := e.ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
			Filters: []types.Filter{filter},
		})
		if err != nil {
			return false, errors.Trace(e.HandleCredentialError(ctx, err))
		}
		if len(resp.Vpcs) > 0 {
			e.defaultVPC = &resp.Vpcs[0]
		}
		e.defaultVPCChecked = true
	}
	return e.defaultVPC != nil, nil
}

// SetCloudSpec is specified in the environs.Environ interface.
func (e *environ) SetCloudSpec(ctx context.Context, spec environscloudspec.CloudSpec) error {
	if err := validateCloudSpec(spec); err != nil {
		return errors.Annotate(err, "validating cloud spec")
	}

	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()

	e.cloud = spec

	// Allow the passing of a client func through the context. This allows
	// passing the client from outside of the environ, one that allows for
	// custom http.Clients.
	//
	// This isn't in it's final form. It is expected that eventually the ec2
	// client will be passed in via the constructor of the environ. That can
	// then be passed in via the environProvider. Unfortunately the provider
	// (factory) is registered in an init function and makes it VERY hard to
	// override. The solution to all of this is to remove the global registry
	// and construct that within the main (or provide sane defaults). The
	// provider/all package can then be removed and the black magic for provider
	// registration can then vanish and plain old dependency management can
	// then be used.
	if value := ctx.Value(AWSClientContextKey); value == nil {
		e.ec2ClientFunc = clientFunc
	} else if s, ok := value.(ClientFunc); ok {
		e.ec2ClientFunc = s
	} else {
		return errors.Errorf("expected a valid client function type")
	}

	if value := ctx.Value(AWSIAMClientContextKey); value == nil {
		e.iamClientFunc = iamClientFunc
	} else if s, ok := value.(IAMClientFunc); ok {
		e.iamClientFunc = s
	} else {
		return errors.Errorf("expected a valid iam client function type")
	}

	httpClient := jujuhttp.NewClient(
		jujuhttp.WithLogger(logger.Child("http", corelogger.HTTP)),
	)

	var err error
	e.ec2Client, err = e.ec2ClientFunc(ctx, spec, WithHTTPClient(httpClient.Client()))
	if err != nil {
		return errors.Annotate(err, "creating aws ec2 client")
	}
	e.iamClient, err = e.iamClientFunc(ctx, spec, WithHTTPClient(httpClient.Client()))
	if err != nil {
		return errors.Annotate(err, "creating aws iam client")
	}

	e.instTypesCacheMutex.Lock()
	e.instTypesCache = newInstanceTypeCache(e.ec2Client, e.cloud.Region)
	e.instTypesCacheMutex.Unlock()
	return nil
}

// SupportsRulesWithIPV6CIDRs returns true if the environment supports
// ingress rules containing IPV6 CIDRs.
//
// This is part of the environs.FirewallFeatureQuerier interface.
func (e *environ) SupportsRulesWithIPV6CIDRs(envcontext.ProviderCallContext) (bool, error) {
	return true, nil
}

// CreateTagSpecification creates an AWS tag specification for the given
// resource type and tags.
func CreateTagSpecification(resourceType types.ResourceType, tags map[string]string) types.TagSpecification {
	spec := types.TagSpecification{
		ResourceType: resourceType,
		Tags:         make([]types.Tag, 0, len(tags)),
	}

	for k, v := range tags {
		spec.Tags = append(spec.Tags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	return spec
}
