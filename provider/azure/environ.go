// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/utils/v3/arch"
	"github.com/juju/version/v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/os"
	jujuseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	"github.com/juju/juju/provider/azure/internal/tracing"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

const (
	jujuMachineNameTag = tags.JujuTagPrefix + "machine-name"

	// minRootDiskSize is the minimum root disk size Azure
	// accepts for a VM's OS disk.
	// It will be used if none is specified by the user.
	minRootDiskSize = 30 * 1024 // 30 GiB

	// serviceErrorCodeDeploymentCannotBeCancelled is the error code for
	// service errors in response to an attempt to cancel a deployment
	// that cannot be cancelled.
	serviceErrorCodeDeploymentCannotBeCancelled = "DeploymentCannotBeCancelled"

	// serviceErrorCodeResourceGroupBeingDeleted is the error code for
	// service errors in response to an attempt to cancel a deployment
	// that has already started to be deleted.
	serviceErrorCodeResourceGroupBeingDeleted = "ResourceGroupBeingDeleted"

	// controllerAvailabilitySet is the name of the availability set
	// used for controller machines.
	controllerAvailabilitySet = "juju-controller"

	// commonDeployment is used to create resources common to all models.
	commonDeployment = "common"

	computeAPIVersion = "2021-11-01"
	networkAPIVersion = "2018-08-01"
)

type azureEnviron struct {
	environs.NoSpaceDiscoveryEnviron

	// provider is the azureEnvironProvider used to open this environment.
	provider *azureEnvironProvider

	// cloud defines the cloud configuration for this environment.
	cloud environscloudspec.CloudSpec

	// location is the canonical location name. Use this instead
	// of cloud.Region in API calls.
	location string

	// subscriptionId is the Azure account subscription ID.
	subscriptionId string

	// tenantId is the Azure account tenant ID.
	tenantId string

	// storageEndpoint is the Azure storage endpoint. This is the host
	// portion of the storage endpoint URL only; use this instead of
	// cloud.StorageEndpoint in API calls.
	storageEndpoint string

	// resourceGroup is the name of the Resource Group in the Azure
	// subscription that corresponds to the environment.
	resourceGroup string

	// modelName is the name of the model.
	modelName string

	clientOptions policy.ClientOptions
	credential    azcore.TokenCredential

	mu                     sync.Mutex
	config                 *azureModelConfig
	instanceTypes          map[string]instances.InstanceType
	commonResourcesCreated bool
}

var _ environs.Environ = (*azureEnviron)(nil)

// SetCloudSpec is specified in the environs.Environ interface.
func (env *azureEnviron) SetCloudSpec(ctx stdcontext.Context, cloud environscloudspec.CloudSpec) error {
	if err := validateCloudSpec(cloud); err != nil {
		return errors.Annotate(err, "validating cloud spec")
	}

	env.mu.Lock()
	defer env.mu.Unlock()

	// The Azure storage code wants the endpoint host only, not the URL.
	storageEndpointURL, err := url.Parse(cloud.StorageEndpoint)
	if err != nil {
		return errors.Annotate(err, "parsing storage endpoint URL")
	}
	env.cloud = cloud
	env.location = canonicalLocation(cloud.Region)
	env.storageEndpoint = storageEndpointURL.Host

	if err := env.initEnviron(ctx); err != nil {
		return errors.Trace(err)
	}

	cfg := env.config
	if env.resourceGroup == "" {
		env.resourceGroup = cfg.resourceGroupName
	}
	// If no user specified resource group, make one from the model UUID.
	if env.resourceGroup == "" {
		modelTag := names.NewModelTag(cfg.UUID())
		if env.resourceGroup, err = env.resourceGroupName(ctx, modelTag, cfg.Name()); err != nil {
			return errors.Trace(err)
		}
	}
	env.modelName = cfg.Name()
	return nil
}

