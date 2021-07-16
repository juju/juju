// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute" // This is the version supporting disk encryption sets.
	keyvaultservices "github.com/Azure/azure-sdk-for-go/services/keyvault/2016-10-01/keyvault"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/mgmt/2018-02-14/keyvault"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-06-01/resources"
	legacystorage "github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2017-10-01/storage" // Pin this legacy storage API to 2017-10-01 since it's only used for unmanaged storage in models was created in 2.2 or earlier.
	azurestorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/utils/v2/arch"
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
	internalazurestorage "github.com/juju/juju/provider/azure/internal/azurestorage"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	"github.com/juju/juju/provider/azure/internal/tracing"
	"github.com/juju/juju/provider/azure/internal/useragent"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools"
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

	computeAPIVersion = "2019-07-01"
	networkAPIVersion = "2018-08-01"
	// Note: do not upgrade this storage API anymore because Juju uses managed storage since 2.3 and this API is only used
	// for models upgraded from 2.2.
	// Upgrading the storage API may break those upgraded old models.
	storageAPIVersion = "2017-10-01"
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

	// storageEndpoint is the Azure storage endpoint. This is the host
	// portion of the storage endpoint URL only; use this instead of
	// cloud.StorageEndpoint in API calls.
	storageEndpoint string

	// resourceGroup is the name of the Resource Group in the Azure
	// subscription that corresponds to the environment.
	resourceGroup string

	// modelName is the name of the model.
	modelName string

	// authorizer is the authorizer we use for Azure.
	authorizer *cloudSpecAuth

	compute            compute.BaseClient
	disk               compute.BaseClient
	vault              keyvault.BaseClient
	vaultKey           keyvaultservices.BaseClient
	resources          resources.BaseClient
	storage            legacystorage.BaseClient
	network            network.BaseClient
	storageClient      azurestorage.Client
	storageAccountName string

	mu                     sync.Mutex
	config                 *azureModelConfig
	instanceTypes          map[string]instances.InstanceType
	storageAccount         **legacystorage.Account
	storageAccountKey      *legacystorage.AccountKey
	commonResourcesCreated bool
}

var _ environs.Environ = (*azureEnviron)(nil)

// SetCloudSpec is specified in the environs.Environ interface.
func (env *azureEnviron) SetCloudSpec(_ stdcontext.Context, cloud environscloudspec.CloudSpec) error {
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

	if err := env.initEnviron(); err != nil {
		return errors.Trace(err)
	}

	cfg := env.config
	if env.resourceGroup == "" {
		env.resourceGroup = cfg.resourceGroupName
	}
	// If no user specified resource group, make one from the model UUID.
	if env.resourceGroup == "" {
		modelTag := names.NewModelTag(cfg.UUID())
		if env.resourceGroup, err = env.resourceGroupName(modelTag, cfg.Name()); err != nil {
			return errors.Trace(err)
		}
	}
	env.modelName = cfg.Name()

	// We need a deterministic storage account name, so that we can
	// defer creation of the storage account to the VM deployment,
	// and retain the ability to create multiple deployments in
	// parallel.
	//
	// We use the last 20 non-hyphen hex characters of the model's
	// UUID as the storage account name, prefixed with "juju". The
	// probability of clashing with another storage account should
	// be negligible.
	uuidAlphaNumeric := strings.Replace(env.config.Config.UUID(), "-", "", -1)
	env.storageAccountName = "juju" + uuidAlphaNumeric[len(uuidAlphaNumeric)-20:]

	return nil
}

func (env *azureEnviron) initEnviron() error {
	credAttrs := env.cloud.Credential.Attributes()
	env.subscriptionId = credAttrs[credAttrSubscriptionId]
	env.authorizer = &cloudSpecAuth{
		cloud:  env.cloud,
		sender: env.provider.config.Sender,
	}

	env.compute = compute.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	env.disk = compute.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	env.vault = keyvault.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	env.resources = resources.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	env.storage = legacystorage.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	env.network = network.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	clients := map[string]*autorest.Client{
		"azure.compute":   &env.compute.Client,
		"azure.disk":      &env.disk.Client,
		"azure.resources": &env.resources.Client,
		"azure.storage":   &env.storage.Client,
		"azure.network":   &env.network.Client,
		"azure.vault":     &env.vault.Client,
		"azure.keyvault":  &env.vaultKey.Client,
	}
	for id, client := range clients {
		useragent.UpdateClient(client)
		client.Authorizer = env.authorizer.auth()
		clientLogger := loggo.GetLogger(id)
		if env.provider.config.Sender != nil {
			client.Sender = env.provider.config.Sender
		}
		client.ResponseInspector = tracing.RespondDecorator(clientLogger)
		client.RequestInspector = tracing.PrepareDecorator(clientLogger)
		if env.provider.config.RequestInspector != nil {
			tracer := client.RequestInspector
			inspector := env.provider.config.RequestInspector
			client.RequestInspector = func(p autorest.Preparer) autorest.Preparer {
				p = tracer(p)
				p = inspector(p)
				return p
			}
		}
	}
	return nil
}