func (env *azureEnviron) initEnviron(ctx stdcontext.Context) error {
	credAttrs := env.cloud.Credential.Attributes()
	env.subscriptionId = credAttrs[credAttrManagedSubscriptionId]
	if env.subscriptionId == "" {
		env.subscriptionId = credAttrs[credAttrSubscriptionId]
	}

	env.clientOptions = azcore.ClientOptions{
		Cloud: azureCloud(env.cloud.Endpoint, env.cloud.IdentityEndpoint),
		PerCallPolicies: []policy.Policy{
			&tracing.LoggingPolicy{
				Logger: logger.Child("azureapi"),
			},
		},
		Telemetry: policy.TelemetryOptions{
			ApplicationID: "Juju/" + jujuversion.Current.String(),
		},
		Transport: env.provider.config.Sender,
		Retry:     env.provider.config.Retry,
	}
	if env.provider.config.RequestInspector != nil {
		env.clientOptions.PerCallPolicies = append(env.clientOptions.PerCallPolicies, env.provider.config.RequestInspector)
	}

	tenantID, err := azureauth.DiscoverTenantID(ctx, env.subscriptionId, arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
	if err != nil {
		return errors.Annotate(err, "getting tenant ID")
	}
	logger.Debugf("discovered tenant id: %s", tenantID)
	env.tenantId = tenantID

	appId := credAttrs[credAttrAppId]
	appPassword := credAttrs[credAttrAppPassword]
	env.credential, err = env.provider.config.CreateTokenCredential(appId, appPassword, tenantID, env.clientOptions)
	if err != nil {
		return errors.Annotate(err, "set up credential")
	}
	return nil
}

// PrepareForBootstrap is part of the Environ interface.
func (env *azureEnviron) PrepareForBootstrap(ctx environs.BootstrapContext, _ string) error {
	if ctx.ShouldVerifyCredentials() {
		cloudCtx := &context.CloudCallContext{
			Context:                  ctx.Context(),
			InvalidateCredentialFunc: func(string) error { return nil },
		}
		if err := verifyCredentials(env, cloudCtx); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Create is part of the Environ interface.
func (env *azureEnviron) Create(ctx context.ProviderCallContext, args environs.CreateParams) error {
	if err := verifyCredentials(env, ctx); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(env.initResourceGroup(ctx, args.ControllerUUID, env.config.resourceGroupName != "", false))
}

// Bootstrap is part of the Environ interface.
func (env *azureEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	callCtx context.ProviderCallContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	if err := env.initResourceGroup(callCtx, args.ControllerConfig.ControllerUUID(), env.config.resourceGroupName != "", true); err != nil {
		return nil, errors.Annotate(err, "creating controller resource group")
	}
	result, err := common.Bootstrap(ctx, env, callCtx, args)
	if err != nil {
		logger.Errorf("bootstrap failed, destroying model: %v", err)

		// First cancel the in-progress deployment.
		var wg sync.WaitGroup
		var cancelResult error
		logger.Debugf("canceling deployment for bootstrap instance")
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			cancelResult = errors.Annotatef(
				env.cancelDeployment(callCtx, id),
				"canceling deployment %q", id,
			)
		}(names.NewMachineTag(agent.BootstrapControllerId).String())
		wg.Wait()
		if cancelResult != nil && !errors.IsNotFound(cancelResult) {
			return nil, errors.Annotate(cancelResult, "aborting failed bootstrap")
		}

		// Then cleanup the resource group.
		if err := env.Destroy(callCtx); err != nil {
			logger.Errorf("failed to destroy model: %v", err)
		}
		return nil, errors.Trace(err)
	}
	return result, nil
}

// initResourceGroup creates a resource group for this environment.
func (env *azureEnviron) initResourceGroup(ctx context.ProviderCallContext, controllerUUID string, existingResourceGroup, controller bool) error {
	env.mu.Lock()
	resourceTags := tags.ResourceTags(
		names.NewModelTag(env.config.Config.UUID()),
		names.NewControllerTag(controllerUUID),
		env.config,
	)
	env.mu.Unlock()

	resourceGroups, err := env.resourceGroupsClient()
	if err != nil {
		return errors.Trace(err)
	}
	if existingResourceGroup {
		logger.Debugf("using existing resource group %q for model %q", env.resourceGroup, env.modelName)
		g, err := resourceGroups.Get(ctx, env.resourceGroup, nil)
		if err != nil {
			return errorutils.HandleCredentialError(errors.Annotatef(err, "checking resource group %q", env.resourceGroup), ctx)
		}
		if region := toValue(g.Location); region != env.location {
			return errors.Errorf("cannot use resource group in region %q when operating in region %q", region, env.location)
		}
	} else {
		logger.Debugf("creating resource group %q for model %q", env.resourceGroup, env.modelName)
		if _, err := resourceGroups.CreateOrUpdate(ctx, env.resourceGroup, armresources.ResourceGroup{
			Location: to.Ptr(env.location),
			Tags:     toMapPtr(resourceTags),
		}, nil); err != nil {
			return errorutils.HandleCredentialError(errors.Annotate(err, "creating resource group"), ctx)
		}
	}

	if !controller {
		// When we create a resource group for a non-controller model,
		// we must create the common resources up-front. This is so
		// that parallel deployments do not affect dynamic changes,
		// e.g. those made by the firewaller. For the controller model,
		// we fold the creation of these resources into the bootstrap
		// machine's deployment.
		if err := env.createCommonResourceDeployment(ctx, resourceTags, nil); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (env *azureEnviron) createCommonResourceDeployment(
	ctx context.ProviderCallContext,
	tags map[string]string,
	rules []*armnetwork.SecurityRule,
	commonResources ...armtemplates.Resource,
) error {
	// Only create network resources if the user has not
	// specified their own to use.
	if env.config.virtualNetworkName == "" {
		networkResources, _ := networkTemplateResources(env.location, tags, nil, rules)
		commonResources = append(commonResources, networkResources...)
	}
	if len(commonResources) == 0 {
		return nil
	}

	template := armtemplates.Template{Resources: commonResources}
	if err := env.createDeployment(
		ctx,
		env.resourceGroup,
		commonDeployment,
		template,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ControllerInstances is specified in the Environ interface.
func (env *azureEnviron) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	inst, err := env.allInstances(ctx, env.resourceGroup, false, controllerUUID)
	if err != nil {
		return nil, err
	}
	if len(inst) == 0 {
		return nil, environs.ErrNoInstances
	}
	ids := make([]instance.Id, len(inst))
	for i, inst := range inst {
		ids[i] = inst.Id()
	}
	return ids, nil
}

// Config is specified in the Environ interface.
func (env *azureEnviron) Config() *config.Config {
	env.mu.Lock()
	defer env.mu.Unlock()
	return env.config.Config
}

// SetConfig is specified in the Environ interface.
func (env *azureEnviron) SetConfig(cfg *config.Config) error {
	env.mu.Lock()
	defer env.mu.Unlock()

	var old *config.Config
	if env.config != nil {
		old = env.config.Config
	}
	ecfg, err := validateConfig(cfg, old)
	if err != nil {
		return err
	}
	env.config = ecfg

	return nil
}

// ConstraintsValidator is defined on the Environs interface.
func (env *azureEnviron) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	instanceTypes, err := env.getInstanceTypes(ctx)
	if err != nil {
		return nil, err
	}
	instTypeNames := make([]string, 0, len(instanceTypes))
	for instTypeName := range instanceTypes {
		instTypeNames = append(instTypeNames, instTypeName)
	}
	sort.Strings(instTypeNames)

	validator := constraints.NewValidator()
	validator.RegisterUnsupported([]string{
		constraints.CpuPower,
		constraints.Tags,
		constraints.VirtType,
	})
	validator.RegisterVocabulary(
		constraints.Arch,
		[]string{arch.AMD64},
	)
	validator.RegisterVocabulary(
		constraints.InstanceType,
		instTypeNames,
	)
	validator.RegisterConflicts(
		[]string{constraints.InstanceType},
		[]string{
			constraints.Mem,
			constraints.Cores,
			// TODO: move to a dynamic conflict for arch when azure supports more than amd64
			//constraints.Arch,
		},
	)
	return validator, nil
}

// PrecheckInstance is defined on the environs.InstancePrechecker interface.
func (env *azureEnviron) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if _, err := env.findPlacementSubnet(ctx, args.Placement); err != nil {
		return errors.Trace(err)
	}
	if !args.Constraints.HasInstanceType() {
		return nil
	}
	// Constraint has an instance-type constraint so let's see if it is valid.
	instanceTypes, err := env.getInstanceTypes(ctx)
	if err != nil {
		return err
	}
	for _, instanceType := range instanceTypes {
		if instanceType.Name == *args.Constraints.InstanceType {
			return nil
		}
	}
	return fmt.Errorf("invalid instance type %q", *args.Constraints.InstanceType)
}

// StartInstance is specified in the InstanceBroker interface.
func (env *azureEnviron) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	if args.ControllerUUID == "" {
		return nil, errors.New("missing controller UUID")
	}

	// Get the required configuration and config-dependent information
	// required to create the instance. We take the lock just once, to
	// ensure we obtain all information based on the same configuration.
	env.mu.Lock()
	envTags := tags.ResourceTags(
		names.NewModelTag(env.config.Config.UUID()),
		names.NewControllerTag(args.ControllerUUID),
		env.config,
	)
	imageStream := env.config.ImageStream()
	envInstanceTypes, err := env.getInstanceTypesLocked(ctx)
	if err != nil {
		env.mu.Unlock()
		return nil, errors.Trace(err)
	}
	instanceTypes := make(map[string]instances.InstanceType)
	for k, v := range envInstanceTypes {
		instanceTypes[k] = v
	}
	env.mu.Unlock()

	// If the user has not specified a root-disk size, then
	// set a sensible default.
	var rootDisk uint64
	// Azure complains if we try and specify a root disk size less than the minimum.
	// See http://pad.lv/1645408
	if args.Constraints.RootDisk != nil && *args.Constraints.RootDisk > minRootDiskSize {
		rootDisk = *args.Constraints.RootDisk
	} else {
		rootDisk = minRootDiskSize
		args.Constraints.RootDisk = &rootDisk
	}
	// Start the instance - if we get a quota error, that instance type is ignored
	// and we'll try the next most expensive one, up to a reasonable number of attempts.
	arch, err := args.Tools.OneArch()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for i := 0; i < 15; i++ {
		// Identify the instance type and image to provision.
		instanceSpec, err := env.findInstanceSpec(
			ctx,
			instanceTypes,
			&instances.InstanceConstraint{
				Region:      env.location,
				Series:      args.InstanceConfig.Series,
				Arch:        arch,
				Constraints: args.Constraints,
			},
			imageStream,
		)
		if err != nil {
			return nil, err
		}
		if rootDisk < instanceSpec.InstanceType.RootDisk {
			// The InstanceType's RootDisk is set to the maximum
			// OS disk size; override it with the user-specified
			// or default root disk size.
			instanceSpec.InstanceType.RootDisk = rootDisk
		}
		result, err := env.startInstance(ctx, args, instanceSpec, envTags)
		quotaErr, ok := errorutils.MaybeQuotaExceededError(err)
		if ok {
			logger.Warningf("%v quota exceeded error: %q", instanceSpec.InstanceType.Name, quotaErr.Error())
			deleteInstanceFamily(instanceTypes, instanceSpec.InstanceType.Name)
			continue
		}
		return result, errorutils.SimpleError(err)
	}
	return nil, errors.New("no suitable instance type found for this subscription")
}
func (env *azureEnviron) startInstance(
	ctx context.ProviderCallContext, args environs.StartInstanceParams,
	instanceSpec *instances.InstanceSpec, envTags map[string]string,
) (*environs.StartInstanceResult, error) {

	// Pick tools by filtering the available tools down to the architecture of
	// the image that will be provisioned.
	selectedTools, err := args.Tools.Match(tools.Filter{
		Arch: instanceSpec.Image.Arch,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("picked agent binaries %q", selectedTools[0].Version)

	// Finalize the instance config, which we'll render to CustomData below.
	if err := args.InstanceConfig.SetTools(selectedTools); err != nil {
		return nil, errors.Trace(err)
	}
	if err := instancecfg.FinishInstanceConfig(
		args.InstanceConfig, env.Config(),
	); err != nil {
		return nil, err
	}

	machineTag := names.NewMachineTag(args.InstanceConfig.MachineId)
	vmName := resourceName(machineTag)
	vmTags := make(map[string]string)
	for k, v := range args.InstanceConfig.Tags {
		vmTags[k] = v
	}
	// jujuMachineNameTag identifies the VM name, in which is encoded
	// the Juju machine name. We tag all resources related to the
	// machine with this.
	vmTags[jujuMachineNameTag] = vmName

	// Use a public IP by default unless a constraint
	// explicitly forbids it.
	usePublicIP := true
	if args.Constraints.HasAllocatePublicIP() {
		usePublicIP = *args.Constraints.AllocatePublicIP
	}
	err = env.createVirtualMachine(
		ctx, vmName, vmTags, envTags,
		instanceSpec, args, usePublicIP, true,
	)
	// If there's a conflict, it's because another machine is
	// being provisioned with the same availability set so
	// retry and do not create the availability set.
	if errorutils.IsConflictError(err) {
		logger.Debugf("conflict creating %s, retrying...", vmName)
		err = env.createVirtualMachine(
			ctx, vmName, vmTags, envTags,
			instanceSpec, args, usePublicIP, false,
		)
	}
	if err != nil {
		logger.Debugf("creating instance failed, destroying: %v", err)
		if err := env.StopInstances(ctx, instance.Id(vmName)); err != nil {
			logger.Errorf("could not destroy failed virtual machine: %v", err)
		}
		return nil, errors.Annotatef(err, "creating virtual machine %q", vmName)
	}

	// Note: the instance is initialised without addresses to keep the
	// API chatter down. We will refresh the instance if we need to know
	// the addresses.
	inst := &azureInstance{
		vmName:            vmName,
		provisioningState: armresources.ProvisioningStateCreating,
		env:               env,
	}
	amd64 := arch.AMD64
	hc := &instance.HardwareCharacteristics{
		Arch:     &amd64,
		Mem:      &instanceSpec.InstanceType.Mem,
		RootDisk: &instanceSpec.InstanceType.RootDisk,
		CpuCores: &instanceSpec.InstanceType.CpuCores,
	}
	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: hc,
	}, nil
}

// referenceInfo splits a reference to an Azure entity into an
// optional resource group and name, or just name if no
// resource group is specified.
func referenceInfo(entityRef string) (entityRG, entityName string) {
	parts := strings.Split(entityRef, "/")
	if len(parts) == 1 {
		return "", entityRef
	}
	return parts[0], parts[1]
}

// createVirtualMachine creates a virtual machine and related resources.
//
// All resources created are tagged with the specified "vmTags", so if
// this function fails then all resources can be deleted by tag.
func (env *azureEnviron) createVirtualMachine(
	ctx context.ProviderCallContext,
	vmName string,
	vmTags, envTags map[string]string,
	instanceSpec *instances.InstanceSpec,
	args environs.StartInstanceParams,
	usePublicIP bool,
	createAvailabilitySet bool,
) error {
	instanceConfig := args.InstanceConfig
	apiPorts := make([]int, 0, 2)
	if instanceConfig.IsController() {
		apiPorts = append(apiPorts, instanceConfig.ControllerConfig.APIPort())
		if instanceConfig.ControllerConfig.AutocertDNSName() != "" {
			// Open port 80 as well as it handles Let's Encrypt HTTP challenge.
			apiPorts = append(apiPorts, 80)
		}
	} else {
		ports := instanceConfig.APIInfo.Ports()
		if len(ports) != 1 {
			return errors.Errorf("expected one API port, found %v", ports)
		}
		apiPorts = append(apiPorts, ports[0])
	}

	var nicDependsOn, vmDependsOn []string
	var res []armtemplates.Resource
	bootstrapping := instanceConfig.Bootstrap != nil
	// We only need to deal with creating network resources
	// if the user has not specified their own to use.
	if bootstrapping && env.config.virtualNetworkName == "" && args.Placement == "" {
		// We're starting the bootstrap machine, so we will create the
		// networking resources in the same deployment.
		networkResources, dependsOn := networkTemplateResources(env.location, envTags, apiPorts, nil)
		res = append(res, networkResources...)
		nicDependsOn = append(nicDependsOn, dependsOn...)
	}
	if !bootstrapping {
		// Wait for the common resource deployment to complete.
		if err := env.waitCommonResourcesCreated(ctx); err != nil {
			return errors.Annotate(
				err, "waiting for common resources to be created",
			)
		}
	}

	osProfile, seriesOS, err := newOSProfile(
		vmName, instanceConfig,
		env.provider.config.GenerateSSHKey,
	)
	if err != nil {
		return errors.Annotate(err, "creating OS profile")
	}
	storageProfile, err := newStorageProfile(
		vmName,
		instanceSpec,
	)
	if err != nil {
		return errors.Annotate(err, "creating storage profile")
	}
	diskEncryptionID, err := env.diskEncryptionInfo(ctx, args.RootDisk, envTags)
	if err != nil {
		return environs.ZoneIndependentError(fmt.Errorf("creating disk encryption info: %w", err))
	}
	if diskEncryptionID != "" && storageProfile.OSDisk.ManagedDisk != nil {
		storageProfile.OSDisk.ManagedDisk.DiskEncryptionSet = &armcompute.DiskEncryptionSetParameters{
			ID: to.Ptr(diskEncryptionID),
		}
	}

	var availabilitySetSubResource *armcompute.SubResource
	availabilitySetName, err := availabilitySetName(
		vmName, vmTags, instanceConfig.IsController(),
	)
	if err != nil {
		return errors.Annotate(err, "getting availability set name")
	}
	availabilitySetId := fmt.Sprintf(
		`[resourceId('Microsoft.Compute/availabilitySets','%s')]`,
		availabilitySetName,
	)
	if availabilitySetName != "" {
		availabilitySetSubResource = &armcompute.SubResource{
			ID: to.Ptr(availabilitySetId),
		}
	}
	if !createAvailabilitySet && availabilitySetName != "" {
		availabilitySet, err := env.availabilitySetsClient()
		if err != nil {
			return errors.Trace(err)
		}
		if _, err = availabilitySet.Get(ctx, env.resourceGroup, availabilitySetName, nil); err != nil {
			return errors.Annotatef(err, "expecting availability set %q to be available", availabilitySetName)
		}
	}
	if createAvailabilitySet && availabilitySetName != "" {
		availabilitySetProperties := &armcompute.AvailabilitySetProperties{
			// Azure complains when the fault domain count
			// is not specified, even though it is meant
			// to be optional and default to the maximum.
			// The maximum depends on the location, and
			// there is no API to query it.
			PlatformFaultDomainCount: to.Ptr(maxFaultDomains(env.location)),
		}
		res = append(res, armtemplates.Resource{
			APIVersion: computeAPIVersion,
			Type:       "Microsoft.Compute/availabilitySets",
			Name:       availabilitySetName,
			Location:   env.location,
			Tags:       envTags,
			Properties: availabilitySetProperties,
			Sku:        &armtemplates.Sku{Name: "Aligned"},
		})
		vmDependsOn = append(vmDependsOn, availabilitySetId)
	}

	placementSubnetID, err := env.findPlacementSubnet(ctx, args.Placement)
	if err != nil {
		return environs.ZoneIndependentError(err)
	}
	vnetId, subnetIds, err := env.networkInfoForInstance(ctx, args, bootstrapping, instanceConfig.IsController(), placementSubnetID)
	if err != nil {
		return environs.ZoneIndependentError(err)
	}
	logger.Debugf("creating instance using vnet %v, subnets %q", vnetId, subnetIds)

	if env.config.virtualNetworkName == "" && bootstrapping {
		nicDependsOn = append(nicDependsOn, vnetId)
	}

	var publicIPAddressId string
	if usePublicIP {
		publicIPAddressName := vmName + "-public-ip"
		publicIPAddressId = fmt.Sprintf(`[resourceId('Microsoft.Network/publicIPAddresses', '%s')]`, publicIPAddressName)
		// Default to static public IP so address is preserved across reboots.
		publicIPAddressAllocationMethod := armnetwork.IPAllocationMethodStatic
		if env.config.loadBalancerSkuName == string(armnetwork.LoadBalancerSKUNameBasic) {
			publicIPAddressAllocationMethod = armnetwork.IPAllocationMethodDynamic // preserve the settings that were used in Juju 2.4 and earlier
		}
		res = append(res, armtemplates.Resource{
			APIVersion: networkAPIVersion,
			Type:       "Microsoft.Network/publicIPAddresses",
			Name:       publicIPAddressName,
			Location:   env.location,
			Tags:       vmTags,
			Sku:        &armtemplates.Sku{Name: env.config.loadBalancerSkuName},
			Properties: &armnetwork.PublicIPAddressPropertiesFormat{
				PublicIPAddressVersion:   to.Ptr(armnetwork.IPVersionIPv4),
				PublicIPAllocationMethod: to.Ptr(publicIPAddressAllocationMethod),
			},
		})
	}

	// Create one NIC per subnet. The first one is the primary and has
	// the public IP address if so configured.
	var nics []*armcompute.NetworkInterfaceReference
	for i, subnetID := range subnetIds {
		primary := i == 0
		ipConfig := &armnetwork.InterfaceIPConfigurationPropertiesFormat{
			Primary:                   to.Ptr(primary),
			PrivateIPAllocationMethod: to.Ptr(armnetwork.IPAllocationMethodDynamic),
			Subnet:                    &armnetwork.Subnet{ID: to.Ptr(string(subnetID))},
		}
		if primary && usePublicIP {
			ipConfig.PublicIPAddress = &armnetwork.PublicIPAddress{
				ID: to.Ptr(publicIPAddressId),
			}
			nicDependsOn = append(nicDependsOn, publicIPAddressId)
		}
		ipConfigName := "primary"
		if i > 0 {
			ipConfigName = fmt.Sprintf("interface-%d", i)
		}
		nicName := vmName + "-" + ipConfigName
		nicId := fmt.Sprintf(`[resourceId('Microsoft.Network/networkInterfaces', '%s')]`, nicName)
		ipConfigurations := []*armnetwork.InterfaceIPConfiguration{{
			Name:       to.Ptr(ipConfigName),
			Properties: ipConfig,
		}}
		res = append(res, armtemplates.Resource{
			APIVersion: networkAPIVersion,
			Type:       "Microsoft.Network/networkInterfaces",
			Name:       nicName,
			Location:   env.location,
			Tags:       vmTags,
			Properties: &armnetwork.InterfacePropertiesFormat{
				IPConfigurations: ipConfigurations,
			},
			DependsOn: nicDependsOn,
		})
		vmDependsOn = append(vmDependsOn, nicId)

		nics = append(nics, &armcompute.NetworkInterfaceReference{
			ID: to.Ptr(nicId),
			Properties: &armcompute.NetworkInterfaceReferenceProperties{
				Primary: to.Ptr(primary),
			},
		})
	}

	res = append(res, armtemplates.Resource{
		APIVersion: computeAPIVersion,
		Type:       "Microsoft.Compute/virtualMachines",
		Name:       vmName,
		Location:   env.location,
		Tags:       vmTags,
		Properties: &armcompute.VirtualMachineProperties{
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes(
					instanceSpec.InstanceType.Name,
				)),
			},
			StorageProfile: storageProfile,
			OSProfile:      osProfile,
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkInterfaces: nics,
			},
			AvailabilitySet: availabilitySetSubResource,
		},
		DependsOn: vmDependsOn,
	})

	// On CentOS, we must add the CustomScript VM extension to run the
	// CustomData script.
	if seriesOS == os.CentOS {
		properties, err := vmExtensionProperties(seriesOS)
		if err != nil {
			return errors.Annotate(
				err, "creating virtual machine extension",
			)
		}
		res = append(res, armtemplates.Resource{
			APIVersion: computeAPIVersion,
			Type:       "Microsoft.Compute/virtualMachines/extensions",
			Name:       vmName + "/" + extensionName,
			Location:   env.location,
			Tags:       vmTags,
			Properties: properties,
			DependsOn:  []string{"Microsoft.Compute/virtualMachines/" + vmName},
		})
	}

	logger.Debugf("- creating virtual machine deployment in %q", env.resourceGroup)
	template := armtemplates.Template{Resources: res}
	if err := env.createDeployment(
		ctx,
		env.resourceGroup,
		vmName, // deployment name
		template,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// maxFaultDomains returns the maximum number of fault domains for the
// given location/region. The numbers were taken from
// https://docs.microsoft.com/en-au/azure/virtual-machines/windows/manage-availability,
// as at 31 August 2017.
func maxFaultDomains(location string) int32 {
	// From the page linked in the doc comment:
	// "The number of fault domains for managed availability sets varies
	// by region - either two or three per region."
	//
	// We record those that at the time of writing have 3. Anything
	// else has at least 2, so we just assume 2.
	switch location {
	case
		"eastus",
		"eastus2",
		"westus",
		"centralus",
		"northcentralus",
		"southcentralus",
		"northeurope",
		"westeurope":
		return 3
	}
	return 2
}

// waitCommonResourcesCreated waits for the "common" deployment to complete.
func (env *azureEnviron) waitCommonResourcesCreated(ctx context.ProviderCallContext) error {
	env.mu.Lock()
	defer env.mu.Unlock()
	if env.commonResourcesCreated {
		return nil
	}
	if _, err := env.waitCommonResourcesCreatedLocked(ctx); err != nil {
		return errors.Trace(err)
	}
	env.commonResourcesCreated = true
	return nil
}

type deploymentIncompleteError struct {
	error
}

func (env *azureEnviron) waitCommonResourcesCreatedLocked(ctx context.ProviderCallContext) (*armresources.DeploymentExtended, error) {
	// Release the lock while we're waiting, to avoid blocking others.
	env.mu.Unlock()
	defer env.mu.Lock()

	deploy, err := env.deployClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Wait for up to 5 minutes, with a 5 second polling interval,
	// for the "common" deployment to be in one of the terminal
	// states. The deployment typically takes only around 30 seconds,
	// but we allow for a longer duration to be defensive.
	var deployment *armresources.DeploymentExtended
	waitDeployment := func() error {
		result, err := deploy.Get(ctx, env.resourceGroup, commonDeployment, nil)
		if err != nil {
			if errorutils.IsNotFoundError(err) {
				// The controller model, and also models with bespoke
				// networks, do not have a "common" deployment
				// For controller models, common resources are created
				// in the machine-0 deployment to keep bootstrap times optimal.
				return nil
			}
			return errors.Annotate(err, "querying common deployment")
		}
		if result.Properties == nil {
			return deploymentIncompleteError{errors.New("deployment incomplete")}
		}

		state := toValue(result.Properties.ProvisioningState)
		if state == armresources.ProvisioningStateSucceeded {
			// The deployment has succeeded, so the resources are
			// ready for use.
			deployment = to.Ptr(result.DeploymentExtended)
			return nil
		}
		err = errors.Errorf("%q resource deployment status is %q", commonDeployment, state)
		switch state {
		case armresources.ProvisioningStateCanceled,
			armresources.ProvisioningStateFailed,
			armresources.ProvisioningStateDeleted:
		default:
			err = deploymentIncompleteError{err}
		}
		return err
	}
	if err := retry.Call(retry.CallArgs{
		Func: waitDeployment,
		IsFatalError: func(err error) bool {
			_, ok := err.(deploymentIncompleteError)
			return !ok
		},
		Attempts:    -1,
		Delay:       5 * time.Second,
		MaxDuration: 5 * time.Minute,
		Clock:       env.provider.config.RetryClock,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return deployment, nil
}

// createAvailabilitySet creates the availability set for a machine to use
// if it doesn't already exist, and returns the availability set's ID. The
// algorithm used for choosing the availability set is:
//   - if the machine is a controller, use the availability set name
//     "juju-controller";
//   - if the machine has units assigned, create an availability
//     name with a name based on the value of the tags.JujuUnitsDeployed tag
//     in vmTags, if it exists;
//   - otherwise, do not assign the machine to an availability set
func availabilitySetName(
	vmName string,
	vmTags map[string]string,
	controller bool,
) (string, error) {
	logger.Debugf("selecting availability set for %q", vmName)
	if controller {
		return controllerAvailabilitySet, nil
	}

	// We'll have to create an availability set. Use the name of one of the
	// services assigned to the machine.
	var availabilitySetName string
	if unitNames, ok := vmTags[tags.JujuUnitsDeployed]; ok {
		for _, unitName := range strings.Fields(unitNames) {
			if !names.IsValidUnit(unitName) {
				continue
			}
			serviceName, err := names.UnitApplication(unitName)
			if err != nil {
				return "", errors.Annotate(err, "getting application name")
			}
			availabilitySetName = serviceName
			break
		}
	}
	return availabilitySetName, nil
}

// newStorageProfile creates the storage profile for a virtual machine,
// based on the series and chosen instance spec.
func newStorageProfile(
	vmName string,
	instanceSpec *instances.InstanceSpec,
) (*armcompute.StorageProfile, error) {
	logger.Debugf("creating storage profile for %q", vmName)

	urnParts := strings.SplitN(instanceSpec.Image.Id, ":", 4)
	if len(urnParts) != 4 {
		return nil, errors.Errorf("invalid image ID %q", instanceSpec.Image.Id)
	}
	publisher := urnParts[0]
	offer := urnParts[1]
	sku := urnParts[2]
	vers := urnParts[3]

	osDiskName := vmName
	osDiskSizeGB := mibToGB(instanceSpec.InstanceType.RootDisk)
	osDisk := &armcompute.OSDisk{
		Name:         to.Ptr(osDiskName),
		CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
		Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
		DiskSizeGB:   to.Ptr(int32(osDiskSizeGB)),
		ManagedDisk: &armcompute.ManagedDiskParameters{
			StorageAccountType: to.Ptr(armcompute.StorageAccountTypesStandardLRS),
		},
	}

	return &armcompute.StorageProfile{
		ImageReference: &armcompute.ImageReference{
			Publisher: to.Ptr(publisher),
			Offer:     to.Ptr(offer),
			SKU:       to.Ptr(sku),
			Version:   to.Ptr(vers),
		},
		OSDisk: osDisk,
	}, nil
}

func mibToGB(mib uint64) uint64 {
	b := float64(mib * 1024 * 1024)
	return uint64(b / (1000 * 1000 * 1000))
}

func newOSProfile(
	vmName string,
	instanceConfig *instancecfg.InstanceConfig,
	generateSSHKey func(string) (string, string, error),
) (*armcompute.OSProfile, os.OSType, error) {
	logger.Debugf("creating OS profile for %q", vmName)

	customData, err := providerinit.ComposeUserData(instanceConfig, nil, AzureRenderer{})
	if err != nil {
		return nil, os.Unknown, errors.Annotate(err, "composing user data")
	}

	osProfile := &armcompute.OSProfile{
		ComputerName: to.Ptr(vmName),
		CustomData:   to.Ptr(string(customData)),
	}

	seriesOS, err := jujuseries.GetOSFromSeries(instanceConfig.Series)
	if err != nil {
		return nil, os.Unknown, errors.Trace(err)
	}
	switch seriesOS {
	case os.Ubuntu, os.CentOS:
		// SSH keys are handled by custom data, but must also be
		// specified in order to forego providing a password, and
		// disable password authentication.
		authorizedKeys := instanceConfig.AuthorizedKeys
		if len(authorizedKeys) == 0 {
			// Azure requires that machines be provisioned with
			// either a password or at least one SSH key. We
			// generate a key-pair to make Azure happy, but throw
			// away the private key so that nobody will be able
			// to log into the machine directly unless the keys
			// are updated with one that Juju tracks.
			_, public, err := generateSSHKey("")
			if err != nil {
				return nil, os.Unknown, errors.Trace(err)
			}
			authorizedKeys = public
		}

		publicKeys := []*armcompute.SSHPublicKey{{
			Path:    to.Ptr("/home/ubuntu/.ssh/authorized_keys"),
			KeyData: to.Ptr(authorizedKeys),
		}}
		osProfile.AdminUsername = to.Ptr("ubuntu")
		osProfile.LinuxConfiguration = &armcompute.LinuxConfiguration{
			DisablePasswordAuthentication: to.Ptr(true),
			SSH:                           &armcompute.SSHConfiguration{PublicKeys: publicKeys},
		}
	default:
		return nil, os.Unknown, errors.NotSupportedf("%s", seriesOS)
	}
	return osProfile, seriesOS, nil
}

// StopInstances is specified in the InstanceBroker interface.
func (env *azureEnviron) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	if len(ids) == 0 {
		return nil
	}

	// First up, cancel the deployments. Then we can identify the resources
	// that need to be deleted without racing with their creation.
	var wg sync.WaitGroup
	var existing int
	cancelResults := make([]error, len(ids))
	for i, id := range ids {
		logger.Debugf("canceling deployment for instance %q", id)
		wg.Add(1)
		go func(i int, id instance.Id) {
			defer wg.Done()
			cancelResults[i] = errors.Annotatef(
				env.cancelDeployment(ctx, string(id)),
				"canceling deployment %q", id,
			)
		}(i, id)
	}
	wg.Wait()
	for _, err := range cancelResults {
		if err == nil {
			existing++
		} else if !errors.IsNotFound(err) {
			return err
		}
	}
	if existing == 0 {
		// None of the instances exist, so we can stop now.
		return nil
	}

	// List network interfaces and public IP addresses.
	instanceNics, err := env.instanceNetworkInterfaces(
		ctx,
		env.resourceGroup,
	)
	if err != nil {
		return errors.Trace(err)
	}
	instancePips, err := env.instancePublicIPAddresses(
		ctx,
		env.resourceGroup,
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Delete the deployments, virtual machines, and related armresources.
	deleteResults := make([]error, len(ids))
	for i, id := range ids {
		if errors.IsNotFound(cancelResults[i]) {
			continue
		}
		// The deployment does not exist, so there's nothing more to do.
		logger.Debugf("deleting instance %q", id)
		wg.Add(1)
		go func(i int, id instance.Id) {
			defer wg.Done()
			err := env.deleteVirtualMachine(
				ctx,
				id,
				instanceNics[id],
				instancePips[id],
			)
			deleteResults[i] = errors.Annotatef(
				err, "deleting instance %q", id,
			)
		}(i, id)
	}
	wg.Wait()
	for _, err := range deleteResults {
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}

	return nil
}

// cancelDeployment cancels a template deployment.
func (env *azureEnviron) cancelDeployment(ctx context.ProviderCallContext, name string) error {
	logger.Debugf("- canceling deployment %q", name)
	deploy, err := env.deployClient()
	if err != nil {
		return errors.Trace(err)
	}
	_, err = deploy.Cancel(ctx, env.resourceGroup, name, nil)
	if err != nil {
		if errorutils.IsNotFoundError(err) {
			return errors.NewNotFound(err, fmt.Sprintf("deployment %q not found", name))
		}
		// Deployments can only canceled while they're running.
		if isDeployConflictError(err) {
			return nil
		}
		return errorutils.HandleCredentialError(errors.Annotatef(err, "canceling deployment %q", name), ctx)
	}
	return nil
}

func isDeployConflictError(err error) bool {
	if errorutils.IsConflictError(err) {
		code := errorutils.ErrorCode(err)
		if code == serviceErrorCodeDeploymentCannotBeCancelled ||
			code == serviceErrorCodeResourceGroupBeingDeleted {
			return true
		}
	}
	return false
}

// deleteVirtualMachine deletes a virtual machine and all of the resources that
// it owns, and any corresponding network security rules.
func (env *azureEnviron) deleteVirtualMachine(
	ctx context.ProviderCallContext,
	instId instance.Id,
	networkInterfaces []*armnetwork.Interface,
	publicIPAddresses []*armnetwork.PublicIPAddress,
) error {
	vmName := string(instId)

	// TODO(axw) delete resources concurrently.

	compute, err := env.computeClient()
	if err != nil {
		return errors.Trace(err)
	}
	// The VM must be deleted first, to release the lock on its armresources.
	logger.Debugf("- deleting virtual machine (%s)", vmName)
	poller, err := compute.BeginDelete(ctx, env.resourceGroup, vmName, nil)
	if err == nil {
		_, err = poller.PollUntilDone(ctx, nil)
	}
	if err != nil {
		if errorutils.MaybeInvalidateCredential(err, ctx) || !errorutils.IsNotFoundError(err) {
			return errors.Annotate(err, "deleting virtual machine")
		}
	}
	// Delete the managed OS disk.
	logger.Debugf("- deleting OS disk (%s)", vmName)
	disks, err := env.disksClient()
	if err != nil {
		return errors.Trace(err)
	}
	diskPoller, err := disks.BeginDelete(ctx, env.resourceGroup, vmName, nil)
	if err == nil {
		_, err = diskPoller.PollUntilDone(ctx, nil)
	}
	if err != nil {
		if errorutils.MaybeInvalidateCredential(err, ctx) || !errorutils.IsNotFoundError(err) {
			return errors.Annotate(err, "deleting OS disk")
		}
	}
	logger.Debugf("- deleting security rules (%s)", vmName)
	if err := deleteInstanceNetworkSecurityRules(
		ctx,
		env, instId, networkInterfaces,
	); err != nil {
		return errors.Annotate(err, "deleting network security rules")
	}

	logger.Debugf("- deleting network interfaces (%s)", vmName)
	interfaces, err := env.interfacesClient()
	if err != nil {
		return errors.Trace(err)
	}
	for _, nic := range networkInterfaces {
		nicName := toValue(nic.Name)
		logger.Tracef("deleting NIC %q", nicName)
		nicPoller, err := interfaces.BeginDelete(ctx, env.resourceGroup, nicName, nil)
		if err == nil {
			_, err = nicPoller.PollUntilDone(ctx, nil)
		}
		if err != nil {
			if errorutils.MaybeInvalidateCredential(err, ctx) || !errorutils.IsNotFoundError(err) {
				return errors.Annotate(err, "deleting NIC")
			}
		}
	}

	logger.Debugf("- deleting public IPs (%s)", vmName)
	publicAddresses, err := env.publicAddressesClient()
	if err != nil {
		return errors.Trace(err)
	}
	for _, pip := range publicIPAddresses {
		pipName := toValue(pip.Name)
		logger.Tracef("deleting public IP %q", pipName)
		ipPoller, err := publicAddresses.BeginDelete(ctx, env.resourceGroup, pipName, nil)
		if err == nil {
			_, err = ipPoller.PollUntilDone(ctx, nil)
		}
		if err != nil {
			if errorutils.MaybeInvalidateCredential(err, ctx) || !errorutils.IsNotFoundError(err) {
				return errors.Annotate(err, "deleting public IP")
			}
		}
	}

	// The deployment must be deleted last, or we risk leaking armresources.
	logger.Debugf("- deleting deployment (%s)", vmName)
	deploy, err := env.deployClient()
	if err != nil {
		return errors.Trace(err)
	}
	deploymentPoller, err := deploy.BeginDelete(ctx, env.resourceGroup, vmName, nil)
	if err == nil {
		_, err = deploymentPoller.PollUntilDone(ctx, nil)
	}
	if err != nil {
		ignoreError := isDeployConflictError(err) || errorutils.IsNotFoundError(err)
		if !ignoreError || errorutils.MaybeInvalidateCredential(err, ctx) {
			return errors.Annotate(err, "deleting deployment")
		}
	}
	return nil
}

// AdoptResources is part of the Environ interface.
func (env *azureEnviron) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, _ version.Number) error {
	resourceGroups, err := env.resourceGroupsClient()
	if err != nil {
		return errors.Trace(err)
	}
	err = env.updateGroupControllerTag(ctx, resourceGroups, env.resourceGroup, controllerUUID)
	if err != nil {
		// If we can't update the group there's no point updating the
		// contained resources - the group will be killed if the
		// controller is destroyed, taking the other things with it.
		return errors.Trace(err)
	}

	providers, err := env.providersClient()
	if err != nil {
		// If we can't update the group there's no point updating the
		// contained resources - the group will be killed if the
		// controller is destroyed, taking the other things with it.
		return errors.Trace(err)
	}
	apiVersions, err := collectAPIVersions(ctx, providers)
	if err != nil {
		return errors.Trace(err)
	}

	resources, err := env.resourcesClient()
	if err != nil {
		return errors.Trace(err)
	}
	var failed []string
	pager := resources.NewListByResourceGroupPager(env.resourceGroup, nil)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return errorutils.HandleCredentialError(errors.Annotate(err, "listing resources"), ctx)
		}
		for _, res := range next.Value {
			apiVersion := apiVersions[toValue(res.Type)]
			err := env.updateResourceControllerTag(
				ctx,
				resources,
				res, controllerUUID, apiVersion,
			)
			if err != nil {
				name := toValue(res.Name)
				logger.Errorf("error updating resource tags for %q: %v", name, err)
				failed = append(failed, name)
			}
		}
	}
	if len(failed) > 0 {
		return errors.Errorf("failed to update controller for some resources: %v", failed)
	}

	return nil
}

func (env *azureEnviron) updateGroupControllerTag(ctx context.ProviderCallContext, client *armresources.ResourceGroupsClient, groupName, controllerUUID string) error {
	group, err := client.Get(ctx, groupName, nil)
	if err != nil {
		return errorutils.HandleCredentialError(errors.Trace(err), ctx)
	}

	logger.Debugf(
		"updating resource group %s juju controller uuid to %s",
		toValue(group.Name), controllerUUID,
	)
	group.Tags[tags.JujuController] = to.Ptr(controllerUUID)

	// The Azure API forbids specifying ProvisioningState on the update.
	if group.Properties != nil {
		(*group.Properties).ProvisioningState = nil
	}

	_, err = client.CreateOrUpdate(ctx, groupName, group.ResourceGroup, nil)
	return errorutils.HandleCredentialError(errors.Annotatef(err, "updating controller for resource group %q", groupName), ctx)
}

func (env *azureEnviron) updateResourceControllerTag(
	ctx context.ProviderCallContext,
	client *armresources.Client,
	stubResource *armresources.GenericResourceExpanded,
	controllerUUID string,
	apiVersion string,
) error {
	stubTags := toMap(stubResource.Tags)
	if stubTags[tags.JujuController] == controllerUUID {
		// No update needed.
		return nil
	}

	// Need to get the resource individually to ensure that the
	// properties are populated.
	resource, err := client.GetByID(ctx, toValue(stubResource.ID), apiVersion, nil)
	if err != nil {
		return errorutils.HandleCredentialError(errors.Annotatef(err, "getting full resource %q", toValue(stubResource.Name)), ctx)
	}

	logger.Debugf("updating %s juju controller UUID to %s", toValue(stubResource.ID), controllerUUID)
	if resource.Tags == nil {
		resource.Tags = make(map[string]*string)
	}
	resource.Tags[tags.JujuController] = to.Ptr(controllerUUID)
	_, err = client.BeginCreateOrUpdateByID(
		ctx,
		toValue(stubResource.ID),
		apiVersion,
		resource.GenericResource,
		nil,
	)
	return errorutils.HandleCredentialError(errors.Annotatef(err, "updating controller for %q", toValue(resource.Name)), ctx)
}

var (
	runningInstStates = []armresources.ProvisioningState{
		armresources.ProvisioningStateCreating,
		armresources.ProvisioningStateUpdating,
		armresources.ProvisioningStateSucceeded,
	}
)

// Instances is specified in the Environ interface.
func (env *azureEnviron) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]instances.Instance, len(ids))
	// Make a series of requests to cope with eventual consistency.
	// Each request will attempt to add more instances to the requested set.
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			var need []instance.Id
			for i, inst := range insts {
				if inst == nil {
					need = append(need, ids[i])
				}
			}
			return env.gatherInstances(ctx, need, insts, env.resourceGroup, true)
		},
		IsFatalError: func(err error) bool {
			return err != environs.ErrPartialInstances
		},
		Attempts:    -1,
		Delay:       200 * time.Millisecond,
		MaxDuration: 5 * time.Second,
		Clock:       env.provider.config.RetryClock,
	})

	if err == environs.ErrPartialInstances {
		for _, inst := range insts {
			if inst != nil {
				return insts, environs.ErrPartialInstances
			}
		}
		return nil, environs.ErrNoInstances
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return insts, nil
}

// AllInstances is specified in the InstanceBroker interface.
func (env *azureEnviron) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return env.allInstances(ctx, env.resourceGroup, true, "")
}

// AllRunningInstances is specified in the InstanceBroker interface.
func (env *azureEnviron) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return env.allInstances(ctx, env.resourceGroup, true, "", runningInstStates...)
}

// gatherInstances tries to get information on each instance id
// whose corresponding insts slot is nil.
// This function returns environs.ErrPartialInstances if the
// insts slice has not been completely filled.
func (env *azureEnviron) gatherInstances(
	ctx context.ProviderCallContext,
	ids []instance.Id,
	insts []instances.Instance,
	resourceGroup string,
	refreshAddresses bool,
	instStates ...armresources.ProvisioningState,
) error {
	allInst, err := env.allInstances(ctx, resourceGroup, refreshAddresses, "", instStates...)
	if err != nil {
		return errors.Trace(err)
	}

	numFound := 0
	// For each requested id, add it to the returned instances
	// if we find it in the latest queried cloud instances.
	for i, id := range ids {
		if insts[i] != nil {
			numFound++
			continue
		}
		for _, inst := range allInst {
			if inst.Id() != id {
				continue
			}
			insts[i] = inst
			numFound++
		}
	}
	if numFound < len(ids) {
		return environs.ErrPartialInstances
	}
	return nil
}