// PrepareForBootstrap is part of the Environ interface.
func (env *azureEnviron) PrepareForBootstrap(ctx environs.BootstrapContext, _ string) error {
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env, nil); err != nil {
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

	resourceGroupsClient := resources.GroupsClient{BaseClient: env.resources}
	if existingResourceGroup {
		logger.Debugf("using existing resource group %q for model %q", env.resourceGroup, env.modelName)
		g, err := resourceGroupsClient.Get(ctx, env.resourceGroup)
		if err != nil {
			return errorutils.HandleCredentialError(errors.Annotatef(err, "checking resource group %q", env.resourceGroup), ctx)
		}
		if region := to.String(g.Location); region != env.location {
			return errors.Errorf("cannot use resource group in region %q when operating in region %q", region, env.location)
		}
	} else {
		logger.Debugf("creating resource group %q for model %q", env.resourceGroup, env.modelName)
		if _, err := resourceGroupsClient.CreateOrUpdate(ctx, env.resourceGroup, resources.Group{
			Location: to.StringPtr(env.location),
			Tags:     *to.StringMapPtr(resourceTags),
		}); err != nil {
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

	// New models are not given a storage account. Initialise the
	// storage account pointer to a pointer to a nil pointer, so
	// "getStorageAccount" avoids making an API call.
	env.storageAccount = new(*legacystorage.Account)

	return nil
}

func (env *azureEnviron) createCommonResourceDeployment(
	ctx context.ProviderCallContext,
	tags map[string]string,
	rules []network.SecurityRule,
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

	// We perform this deployment asynchronously, to avoid blocking
	// the "juju add-model" command; Create is called synchronously.
	// Eventually we should have Create called asynchronously, but
	// until then we do this, and ensure that the deployment has
	// completed before we schedule additional deployments.
	deploymentsClient := resources.DeploymentsClient{BaseClient: env.resources}
	deploymentsClient.ResponseInspector = asyncCreationRespondDecorator(
		deploymentsClient.ResponseInspector,
	)
	template := armtemplates.Template{Resources: commonResources}
	if err := createDeployment(
		ctx,
		deploymentsClient,
		env.resourceGroup,
		"common", // deployment name
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
			constraints.Arch,
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
	instanceTypes, err := env.getInstanceTypesLocked(ctx)
	if err != nil {
		env.mu.Unlock()
		return nil, errors.Trace(err)
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

	// Identify the instance type and image to provision.
	instanceSpec, err := findInstanceSpec(
		ctx,
		compute.VirtualMachineImagesClient{BaseClient: env.compute},
		instanceTypes,
		&instances.InstanceConstraint{
			Region:      env.location,
			Series:      args.InstanceConfig.Series,
			Arches:      args.Tools.Arches(),
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

	// Windows images are 127GiB, and cannot be made smaller.
	const windowsMinRootDiskMB = 127 * 1024
	seriesOS := args.Tools.OneRelease()
	if seriesOS == strings.ToLower(os.Windows.String()) {
		if instanceSpec.InstanceType.RootDisk < windowsMinRootDiskMB {
			logger.Infof("root disk size has been increased to 127GiB")
			instanceSpec.InstanceType.RootDisk = windowsMinRootDiskMB
		}
	}

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
	stdCtx := stdcontext.Background()
	if err := env.createVirtualMachine(
		stdCtx, ctx, vmName, vmTags, envTags,
		instanceSpec, args, usePublicIP,
	); err != nil {
		logger.Errorf("creating instance failed, destroying: %v", err)
		if err := env.StopInstances(ctx, instance.Id(vmName)); err != nil {
			logger.Errorf("could not destroy failed virtual machine: %v", err)
		}
		return nil, errors.Annotatef(err, "creating virtual machine %q", vmName)
	}

	// Note: the instance is initialised without addresses to keep the
	// API chatter down. We will refresh the instance if we need to know
	// the addresses.
	inst := &azureInstance{vmName, compute.ProvisioningStateCreating, env, nil, nil}
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
	stdCtx stdcontext.Context,
	ctx context.ProviderCallContext,
	vmName string,
	vmTags, envTags map[string]string,
	instanceSpec *instances.InstanceSpec,
	args environs.StartInstanceParams,
	usePublicIP bool,
) error {
	deploymentsClient := resources.DeploymentsClient{
		BaseClient: env.resources,
	}
	instanceConfig := args.InstanceConfig
	controller := instanceConfig.Controller != nil
	apiPorts := make([]int, 0, 2)
	if controller {
		apiPorts = append(apiPorts, instanceConfig.Controller.Config.APIPort())
		if instanceConfig.Controller.Config.AutocertDNSName() != "" {
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
		if err := env.waitCommonResourcesCreated(); err != nil {
			return errors.Annotate(
				err, "waiting for common resources to be created",
			)
		}
	}

	maybeStorageAccount, err := env.getStorageAccount(ctx)
	if errors.IsNotFound(err) {
		// Only models created prior to Juju 2.3 will have a storage
		// account. Juju 2.3 onwards exclusively uses managed disks
		// for all new models, and handles both managed and unmanaged
		// disks for upgraded models.
		maybeStorageAccount = nil
	} else if err != nil {
		return errors.Trace(err)
	}

	osProfile, seriesOS, err := newOSProfile(
		vmName, instanceConfig,
		env.provider.config.RandomWindowsAdminPassword,
		env.provider.config.GenerateSSHKey,
	)
	if err != nil {
		return errors.Annotate(err, "creating OS profile")
	}
	storageProfile, err := newStorageProfile(
		vmName,
		maybeStorageAccount,
		env.config.storageAccountType,
		instanceSpec,
	)
	if err != nil {
		return errors.Annotate(err, "creating storage profile")
	}
	diskEncryptionID, err := env.diskEncryptionInfo(stdCtx, ctx, args.RootDisk, envTags)
	if err != nil {
		return common.ZoneIndependentError(errors.Annotate(err, "creating disk encryption info"))
	}
	if diskEncryptionID != "" && storageProfile.OsDisk.ManagedDisk != nil {
		storageProfile.OsDisk.ManagedDisk.DiskEncryptionSet = &compute.DiskEncryptionSetParameters{
			ID: to.StringPtr(diskEncryptionID),
		}
	}

	var availabilitySetSubResource *compute.SubResource
	availabilitySetName, err := availabilitySetName(
		vmName, vmTags, instanceConfig.Controller != nil,
	)
	if err != nil {
		return errors.Annotate(err, "getting availability set name")
	}
	if availabilitySetName != "" {
		availabilitySetId := fmt.Sprintf(
			`[resourceId('Microsoft.Compute/availabilitySets','%s')]`,
			availabilitySetName,
		)
		var (
			availabilitySetProperties  interface{}
			availabilityStorageOptions *armtemplates.Sku
		)
		if maybeStorageAccount == nil {
			// This model uses managed disks; we must create
			// the availability set as "aligned" to support
			// them.
			availabilitySetProperties = &compute.AvailabilitySetProperties{
				// Azure complains when the fault domain count
				// is not specified, even though it is meant
				// to be optional and default to the maximum.
				// The maximum depends on the location, and
				// there is no API to query it.
				PlatformFaultDomainCount: to.Int32Ptr(maxFaultDomains(env.location)),
			}
			// Availability needs to be 'Aligned' to support managed disks.
			availabilityStorageOptions = &armtemplates.Sku{Name: "Aligned"}
		}
		res = append(res, armtemplates.Resource{
			APIVersion: computeAPIVersion,
			Type:       "Microsoft.Compute/availabilitySets",
			Name:       availabilitySetName,
			Location:   env.location,
			Tags:       envTags,
			Properties: availabilitySetProperties,
			Sku:        availabilityStorageOptions,
		})
		availabilitySetSubResource = &compute.SubResource{
			ID: to.StringPtr(availabilitySetId),
		}
		vmDependsOn = append(vmDependsOn, availabilitySetId)
	}

	placementSubnetID, err := env.findPlacementSubnet(ctx, args.Placement)
	if err != nil {
		return common.ZoneIndependentError(err)
	}
	vnetId, subnetIds, err := env.networkInfoForInstance(ctx, args, bootstrapping, controller, placementSubnetID)
	if err != nil {
		return common.ZoneIndependentError(err)
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
		publicIPAddressAllocationMethod := network.Static
		if env.config.loadBalancerSkuName == string(network.LoadBalancerSkuNameBasic) {
			publicIPAddressAllocationMethod = network.Dynamic // preserve the settings that were used in Juju 2.4 and earlier
		}
		res = append(res, armtemplates.Resource{
			APIVersion: networkAPIVersion,
			Type:       "Microsoft.Network/publicIPAddresses",
			Name:       publicIPAddressName,
			Location:   env.location,
			Tags:       vmTags,
			Sku:        &armtemplates.Sku{Name: env.config.loadBalancerSkuName},
			Properties: &network.PublicIPAddressPropertiesFormat{
				PublicIPAddressVersion:   network.IPv4,
				PublicIPAllocationMethod: publicIPAddressAllocationMethod,
			},
		})
	}

	// Create one NIC per subnet. The first one is the primary and has
	// the public IP address if so configured.
	var nics []compute.NetworkInterfaceReference
	for i, subnetID := range subnetIds {
		primary := i == 0
		ipConfig := &network.InterfaceIPConfigurationPropertiesFormat{
			Primary:                   to.BoolPtr(primary),
			PrivateIPAllocationMethod: network.Dynamic,
			Subnet:                    &network.Subnet{ID: to.StringPtr(string(subnetID))},
		}
		if primary && usePublicIP {
			ipConfig.PublicIPAddress = &network.PublicIPAddress{
				ID: to.StringPtr(publicIPAddressId),
			}
			nicDependsOn = append(nicDependsOn, publicIPAddressId)
		}
		ipConfigName := "primary"
		if i > 0 {
			ipConfigName = fmt.Sprintf("interface-%d", i)
		}
		nicName := vmName + "-" + ipConfigName
		nicId := fmt.Sprintf(`[resourceId('Microsoft.Network/networkInterfaces', '%s')]`, nicName)
		ipConfigurations := []network.InterfaceIPConfiguration{{
			Name:                                     to.StringPtr(ipConfigName),
			InterfaceIPConfigurationPropertiesFormat: ipConfig,
		}}
		res = append(res, armtemplates.Resource{
			APIVersion: networkAPIVersion,
			Type:       "Microsoft.Network/networkInterfaces",
			Name:       nicName,
			Location:   env.location,
			Tags:       vmTags,
			Properties: &network.InterfacePropertiesFormat{
				IPConfigurations: &ipConfigurations,
			},
			DependsOn: nicDependsOn,
		})
		vmDependsOn = append(vmDependsOn, nicId)

		nics = append(nics, compute.NetworkInterfaceReference{
			ID: to.StringPtr(nicId),
			NetworkInterfaceReferenceProperties: &compute.NetworkInterfaceReferenceProperties{
				Primary: to.BoolPtr(primary),
			},
		})
	}

	res = append(res, armtemplates.Resource{
		APIVersion: computeAPIVersion,
		Type:       "Microsoft.Compute/virtualMachines",
		Name:       vmName,
		Location:   env.location,
		Tags:       vmTags,
		Properties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: compute.VirtualMachineSizeTypes(
					instanceSpec.InstanceType.Name,
				),
			},
			StorageProfile:  storageProfile,
			OsProfile:       osProfile,
			NetworkProfile:  &compute.NetworkProfile{NetworkInterfaces: &nics},
			AvailabilitySet: availabilitySetSubResource,
		},
		DependsOn: vmDependsOn,
	})

	// On Windows and CentOS, we must add the CustomScript VM
	// extension to run the CustomData script.
	switch seriesOS {
	case os.Windows, os.CentOS:
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
	// NOTE(axw) VMs take a long time to go to "Succeeded", so we do not
	// block waiting for them to be fully provisioned. This means we won't
	// return an error from StartInstance if the VM fails provisioning;
	// we will instead report the error via the instance's status.
	deploymentsClient.ResponseInspector = asyncCreationRespondDecorator(
		deploymentsClient.ResponseInspector,
	)
	if err := createDeployment(
		ctx,
		deploymentsClient,
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
func (env *azureEnviron) waitCommonResourcesCreated() error {
	env.mu.Lock()
	defer env.mu.Unlock()
	if env.commonResourcesCreated {
		return nil
	}
	deployment, err := env.waitCommonResourcesCreatedLocked()
	if err != nil {
		return errors.Trace(err)
	}
	env.commonResourcesCreated = true
	if deployment != nil {
		// Check if the common deployment created
		// a storage account. If it didn't, we can
		// avoid a query for the storage account.
		var hasStorageAccount bool
		if deployment.Properties.Providers != nil {
			for _, p := range *deployment.Properties.Providers {
				if to.String(p.Namespace) != "Microsoft.Storage" {
					continue
				}
				if p.ResourceTypes == nil {
					continue
				}
				for _, rt := range *p.ResourceTypes {
					if to.String(rt.ResourceType) != "storageAccounts" {
						continue
					}
					hasStorageAccount = true
					break
				}
				break
			}
		}
		if !hasStorageAccount {
			env.storageAccount = new(*legacystorage.Account)
		}
	}
	return nil
}

type deploymentIncompleteError struct {
	error
}

func (env *azureEnviron) waitCommonResourcesCreatedLocked() (*resources.DeploymentExtended, error) {
	deploymentsClient := resources.DeploymentsClient{env.resources}

	// Release the lock while we're waiting, to avoid blocking others.
	env.mu.Unlock()
	defer env.mu.Lock()

	// Wait for up to 5 minutes, with a 5 second polling interval,
	// for the "common" deployment to be in one of the terminal
	// states. The deployment typically takes only around 30 seconds,
	// but we allow for a longer duration to be defensive.
	var deployment *resources.DeploymentExtended
	sdkCtx := stdcontext.Background()
	waitDeployment := func() error {
		result, err := deploymentsClient.Get(sdkCtx, env.resourceGroup, "common")
		if err != nil {
			if result.StatusCode == http.StatusNotFound {
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

		state := result.Properties.ProvisioningState
		if state == resources.ProvisioningStateSucceeded {
			// The deployment has succeeded, so the resources are
			// ready for use.
			deployment = &result
			return nil
		}
		err = errors.Errorf("common resource deployment status is %q", state)
		switch state {
		case resources.ProvisioningStateCanceled,
			resources.ProvisioningStateFailed,
			resources.ProvisioningStateDeleted:
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
//  - if the machine is a controller, use the availability set name
//    "juju-controller";
//  - if the machine has units assigned, create an availability
//    name with a name based on the value of the tags.JujuUnitsDeployed tag
//    in vmTags, if it exists;
//  - otherwise, do not assign the machine to an availability set
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
	maybeStorageAccount *legacystorage.Account,
	storageAccountType string,
	instanceSpec *instances.InstanceSpec,
) (*compute.StorageProfile, error) {
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
	osDisk := &compute.OSDisk{
		Name:         to.StringPtr(osDiskName),
		CreateOption: compute.DiskCreateOptionTypesFromImage,
		Caching:      compute.CachingTypesReadWrite,
		DiskSizeGB:   to.Int32Ptr(int32(osDiskSizeGB)),
	}

	if maybeStorageAccount == nil {
		// This model uses managed disks.
		osDisk.ManagedDisk = &compute.ManagedDiskParameters{
			StorageAccountType: compute.StorageAccountTypes(storageAccountType),
		}
	} else {
		// This model uses unmanaged disks.
		osDiskVhdRoot := blobContainerURL(maybeStorageAccount, osDiskVHDContainer)
		vhdURI := osDiskVhdRoot + osDiskName + vhdExtension
		osDisk.Vhd = &compute.VirtualHardDisk{to.StringPtr(vhdURI)}
	}

	return &compute.StorageProfile{
		ImageReference: &compute.ImageReference{
			Publisher: to.StringPtr(publisher),
			Offer:     to.StringPtr(offer),
			Sku:       to.StringPtr(sku),
			Version:   to.StringPtr(vers),
		},
		OsDisk: osDisk,
	}, nil
}

func mibToGB(mib uint64) uint64 {
	b := float64(mib * 1024 * 1024)
	return uint64(b / (1000 * 1000 * 1000))
}

func newOSProfile(
	vmName string,
	instanceConfig *instancecfg.InstanceConfig,
	randomAdminPassword func() string,
	generateSSHKey func(string) (string, string, error),
) (*compute.OSProfile, os.OSType, error) {
	logger.Debugf("creating OS profile for %q", vmName)

	customData, err := providerinit.ComposeUserData(instanceConfig, nil, AzureRenderer{})
	if err != nil {
		return nil, os.Unknown, errors.Annotate(err, "composing user data")
	}

	osProfile := &compute.OSProfile{
		ComputerName: to.StringPtr(vmName),
		CustomData:   to.StringPtr(string(customData)),
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

		publicKeys := []compute.SSHPublicKey{{
			Path:    to.StringPtr("/home/ubuntu/.ssh/authorized_keys"),
			KeyData: to.StringPtr(authorizedKeys),
		}}
		osProfile.AdminUsername = to.StringPtr("ubuntu")
		osProfile.LinuxConfiguration = &compute.LinuxConfiguration{
			DisablePasswordAuthentication: to.BoolPtr(true),
			SSH:                           &compute.SSHConfiguration{PublicKeys: &publicKeys},
		}
	case os.Windows:
		osProfile.AdminUsername = to.StringPtr("JujuAdministrator")
		// A password is required by Azure, but we will never use it.
		// We generate something sufficiently long and random that it
		// should be infeasible to guess.
		osProfile.AdminPassword = to.StringPtr(randomAdminPassword())
		osProfile.WindowsConfiguration = &compute.WindowsConfiguration{
			ProvisionVMAgent:       to.BoolPtr(true),
			EnableAutomaticUpdates: to.BoolPtr(true),
			// TODO(?) add WinRM configuration here.
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

	maybeStorageClient, _, err := env.maybeGetStorageClient(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// List network interfaces and public IP addresses.
	instanceNics, err := instanceNetworkInterfaces(
		ctx,
		env.resourceGroup,
		network.InterfacesClient{env.network},
	)
	if err != nil {
		return errors.Trace(err)
	}
	instancePips, err := instancePublicIPAddresses(
		ctx,
		env.resourceGroup,
		network.PublicIPAddressesClient{BaseClient: env.network},
	)
	if err != nil {
		return errors.Trace(err)
	}

	// Delete the deployments, virtual machines, and related resources.
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
				maybeStorageClient,
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
	deploymentsClient := resources.DeploymentsClient{env.resources}
	logger.Debugf("- canceling deployment %q", name)
	cancelResult, err := deploymentsClient.Cancel(ctx, env.resourceGroup, name)
	if err != nil {
		if cancelResult.Response != nil {
			switch cancelResult.StatusCode {
			case http.StatusNotFound:
				return errors.NewNotFound(err, fmt.Sprintf("deployment %q not found", name))
			case http.StatusConflict:
				if err, ok := errorutils.ServiceError(err); ok {
					if err.Code == serviceErrorCodeDeploymentCannotBeCancelled ||
						err.Code == serviceErrorCodeResourceGroupBeingDeleted {
						// Deployments can only canceled while they're running.
						return nil
					}
				}
			}
		}
		return errorutils.HandleCredentialError(errors.Annotatef(err, "canceling deployment %q", name), ctx)
	}
	return nil
}

// deleteVirtualMachine deletes a virtual machine and all of the resources that
// it owns, and any corresponding network security rules.
func (env *azureEnviron) deleteVirtualMachine(
	ctx context.ProviderCallContext,
	instId instance.Id,
	maybeStorageClient internalazurestorage.Client,
	networkInterfaces []network.Interface,
	publicIPAddresses []network.PublicIPAddress,
) error {
	vmClient := compute.VirtualMachinesClient{env.compute}
	diskClient := compute.DisksClient{env.disk}
	nicClient := network.InterfacesClient{env.network}
	pipClient := network.PublicIPAddressesClient{env.network}
	deploymentsClient := resources.DeploymentsClient{env.resources}
	vmName := string(instId)

	// TODO(axw) delete resources concurrently.

	// The VM must be deleted first, to release the lock on its resources.
	logger.Debugf("- deleting virtual machine (%s)", vmName)
	vmErrMsg := "deleting virtual machine"
	vmFuture, err := vmClient.Delete(ctx, env.resourceGroup, vmName)
	if err != nil {
		if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResponse(vmFuture.Response()) {
			return errors.Annotate(err, vmErrMsg)
		}
	} else {
		err = vmFuture.WaitForCompletionRef(ctx, vmClient.Client)
		if err != nil {
			return errorutils.HandleCredentialError(errors.Annotate(err, vmErrMsg), ctx)
		}
		result, err := vmFuture.Result(vmClient)
		if err != nil {
			if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResult(result) {
				return errors.Annotate(err, vmErrMsg)
			}
		}
	}
	if maybeStorageClient != nil {
		logger.Debugf("- deleting OS VHD (%s)", vmName)
		blobClient := maybeStorageClient.GetBlobService()
		vhdContainer := blobClient.GetContainerReference(osDiskVHDContainer)
		vhdBlob := vhdContainer.Blob(vmName)
		_, err := vhdBlob.DeleteIfExists(nil)
		return errorutils.HandleCredentialError(errors.Annotate(err, "deleting OS VHD"), ctx)
	}

	// Delete the managed OS disk.
	logger.Debugf("- deleting OS disk (%s)", vmName)
	diskErrMsg := "deleting OS disk"
	diskFuture, err := diskClient.Delete(ctx, env.resourceGroup, vmName)
	if err != nil {
		if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResponse(diskFuture.Response()) {
			return errors.Annotate(err, diskErrMsg)
		}
	}
	if err == nil {
		err = diskFuture.WaitForCompletionRef(ctx, diskClient.Client)
		if err != nil {
			return errorutils.HandleCredentialError(errors.Annotate(err, diskErrMsg), ctx)
		}
		result, err := diskFuture.Result(diskClient)
		if err != nil {
			if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResult(result) {
				return errors.Annotate(err, diskErrMsg)
			}
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
	networkErrMsg := "deleting NIC"
	for _, nic := range networkInterfaces {
		nicName := to.String(nic.Name)
		logger.Tracef("deleting NIC %q", nicName)
		nicFuture, err := nicClient.Delete(ctx, env.resourceGroup, nicName)
		if err != nil {
			if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResponse(nicFuture.Response()) {
				return errors.Annotate(err, networkErrMsg)
			}
		} else {
			err = nicFuture.WaitForCompletionRef(ctx, nicClient.Client)
			if err != nil {
				return errorutils.HandleCredentialError(errors.Annotate(err, networkErrMsg), ctx)
			}
			result, err := nicFuture.Result(nicClient)
			if err != nil {
				if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResult(result) {
					return errors.Annotate(err, networkErrMsg)
				}
			}
		}
	}

	logger.Debugf("- deleting public IPs (%s)", vmName)
	ipErrMsg := "deleting public IP"
	for _, pip := range publicIPAddresses {
		pipName := to.String(pip.Name)
		logger.Tracef("deleting public IP %q", pipName)
		ipFuture, err := pipClient.Delete(ctx, env.resourceGroup, pipName)
		if err != nil {
			if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResponse(ipFuture.Response()) {
				return errors.Annotate(err, ipErrMsg)
			}
		} else {
			err = ipFuture.WaitForCompletionRef(ctx, pipClient.Client)
			if err != nil {
				return errorutils.HandleCredentialError(errors.Annotate(err, ipErrMsg), ctx)
			}
			result, err := ipFuture.Result(pipClient)
			if err != nil {
				if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResult(result) {
					return errors.Annotate(err, ipErrMsg)
				}
			}
		}
	}

	// The deployment must be deleted last, or we risk leaking resources.
	logger.Debugf("- deleting deployment (%s)", vmName)
	deploymentFuture, err := deploymentsClient.Delete(ctx, env.resourceGroup, vmName)
	deploymentErrMsg := "deleting deployment"
	if err != nil {
		if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResponse(deploymentFuture.Response()) {
			return errors.Annotate(err, deploymentErrMsg)
		}
	} else {
		err = deploymentFuture.WaitForCompletionRef(ctx, deploymentsClient.Client)
		if err != nil {
			return errorutils.HandleCredentialError(errors.Annotate(err, deploymentErrMsg), ctx)
		}
		deploymentResult, err := deploymentFuture.Result(deploymentsClient)
		if err != nil {
			if errorutils.MaybeInvalidateCredential(err, ctx) || !isNotFoundResult(deploymentResult) {
				return errors.Annotate(err, deploymentErrMsg)
			}
		}
	}
	return nil
}

// AdoptResources is part of the Environ interface.
func (env *azureEnviron) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, _ version.Number) error {
	groupClient := resources.GroupsClient{BaseClient: env.resources}

	err := env.updateGroupControllerTag(ctx, &groupClient, env.resourceGroup, controllerUUID)
	if err != nil {
		// If we can't update the group there's no point updating the
		// contained resources - the group will be killed if the
		// controller is destroyed, taking the other things with it.
		return errors.Trace(err)
	}

	apiVersions, err := collectAPIVersions(ctx, resources.ProvidersClient{BaseClient: env.resources})
	if err != nil {
		return errors.Trace(err)
	}

	resourceClient := resources.Client{BaseClient: env.resources}
	res, err := resourceClient.ListByResourceGroupComplete(ctx, env.resourceGroup, "", "", nil)
	if err != nil {
		return errorutils.HandleCredentialError(errors.Annotate(err, "listing resources"), ctx)
	}
	var failed []string
	for ; res.NotDone(); err = res.NextWithContext(ctx) {
		if err != nil {
			return errors.Annotate(err, "listing resources")
		}
		resource := res.Value()
		apiVersion := apiVersions[to.String(resource.Type)]
		err := env.updateResourceControllerTag(
			ctx,
			resourceClient,
			resource, controllerUUID, apiVersion,
		)
		if err != nil {
			name := to.String(resource.Name)
			logger.Errorf("error updating resource tags for %q: %v", name, err)
			failed = append(failed, name)
		}
	}
	if len(failed) > 0 {
		return errors.Errorf("failed to update controller for some resources: %v", failed)
	}

	return nil
}

func (env *azureEnviron) updateGroupControllerTag(ctx context.ProviderCallContext, client *resources.GroupsClient, groupName, controllerUUID string) error {
	sdkCtx := stdcontext.Background()
	group, err := client.Get(sdkCtx, groupName)
	if err != nil {
		return errorutils.HandleCredentialError(errors.Trace(err), ctx)
	}

	logger.Debugf(
		"updating resource group %s juju controller uuid to %s",
		to.String(group.Name), controllerUUID,
	)
	group.Tags[tags.JujuController] = to.StringPtr(controllerUUID)

	// The Azure API forbids specifying ProvisioningState on the update.
	if group.Properties != nil {
		(*group.Properties).ProvisioningState = nil
	}

	_, err = client.CreateOrUpdate(sdkCtx, groupName, group)
	return errorutils.HandleCredentialError(errors.Annotatef(err, "updating controller for resource group %q", groupName), ctx)
}

func (env *azureEnviron) updateResourceControllerTag(
	ctx context.ProviderCallContext,
	client resources.Client,
	stubResource resources.GenericResourceExpanded,
	controllerUUID string,
	apiVersion string,
) error {
	stubTags := to.StringMap(stubResource.Tags)
	if stubTags[tags.JujuController] == controllerUUID {
		// No update needed.
		return nil
	}

	// Need to get the resource individually to ensure that the
	// properties are populated.
	resource, err := client.GetByID(ctx, to.String(stubResource.ID), apiVersion)
	if err != nil {
		return errorutils.HandleCredentialError(errors.Annotatef(err, "getting full resource %q", to.String(stubResource.Name)), ctx)
	}

	logger.Debugf("updating %s juju controller UUID to %s", to.String(stubResource.ID), controllerUUID)
	resource.Tags[tags.JujuController] = to.StringPtr(controllerUUID)
	_, err = client.CreateOrUpdateByID(
		ctx,
		to.String(stubResource.ID),
		apiVersion,
		resource,
	)
	return errorutils.HandleCredentialError(errors.Annotatef(err, "updating controller for %q", to.String(resource.Name)), ctx)
}

var (
	runningInstStates = []compute.ProvisioningState{
		compute.ProvisioningStateCreating,
		compute.ProvisioningStateUpdating,
		compute.ProvisioningStateMigrating,
		compute.ProvisioningStateSucceeded,
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
	instStates ...compute.ProvisioningState,
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
	instStates ...compute.ProvisioningState,
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
		if err := setInstanceAddresses(
			ctx,
			resourceGroup,
			network.InterfacesClient{env.network},
			network.PublicIPAddressesClient{env.network},
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
	deploymentsClient := resources.DeploymentsClient{BaseClient: env.resources}
	deploymentsResult, err := deploymentsClient.ListByResourceGroupComplete(ctx, resourceGroup, "", nil)
	if err != nil {
		if isNotFoundResult(deploymentsResult.Response().Response) {
			// This will occur if the resource group does not
			// exist, e.g. in a fresh hosted environment.
			return nil, nil
		}
		return nil, errorutils.HandleCredentialError(errors.Trace(err), ctx)
	}
	if deploymentsResult.Response().IsEmpty() {
		return nil, nil
	}

	var azureInstances []*azureInstance
	for ; deploymentsResult.NotDone(); err = deploymentsResult.NextWithContext(ctx) {
		if err != nil {
			return nil, errors.Annotate(err, "listing resources")
		}
		deployment := deploymentsResult.Value()
		deployProvisioningState := resources.ProvisioningStateNotSpecified
		if deployment.Properties != nil {
			deployProvisioningState = deployment.Properties.ProvisioningState
		}
		switch deployProvisioningState {
		case resources.ProvisioningStateAccepted,
			resources.ProvisioningStateCreating,
			resources.ProvisioningStateRunning,
			resources.ProvisioningStateFailed,
			resources.ProvisioningStateCanceled,
			resources.ProvisioningStateNotSpecified:
		default:
			continue
		}
		name := to.String(deployment.Name)
		if _, err := names.ParseMachineTag(name); err != nil {
			// Deployments we create for Juju machines are named
			// with the machine tag. We also create a "common"
			// deployment, so this will exclude that VM and any
			// other stray deployment resources.
			continue
		}
		if deployment.Properties == nil || deployment.Properties.Dependencies == nil {
			continue
		}
		if controllerOnly && !isControllerDeployment(deployment) {
			continue
		}
		provisioningState := compute.ProvisioningStateCreating
		switch deployProvisioningState {
		case resources.ProvisioningStateFailed,
			resources.ProvisioningStateCanceled:
			provisioningState = compute.ProvisioningStateFailed
		}
		inst := &azureInstance{name, provisioningState, env, nil, nil}
		azureInstances = append(azureInstances, inst)
	}
	return azureInstances, nil
}

func isControllerDeployment(deployment resources.DeploymentExtended) bool {
	for _, d := range *deployment.Properties.Dependencies {
		if d.DependsOn == nil {
			continue
		}
		if to.String(d.ResourceType) != "Microsoft.Compute/virtualMachines" {
			continue
		}
		for _, on := range *d.DependsOn {
			if to.String(on.ResourceType) != "Microsoft.Compute/availabilitySets" {
				continue
			}
			if to.String(on.ResourceName) == controllerAvailabilitySet {
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
	instStates ...compute.ProvisioningState,
) ([]*azureInstance, error) {
	vmClient := compute.VirtualMachinesClient{BaseClient: env.compute}
	vmResult, err := vmClient.ListComplete(ctx, resourceGroup)
	if err != nil {
		if isNotFoundResult(vmResult.Response().Response) {
			// This will occur if the resource group does not
			// exist, e.g. in a fresh hosted environment.
			return nil, nil
		}
		return nil, errorutils.HandleCredentialError(errors.Trace(err), ctx)
	}
	if vmResult.Response().IsEmpty() {
		return nil, nil
	}

	var azureInstances []*azureInstance
	for ; vmResult.NotDone(); err = vmResult.NextWithContext(ctx) {
		if err != nil {
			return nil, errors.Annotate(err, "listing instances")
		}
		vm := vmResult.Value()
		name := to.String(vm.Name)
		provisioningState := compute.ProvisioningState(to.String(vm.ProvisioningState))
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
		inst := &azureInstance{name, provisioningState, env, nil, nil}
		azureInstances = append(azureInstances, inst)
	}
	return azureInstances, nil
}

func isControllerInstance(vm compute.VirtualMachine, controllerUUID string) bool {
	if controllerUUID == "" {
		return true
	}
	vmTags := vm.Tags
	if v, ok := vmTags[tags.JujuIsController]; !ok || to.String(v) != "true" {
		return false
	}
	if v, ok := vmTags[tags.JujuController]; !ok || to.String(v) != controllerUUID {
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
	// all environ resources. Once you delete the group, there
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
	// all environ resources. Once you delete the group, there
	// is nothing else to do.
	return nil
}

func (env *azureEnviron) deleteControllerManagedResourceGroups(ctx context.ProviderCallContext, controllerUUID string) error {
	filter := fmt.Sprintf(
		"tagName eq '%s' and tagValue eq '%s'",
		tags.JujuController, controllerUUID,
	)
	client := resources.GroupsClient{env.resources}
	result, err := client.List(ctx, filter, nil)
	if err != nil {
		return errorutils.HandleCredentialError(errors.Annotate(err, "listing resource groups"), ctx)
	}
	if result.Values() == nil {
		return nil
	}

	// Walk all the pages of results so we can get a total list of groups to remove.
	var groupNames []*string
	for ; result.NotDone(); err = result.NextWithContext(ctx) {
		if err != nil {
			return errors.Trace(err)
		}
		for _, group := range result.Values() {
			groupNames = append(groupNames, group.Name)
		}
	}
	// Deleting groups can take a long time, so make sure they are
	// deleted in parallel.
	var wg sync.WaitGroup
	errs := make([]error, len(groupNames))
	for i, name := range groupNames {
		groupName := to.String(name)
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
	client := resources.GroupsClient{env.resources}
	future, err := client.Delete(ctx, resourceGroup)
	if err != nil {
		err = errorutils.HandleCredentialError(err, ctx)
		if !isNotFoundResponse(future.Response()) {
			return errors.Annotatef(err, "deleting resource group %q", resourceGroup)
		}
		return nil
	}
	err = future.WaitForCompletionRef(ctx, client.Client)
	if err != nil {
		return errors.Annotatef(err, "deleting resource group %q", resourceGroup)
	}
	result, err := future.Result(client)
	if err != nil && !isNotFoundResult(result) {
		return errors.Annotatef(err, "deleting resource group %q", resourceGroup)
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
		otherResources []resources.GenericResourceExpanded
	)
	for _, r := range resourceItems {
		rType := to.String(r.Type)
		logger.Debugf("resource to delete: %v (%v)", to.String(r.Name), rType)
		// Vault resources are handled by a separate client.
		if rType == "Microsoft.KeyVault/vaults" {
			vaultNames = append(vaultNames, to.String(r.Name))
			continue
		}
		if rType == "Microsoft.Compute/virtualMachines" {
			instIds = append(instIds, instance.Id(to.String(r.Name)))
			continue
		}
		if !machineResourceTypes.Contains(rType) {
			otherResources = append(otherResources, r)
		}
	}

	// Stopping instances will also remove most of their dependent resources.
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

	// Lastly delete the vault resources.
	for _, vaultName := range vaultNames {
		if err := env.deleteVault(ctx, ctx, vaultName); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (env *azureEnviron) getModelResources(sdkCtx stdcontext.Context, resourceGroup, modelFilter string) ([]resources.GenericResourceExpanded, error) {
	resourcesClient := resources.Client{env.resources}
	result, err := resourcesClient.ListByResourceGroupComplete(sdkCtx, resourceGroup, modelFilter, "", nil)
	if err != nil {
		return nil, errors.Annotate(err, "listing resources to delete")
	}

	var resourceItems []resources.GenericResourceExpanded
	for result.NotDone() {
		res := result.Value()
		// If no modelFilter specified, we need to check that the resource
		// belongs to this model.
		if modelFilter == "" {
			fullRes, err := resourcesClient.GetByID(sdkCtx, to.String(res.ID), computeAPIVersion)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if env.config.UUID() != to.String(fullRes.Tags[tags.JujuModel]) {
				continue
			}
		}
		resourceItems = append(resourceItems, res)
		err = result.NextWithContext(sdkCtx)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return resourceItems, nil
}

// deleteResources deletes the specified resources, returning any that
// cannot be deleted because they are in use.
func (env *azureEnviron) deleteResources(sdkCtx stdcontext.Context, toDelete []resources.GenericResourceExpanded) ([]resources.GenericResourceExpanded, error) {
	logger.Debugf("deleting %d resources", len(toDelete))

	resourcesClient := resources.Client{env.resources}
	resourcesClient.ResponseInspector = errorutils.CheckForDetailedError

	var remainingResources []resources.GenericResourceExpanded
	var wg sync.WaitGroup
	deleteResults := make([]error, len(toDelete))
	for i, res := range toDelete {
		id := to.String(res.ID)
		logger.Debugf("- deleting resource %q", id)
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			result, err := resourcesClient.DeleteByID(sdkCtx, id, computeAPIVersion)
			if err != nil {
				// If the resource is in use, don't error, just queue it up for another pass.
				if se, ok := errorutils.ServiceError(err); ok && strings.HasPrefix(se.Code, "InUse") {
					remainingResources = append(remainingResources, toDelete[i])
				} else {
					deleteResults[i] = errors.Annotatef(err, "deleting resource %q: %v", id, err)
				}
				return
			}
			if err = result.WaitForCompletionRef(sdkCtx, resourcesClient.Client); err != nil {
				resp := result.Response()
				if !isNotFoundResponse(resp) {
					deleteResults[i] = errors.Annotatef(err, "deleting resource %q: %v", id, err)
				}
			}
		}(i, id)
	}
	wg.Wait()

	var errStrings []string
	for i, err := range deleteResults {
		if err != nil && !errors.IsNotFound(err) {
			msg := fmt.Sprintf("error deleting resource %q: %#v", to.String(toDelete[i].ID), err)
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
func (env *azureEnviron) resourceGroupName(modelTag names.ModelTag, modelName string) (string, error) {
	ctx := stdcontext.Background()
	resourceGroupsClient := resources.GroupsClient{env.resources}

	// First look for a resource group name with the full model UUID.
	legacyName := legacyResourceGroupName(modelTag, modelName)
	g, err := resourceGroupsClient.Get(ctx, legacyName)
	if err == nil {
		logger.Debugf("using existing legacy resource group %q for model %q", legacyName, modelName)
		return legacyName, nil
	}
	if !isNotFoundResult(g.Response) {
		return "", errors.Trace(err)
	}

	logger.Debugf("legacy resource group name doesn't exist, using short name")
	resourceGroup := resourceGroupName(modelTag, modelName)
	g, err = resourceGroupsClient.Get(ctx, resourceGroup)
	if err == nil {
		mTag, ok := g.Tags[tags.JujuModel]
		if !ok || to.String(mTag) != modelTag.Id() {
			// This should never happen in practice - combination of model name and first 8
			// digits of UUID should be unique.
			return "", errors.Errorf("unexpected model UUID on resource group %q; expected %q, got %q", resourceGroup, modelTag.Id(), to.String(mTag))
		}
		return resourceGroup, nil
	}
	if isNotFoundResult(g.Response) {
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

	client := compute.ResourceSkusClient{env.compute}

	res, err := client.ListComplete(ctx, "")
	if err != nil {
		return nil, errorutils.HandleCredentialError(errors.Annotate(err, "listing VM sizes"), ctx)
	}
	instanceTypes := make(map[string]instances.InstanceType)
	sdkCtx := stdcontext.Background()
nextResource:
	for ; res.NotDone(); err = res.NextWithContext(sdkCtx) {
		if err != nil {
			return nil, errors.Annotate(err, "listing resources")
		}
		resource := res.Value()
		if resource.ResourceType == nil || *resource.ResourceType != "virtualMachines" {
			continue
		}
		if resource.Restrictions != nil {
			for _, r := range *resource.Restrictions {
				if r.ReasonCode == compute.NotAvailableForSubscription {
					continue nextResource
				}
			}
		}
		locationOk := false
		if resource.Locations != nil {
			for _, loc := range *resource.Locations {
				if strings.ToLower(loc) == env.location {
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
		for _, capability := range *resource.Capabilities {
			if capability.Name == nil || capability.Value == nil {
				continue
			}
			switch *capability.Name {
			case "MemoryGB":
				memValue, _ := strconv.ParseFloat(*capability.Value, 32)
				mem = to.Int32Ptr(int32(1024 * memValue))
			case "vCPUsAvailable", "vCPUs":
				coresValue, _ := strconv.Atoi(*capability.Value)
				cores = to.Int32Ptr(int32(coresValue))
			case "OSVhdSizeMB":
				rootDiskValue, _ := strconv.Atoi(*capability.Value)
				rootDisk = to.Int32Ptr(int32(rootDiskValue))
			}
		}
		instanceType := newInstanceType(compute.VirtualMachineSize{
			Name:           resource.Name,
			NumberOfCores:  cores,
			OsDiskSizeInMB: rootDisk,
			MemoryInMB:     mem,
		})
		instanceTypes[instanceType.Name] = instanceType
		// Create aliases for standard role sizes.
		if strings.HasPrefix(instanceType.Name, "Standard_") {
			instanceTypes[instanceType.Name[len("Standard_"):]] = instanceType
		}
	}
	env.instanceTypes = instanceTypes
	return instanceTypes, nil
}

// maybeGetStorageClient returns the environment's storage client if it
// has one, and nil if it does not.
func (env *azureEnviron) maybeGetStorageClient(ctx stdcontext.Context) (internalazurestorage.Client, *legacystorage.Account, error) {
	storageClient, storageAccount, err := env.getStorageClient(ctx)
	if errors.IsNotFound(err) {
		// Only models created prior to Juju 2.3 will have a storage
		// account. Juju 2.3 onwards exclusively uses managed disks
		// for all new models, and handles both managed and unmanaged
		// disks for upgraded models.
		storageClient = nil
		storageAccount = nil
	} else if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return storageClient, storageAccount, nil
}

// getStorageClient queries the storage account key, and uses it to construct
// a new storage client.
func (env *azureEnviron) getStorageClient(ctx stdcontext.Context) (internalazurestorage.Client, *legacystorage.Account, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	storageAccount, err := env.getStorageAccountLocked(ctx)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting storage account")
	}
	storageAccountKey, err := env.getStorageAccountKeyLocked(
		to.String(storageAccount.Name), false,
	)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting storage account key")
	}
	client, err := getStorageClient(
		env.provider.config.NewStorageClient,
		env.storageEndpoint,
		storageAccount,
		storageAccountKey,
	)
	if err != nil {
		return nil, nil, errors.Annotate(err, "getting storage client")
	}
	return client, storageAccount, nil
}

// getStorageAccount returns the storage account for this environment's
// resource group.
func (env *azureEnviron) getStorageAccount(ctx stdcontext.Context) (*legacystorage.Account, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	return env.getStorageAccountLocked(ctx)
}

func (env *azureEnviron) getStorageAccountLocked(ctx stdcontext.Context) (*legacystorage.Account, error) {
	if env.storageAccount != nil {
		if *env.storageAccount == nil {
			return nil, errors.NotFoundf("storage account")
		}
		return *env.storageAccount, nil
	}
	client := legacystorage.AccountsClient{env.storage}
	account, err := client.GetProperties(ctx, env.resourceGroup, env.storageAccountName)
	if err != nil {
		if isNotFoundResult(account.Response) {
			// Remember that the account was not found
			// by storing a pointer to a nil pointer.
			env.storageAccount = new(*legacystorage.Account)
			return nil, errors.NewNotFound(err, fmt.Sprintf("storage account not found"))
		}
		return nil, errors.Trace(err)
	}
	env.storageAccount = new(*legacystorage.Account)
	*env.storageAccount = &account
	return &account, nil
}

// getStorageAccountKeysLocked returns a storage account key for this
// environment's storage account. If refresh is true, any cached key
// will be refreshed. This method assumes that env.mu is held.
func (env *azureEnviron) getStorageAccountKeyLocked(accountName string, refresh bool) (*legacystorage.AccountKey, error) {
	if !refresh && env.storageAccountKey != nil {
		return env.storageAccountKey, nil
	}
	client := legacystorage.AccountsClient{env.storage}
	key, err := getStorageAccountKey(client, env.resourceGroup, accountName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	env.storageAccountKey = key
	return key, nil
}

// Region is specified in the HasRegion interface.
func (env *azureEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   env.cloud.Region,
		Endpoint: env.cloud.Endpoint,
	}, nil
}