// allInstances returns all instances in the environment
// with one of the specified instance states.
// If no instance states are specified, then return all instances.
func (env *azureEnviron) allInstances(
	ctx context.ProviderCallContext,
	resourceGroup string,
	refreshAddresses bool,
	controllerUUID string,
	instStates ...armresources.ProvisioningState,
) ([]instances.Instance, error) {
	// Instances may be queued for deployment but provisioning has not yet started.
	queued, err := env.allQueuedInstances(ctx, resourceGroup, controllerUUID != "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	provisioned, err := env.allProvisionedInstances(ctx, resourceGroup, controllerUUID, instStates...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Any provisioned or provisioning instances take precedence
	// over any entries in the queued slice.
	seenInst := set.NewStrings()
	azureInstances := provisioned
	for _, p := range provisioned {
		seenInst.Add(string(p.Id()))
	}
	for _, q := range queued {
		if seenInst.Contains(string(q.Id())) {
			continue
		}
		azureInstances = append(azureInstances, q)
	}

	// Get the instance addresses if needed.
	if len(azureInstances) > 0 && refreshAddresses {
		if err := env.setInstanceAddresses(
			ctx,
			resourceGroup,
			azureInstances,
		); err != nil {
			return nil, errors.Trace(err)
		}
	}

	var result []instances.Instance
	for _, inst := range azureInstances {
		result = append(result, inst)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Id() < result[j].Id()
	})
	return result, nil
}

// allQueuedInstances returns any pending or failed machine deployments
// in the given resource group.
func (env *azureEnviron) allQueuedInstances(
	ctx context.ProviderCallContext,
	resourceGroup string,
	controllerOnly bool,
) ([]*azureInstance, error) {
	deploy, err := env.deployClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var azureInstances []*azureInstance
	pager := deploy.NewListByResourceGroupPager(resourceGroup, nil)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			if errorutils.IsNotFoundError(err) {
				// This will occur if the resource group does not
				// exist, e.g. in a fresh hosted environment.
				return nil, nil
			}
			return nil, errorutils.HandleCredentialError(errors.Trace(err), ctx)
		}
		for _, deployment := range next.Value {
			deployProvisioningState := armresources.ProvisioningStateNotSpecified
			deployError := "Failed"
			if deployment.Properties != nil {
				deployProvisioningState = toValue(deployment.Properties.ProvisioningState)
				deployError = string(deployProvisioningState)
				if deployment.Properties.Error != nil {
					deployError = toValue(deployment.Properties.Error.Message)
					if deployment.Properties.Error.Details != nil && len(deployment.Properties.Error.Details) > 0 {
						deployError = toValue((deployment.Properties.Error.Details)[0].Message)
					}
				}
			}
			switch deployProvisioningState {
			case armresources.ProvisioningStateAccepted,
				armresources.ProvisioningStateCreating,
				armresources.ProvisioningStateRunning,
				armresources.ProvisioningStateFailed,
				armresources.ProvisioningStateCanceled,
				armresources.ProvisioningStateNotSpecified:
			default:
				continue
			}
			name := toValue(deployment.Name)
			if _, err := names.ParseMachineTag(name); err != nil {
				// Deployments we create for Juju machines are named
				// with the machine tag. We also create a "common"
				// deployment, so this will exclude that VM and any
				// other stray deployment armresources.
				continue
			}
			if deployment.Properties == nil || deployment.Properties.Dependencies == nil {
				continue
			}
			if controllerOnly && !isControllerDeployment(deployment) {
				continue
			}
			if len(deployment.Tags) == 0 {
				continue
			}
			if toValue(deployment.Tags[tags.JujuModel]) != env.Config().UUID() {
				continue
			}
			provisioningState := armresources.ProvisioningStateCreating
			switch deployProvisioningState {
			case armresources.ProvisioningStateFailed,
				armresources.ProvisioningStateCanceled:
				provisioningState = armresources.ProvisioningStateFailed
			}
			inst := &azureInstance{
				vmName:            name,
				provisioningState: provisioningState,
				provisioningError: deployError,
				env:               env,
			}
			azureInstances = append(azureInstances, inst)
		}
	}
	return azureInstances, nil
}

func isControllerDeployment(deployment *armresources.DeploymentExtended) bool {
	if deployment.Properties == nil {
		return false
	}
	for _, d := range deployment.Properties.Dependencies {
		if d.DependsOn == nil {
			continue
		}
		if toValue(d.ResourceType) != "Microsoft.Compute/virtualMachines" {
			continue
		}
		for _, on := range d.DependsOn {
			if toValue(on.ResourceType) != "Microsoft.Compute/availabilitySets" {
				continue
			}
			if toValue(on.ResourceName) == controllerAvailabilitySet {
				return true
			}
		}
	}
	return false
}

// allProvisionedInstances returns all of the instances
// in the given resource group.
func (env *azureEnviron) allProvisionedInstances(
	ctx context.ProviderCallContext,
	resourceGroup string,
	controllerUUID string,
	instStates ...armresources.ProvisioningState,
) ([]*azureInstance, error) {
	compute, err := env.computeClient()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var azureInstances []*azureInstance
	pager := compute.NewListPager(resourceGroup, nil)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			if errorutils.IsNotFoundError(err) {
				// This will occur if the resource group does not
				// exist, e.g. in a fresh hosted environment.
				return nil, nil
			}
			return nil, errorutils.HandleCredentialError(errors.Trace(err), ctx)
		}
		for _, vm := range next.Value {
			name := toValue(vm.Name)
			provisioningState := armresources.ProvisioningStateNotSpecified
			if vm.Properties != nil {
				provisioningState = armresources.ProvisioningState(toValue(vm.Properties.ProvisioningState))
			}
			if len(instStates) > 0 {
				haveState := false
				for _, wantState := range instStates {
					if provisioningState == wantState {
						haveState = true
						break
					}
				}
				if !haveState {
					continue
				}
			}
			if !isControllerInstance(vm, controllerUUID) {
				continue
			}
			if len(vm.Tags) == 0 {
				continue
			}
			if toValue(vm.Tags[tags.JujuModel]) != env.Config().UUID() {
				continue
			}
			inst := &azureInstance{
				vmName:            name,
				provisioningState: provisioningState,
				env:               env,
			}
			azureInstances = append(azureInstances, inst)
		}
	}
	return azureInstances, nil
}

func isControllerInstance(vm *armcompute.VirtualMachine, controllerUUID string) bool {
	if controllerUUID == "" {
		return true
	}
	vmTags := vm.Tags
	if v, ok := vmTags[tags.JujuIsController]; !ok || toValue(v) != "true" {
		return false
	}
	if v, ok := vmTags[tags.JujuController]; !ok || toValue(v) != controllerUUID {
		return false
	}
	return true
}

// Destroy is specified in the Environ interface.
func (env *azureEnviron) Destroy(ctx context.ProviderCallContext) error {
	logger.Debugf("destroying model %q", env.modelName)
	logger.Debugf("- deleting resource group %q", env.resourceGroup)
	if err := env.deleteResourceGroup(ctx, env.resourceGroup); err != nil {
		return errors.Trace(err)
	}
	// Resource groups are self-contained and fully encompass
	// all environ armresources. Once you delete the group, there
	// is nothing else to do.
	return nil
}

// DestroyController is specified in the Environ interface.
func (env *azureEnviron) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	logger.Debugf("destroying model %q", env.modelName)
	logger.Debugf("deleting resource groups")
	if err := env.deleteControllerManagedResourceGroups(ctx, controllerUUID); err != nil {
		return errors.Trace(err)
	}
	// Resource groups are self-contained and fully encompass
	// all environ armresources. Once you delete the group, there
	// is nothing else to do.
	return nil
}

func (env *azureEnviron) deleteControllerManagedResourceGroups(ctx context.ProviderCallContext, controllerUUID string) error {
	resourceGroups, err := env.resourceGroupsClient()
	if err != nil {
		return errors.Trace(err)
	}
	filter := fmt.Sprintf(
		"tagName eq '%s' and tagValue eq '%s'",
		tags.JujuController, controllerUUID,
	)
	pager := resourceGroups.NewListPager(&armresources.ResourceGroupsClientListOptions{
		Filter: to.Ptr(filter),
	})
	var groupNames []*string
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return errorutils.HandleCredentialError(errors.Annotate(err, "listing resource groups"), ctx)
		}
		// Walk all the pages of results so we can get a total list of groups to remove.
		for _, result := range next.Value {
			groupNames = append(groupNames, result.Name)
		}
	}
	// Deleting groups can take a long time, so make sure they are
	// deleted in parallel.
	var wg sync.WaitGroup
	errs := make([]error, len(groupNames))
	for i, name := range groupNames {
		groupName := toValue(name)
		logger.Debugf("  - deleting resource group %q", groupName)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := env.deleteResourceGroup(ctx, groupName); err != nil {
				errs[i] = errors.Annotatef(
					err, "deleting resource group %q", groupName,
				)
			}
		}(i)
	}
	wg.Wait()

	// If there is just one error, return it. If there are multiple,
	// then combine their messages.
	var nonNilErrs []error
	for _, err := range errs {
		if err != nil {
			nonNilErrs = append(nonNilErrs, err)
		}
	}
	switch len(nonNilErrs) {
	case 0:
		return nil
	case 1:
		return nonNilErrs[0]
	}
	combined := make([]string, len(nonNilErrs))
	for i, err := range nonNilErrs {
		combined[i] = err.Error()
	}
	return errors.New(strings.Join(combined, "; "))
}

func (env *azureEnviron) deleteResourceGroup(ctx context.ProviderCallContext, resourceGroup string) error {
	// For user specified, existing resource groups, delete the contents, not the group.
	if env.config.resourceGroupName != "" {
		return env.deleteResourcesInGroup(ctx, resourceGroup)
	}
	resourceGroups, err := env.resourceGroupsClient()
	if err != nil {
		return errors.Trace(err)
	}
	poller, err := resourceGroups.BeginDelete(ctx, resourceGroup, nil)
	if err == nil {
		_, err = poller.PollUntilDone(ctx, nil)
	}
	if err != nil {
		if errorutils.MaybeInvalidateCredential(err, ctx) || !errorutils.IsNotFoundError(err) {
			return errors.Annotatef(err, "deleting resource group %q", resourceGroup)
		}
	}
	return nil
}

func (env *azureEnviron) deleteResourcesInGroup(ctx context.ProviderCallContext, resourceGroup string) (err error) {
	logger.Debugf("deleting all resources in %s", resourceGroup)

	defer func() {
		err = errorutils.HandleCredentialError(err, ctx)
	}()

	// Find all the resources tagged as belonging to this model.
	filter := fmt.Sprintf("tagName eq '%s' and tagValue eq '%s'", tags.JujuModel, env.config.UUID())
	resourceItems, err := env.getModelResources(ctx, resourceGroup, filter)
	if err != nil {
		return errors.Trace(err)
	}

	// Older APIs can ignore the filter above, so query the hard way just in case.
	if len(resourceItems) == 0 {
		resourceItems, err = env.getModelResources(ctx, resourceGroup, filter)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// These will be deleted as part of stopping the instance below.
	machineResourceTypes := set.NewStrings(
		"Microsoft.Compute/virtualMachines",
		"Microsoft.Compute/disks",
		"Microsoft.Network/publicIPAddresses",
		"Microsoft.Network/networkInterfaces",
	)

	var (
		instIds        []instance.Id
		vaultNames     []string
		otherResources []*armresources.GenericResourceExpanded
	)
	for _, r := range resourceItems {
		rType := toValue(r.Type)
		logger.Debugf("resource to delete: %v (%v)", toValue(r.Name), rType)
		// Vault resources are handled by a separate client.
		if rType == "Microsoft.KeyVault/vaults" {
			vaultNames = append(vaultNames, toValue(r.Name))
			continue
		}
		if rType == "Microsoft.Compute/virtualMachines" {
			instIds = append(instIds, instance.Id(toValue(r.Name)))
			continue
		}
		if !machineResourceTypes.Contains(rType) {
			otherResources = append(otherResources, r)
		}
	}

	// Stopping instances will also remove most of their dependent armresources.
	err = env.StopInstances(ctx, instIds...)
	if err != nil {
		return errors.Annotatef(err, "deleting machine instances %q", instIds)
	}

	// Loop until all remaining resources are deleted.
	// For safety, add an upper retry limit; in reality, this will never be hit.
	remainingResources := otherResources
	retries := 0
	for len(remainingResources) > 0 && retries < 10 {
		remainingResources, err = env.deleteResources(ctx, remainingResources)
		if err != nil {
			return errors.Trace(err)
		}
		retries++
	}
	if len(remainingResources) > 0 {
		logger.Warningf("could not delete all Azure resources, remaining: %v", remainingResources)
	}

	// Lastly delete the vault armresources.
	for _, vaultName := range vaultNames {
		if err := env.deleteVault(ctx, vaultName); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (env *azureEnviron) getModelResources(sdkCtx stdcontext.Context, resourceGroup, modelFilter string) ([]*armresources.GenericResourceExpanded, error) {
	resources, err := env.resourcesClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var resourceItems []*armresources.GenericResourceExpanded
	pager := resources.NewListByResourceGroupPager(resourceGroup, &armresources.ClientListByResourceGroupOptions{
		Filter: to.Ptr(modelFilter),
	})
	for pager.More() {
		next, err := pager.NextPage(sdkCtx)
		if err != nil {
			return nil, errors.Annotate(err, "listing resources to delete")
		}
		for _, res := range next.Value {
			// If no modelFilter specified, we need to check that the resource
			// belongs to this model.
			if modelFilter == "" {
				fullRes, err := resources.GetByID(sdkCtx, toValue(res.ID), computeAPIVersion, nil)
				if err != nil {
					return nil, errors.Trace(err)
				}
				if env.config.UUID() != toValue(fullRes.Tags[tags.JujuModel]) {
					continue
				}
			}
			resourceItems = append(resourceItems, res)
		}
	}
	return resourceItems, nil
}

// deleteResources deletes the specified resources, returning any that
// cannot be deleted because they are in use.
func (env *azureEnviron) deleteResources(sdkCtx stdcontext.Context, toDelete []*armresources.GenericResourceExpanded) ([]*armresources.GenericResourceExpanded, error) {
	logger.Debugf("deleting %d resources", len(toDelete))

	var remainingResources []*armresources.GenericResourceExpanded
	var wg sync.WaitGroup
	deleteResults := make([]error, len(toDelete))
	for i, res := range toDelete {
		id := toValue(res.ID)
		logger.Debugf("- deleting resource %q", id)
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			resources, err := env.resourcesClient()
			if err != nil {
				deleteResults[i] = err
				return
			}
			poller, err := resources.BeginDeleteByID(sdkCtx, id, computeAPIVersion, nil)
			if err == nil {
				_, err = poller.PollUntilDone(sdkCtx, nil)
			}
			if err != nil {
				if errorutils.IsNotFoundError(err) {
					return
				}
				// If the resource is in use, don't error, just queue it up for another pass.
				if strings.HasPrefix(errorutils.ErrorCode(err), "InUse") {
					remainingResources = append(remainingResources, toDelete[i])
				} else {
					deleteResults[i] = errors.Annotatef(err, "deleting resource %q: %v", id, err)
				}
				return
			}
		}(i, id)
	}
	wg.Wait()

	var errStrings []string
	for i, err := range deleteResults {
		if err != nil && !errors.IsNotFound(err) {
			msg := fmt.Sprintf("error deleting resource %q: %#v", toValue(toDelete[i].ID), err)
			errStrings = append(errStrings, msg)
		}
	}
	if len(errStrings) > 0 {
		return nil, errors.Annotate(errors.New(strings.Join(errStrings, "\n")), "deleting resources")
	}
	return remainingResources, nil
}

// Provider is specified in the Environ interface.
func (env *azureEnviron) Provider() environs.EnvironProvider {
	return env.provider
}

// resourceGroupName returns the name of the model's resource group to use.
// It may be that a legacy group name is already in use, so use that if present.
func (env *azureEnviron) resourceGroupName(ctx stdcontext.Context, modelTag names.ModelTag, modelName string) (string, error) {
	resourceGroups, err := env.resourceGroupsClient()
	if err != nil {
		return "", errors.Trace(err)
	}
	// First look for a resource group name with the full model UUID.
	legacyName := legacyResourceGroupName(modelTag, modelName)
	g, err := resourceGroups.Get(ctx, legacyName, nil)
	if err == nil {
		logger.Debugf("using existing legacy resource group %q for model %q", legacyName, modelName)
		return legacyName, nil
	}
	if !errorutils.IsNotFoundError(err) {
		return "", errors.Trace(err)
	}

	logger.Debugf("legacy resource group name doesn't exist, using short name")
	resourceGroup := resourceGroupName(modelTag, modelName)
	g, err = resourceGroups.Get(ctx, resourceGroup, nil)
	if err == nil {
		mTag, ok := g.Tags[tags.JujuModel]
		if !ok || toValue(mTag) != modelTag.Id() {
			// This should never happen in practice - combination of model name and first 8
			// digits of UUID should be unique.
			return "", errors.Errorf("unexpected model UUID on resource group %q; expected %q, got %q", resourceGroup, modelTag.Id(), toValue(mTag))
		}
		return resourceGroup, nil
	}
	if errorutils.IsNotFoundError(err) {
		return resourceGroup, nil
	}
	return "", errors.Trace(err)
}

// resourceGroupName returns the name of the environment's resource group.
func legacyResourceGroupName(modelTag names.ModelTag, modelName string) string {
	return fmt.Sprintf("juju-%s-%s", modelName, resourceName(modelTag))
}

// resourceGroupName returns the name of the environment's resource group.
func resourceGroupName(modelTag names.ModelTag, modelName string) string {
	// The first chunk of the UUID string plus model name should be good enough.
	return fmt.Sprintf("juju-%s-%s", modelName, modelTag.Id()[:8])
}

// resourceName returns the string to use for a resource's Name tag,
// to help users identify Juju-managed resources in the Azure portal.
//
// Since resources are grouped under resource groups, we just use the
// tag.
func resourceName(tag names.Tag) string {
	return tag.String()
}

// getInstanceTypes gets the instance types available for the configured
// location, keyed by name.
func (env *azureEnviron) getInstanceTypes(ctx context.ProviderCallContext) (map[string]instances.InstanceType, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	instanceTypes, err := env.getInstanceTypesLocked(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting instance types")
	}
	return instanceTypes, nil
}

// getInstanceTypesLocked returns the instance types for Azure, by listing the
// role sizes available to the subscription.
func (env *azureEnviron) getInstanceTypesLocked(ctx context.ProviderCallContext) (map[string]instances.InstanceType, error) {
	if env.instanceTypes != nil {
		return env.instanceTypes, nil
	}

	skus, err := env.resourceSKUsClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	instanceTypes := make(map[string]instances.InstanceType)
	pager := skus.NewListPager(nil)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errorutils.HandleCredentialError(errors.Annotate(err, "listing VM sizes"), ctx)
		}
	nextResource:
		for _, resource := range next.Value {
			if resource.ResourceType == nil || *resource.ResourceType != "virtualMachines" {
				continue
			}
			for _, r := range resource.Restrictions {
				if toValue(r.ReasonCode) == armcompute.ResourceSKURestrictionsReasonCodeNotAvailableForSubscription {
					continue nextResource
				}
			}
			locationOk := false
			if resource.Locations != nil {
				for _, loc := range resource.Locations {
					if strings.ToLower(toValue(loc)) == env.location {
						locationOk = true
						break
					}
				}
			}
			if !locationOk {
				continue
			}
			var (
				cores    *int32
				mem      *int32
				rootDisk *int32
			)
			for _, capability := range resource.Capabilities {
				if capability.Name == nil || capability.Value == nil {
					continue
				}
				switch toValue(capability.Name) {
				case "MemoryGB":
					memValue, _ := strconv.ParseFloat(*capability.Value, 32)
					mem = to.Ptr(int32(1024 * memValue))
				case "vCPUsAvailable", "vCPUs":
					coresValue, _ := strconv.Atoi(*capability.Value)
					cores = to.Ptr(int32(coresValue))
				case "OSVhdSizeMB":
					rootDiskValue, _ := strconv.Atoi(*capability.Value)
					rootDisk = to.Ptr(int32(rootDiskValue))
				}
			}
			instanceType := newInstanceType(armcompute.VirtualMachineSize{
				Name:           resource.Name,
				NumberOfCores:  cores,
				OSDiskSizeInMB: rootDisk,
				MemoryInMB:     mem,
			})
			instanceTypes[instanceType.Name] = instanceType
			// Create aliases for standard role sizes.
			if strings.HasPrefix(instanceType.Name, "Standard_") {
				instanceTypes[instanceType.Name[len("Standard_"):]] = instanceType
			}
		}
	}
	env.instanceTypes = instanceTypes
	return instanceTypes, nil
}

// Region is specified in the HasRegion interface.
func (env *azureEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   env.cloud.Region,
		Endpoint: env.cloud.Endpoint,
	}, nil
}
