// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	azurestorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	jujunetwork "github.com/juju/juju/network"
	internalazurestorage "github.com/juju/juju/provider/azure/internal/azurestorage"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

const (
	jujuMachineNameTag = tags.JujuTagPrefix + "machine-name"

	// defaultRootDiskSize is the default root disk size to give
	// to a VM, if none is specified.
	defaultRootDiskSize = 30 * 1024 // 30 GiB
)

type azureEnviron struct {
	// provider is the azureEnvironProvider used to open this environment.
	provider *azureEnvironProvider

	// cloud defines the cloud configuration for this environment.
	cloud environs.CloudSpec

	// location is the canonicalized location name. Use this instead
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

	// envName is the name of the environment.
	envName string

	// azure auth token, and management clients
	token *azure.ServicePrincipalToken

	compute       compute.ManagementClient
	resources     resources.ManagementClient
	storage       storage.ManagementClient
	network       network.ManagementClient
	storageClient azurestorage.Client

	mu                sync.Mutex
	config            *azureModelConfig
	instanceTypes     map[string]instances.InstanceType
	storageAccount    *storage.Account
	storageAccountKey *storage.AccountKey
}

var _ environs.Environ = (*azureEnviron)(nil)
var _ state.Prechecker = (*azureEnviron)(nil)

// newEnviron creates a new azureEnviron.
func newEnviron(
	provider *azureEnvironProvider,
	cloud environs.CloudSpec,
	cfg *config.Config,
) (*azureEnviron, error) {

	// The Azure storage code wants the endpoint host only, not the URL.
	storageEndpointURL, err := url.Parse(cloud.StorageEndpoint)
	if err != nil {
		return nil, errors.Annotate(err, "parsing storage endpoint URL")
	}

	env := azureEnviron{
		provider:        provider,
		cloud:           cloud,
		location:        canonicalLocation(cloud.Region),
		storageEndpoint: storageEndpointURL.Host,
	}
	if err := env.initEnviron(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := env.SetConfig(cfg); err != nil {
		return nil, errors.Trace(err)
	}
	modelTag := names.NewModelTag(cfg.UUID())
	env.resourceGroup = resourceGroupName(modelTag, cfg.Name())
	env.envName = cfg.Name()
	return &env, nil
}

func (env *azureEnviron) initEnviron() error {
	credAttrs := env.cloud.Credential.Attributes()
	env.subscriptionId = credAttrs[credAttrSubscriptionId]
	tenantId := credAttrs[credAttrTenantId]
	appId := credAttrs[credAttrAppId]
	appPassword := credAttrs[credAttrAppPassword]

	cloudEnv := azure.Environment{
		ActiveDirectoryEndpoint: env.cloud.IdentityEndpoint,
	}
	oauthConfig, err := cloudEnv.OAuthConfigForTenant(tenantId)
	if err != nil {
		return errors.Annotate(err, "getting OAuth configuration")
	}

	// We want to create a service principal token for the resource
	// manager endpoint. Azure demands that the URL end with a '/'.
	resource := env.cloud.Endpoint
	if !strings.HasSuffix(resource, "/") {
		resource += "/"
	}

	token, err := azure.NewServicePrincipalToken(
		*oauthConfig,
		appId,
		appPassword,
		resource,
	)
	if err != nil {
		return errors.Annotate(err, "constructing service principal token")
	}
	env.token = token
	if env.provider.config.Sender != nil {
		env.token.SetSender(env.provider.config.Sender)
	}

	env.compute = compute.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	env.resources = resources.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	env.storage = storage.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	env.network = network.NewWithBaseURI(env.cloud.Endpoint, env.subscriptionId)
	clients := map[string]*autorest.Client{
		"azure.compute":   &env.compute.Client,
		"azure.resources": &env.resources.Client,
		"azure.storage":   &env.storage.Client,
		"azure.network":   &env.network.Client,
	}
	for id, client := range clients {
		client.Authorizer = env.token
		logger := loggo.GetLogger(id)
		if env.provider.config.Sender != nil {
			client.Sender = env.provider.config.Sender
		}
		client.ResponseInspector = tracingRespondDecorator(logger)
		client.RequestInspector = tracingPrepareDecorator(logger)
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
func (env *azureEnviron) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Create is part of the Environ interface.
func (env *azureEnviron) Create(args environs.CreateParams) error {
	if err := verifyCredentials(env); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(env.initResourceGroup(args.ControllerUUID, nil))
}

// Bootstrap is part of the Environ interface.
func (env *azureEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {

	apiPort := args.ControllerConfig.APIPort()
	if err := env.initResourceGroup(args.ControllerConfig.ControllerUUID(), &apiPort); err != nil {
		return nil, errors.Annotate(err, "creating controller resource group")
	}
	result, err := common.Bootstrap(ctx, env, args)
	if err != nil {
		logger.Errorf("bootstrap failed, destroying model: %v", err)
		if err := env.Destroy(); err != nil {
			logger.Errorf("failed to destroy model: %v", err)
		}
		return nil, errors.Trace(err)
	}
	return result, nil
}

// initResourceGroup creates and initialises a resource group for this
// environment. The resource group will have a storage account, and
// internal network and subnet.
func (env *azureEnviron) initResourceGroup(controllerUUID string, apiPort *int) error {
	location := env.location
	resourceGroupsClient := resources.GroupsClient{env.resources}
	networkClient := env.network
	storageAccountsClient := storage.AccountsClient{env.storage}

	env.mu.Lock()
	tags := tags.ResourceTags(
		names.NewModelTag(env.config.Config.UUID()),
		names.NewControllerTag(controllerUUID),
		env.config,
	)
	storageAccountType := env.config.storageAccountType
	env.mu.Unlock()

	logger.Debugf("creating resource group %q", env.resourceGroup)
	if err := env.callAPI(func() (autorest.Response, error) {
		group, err := resourceGroupsClient.CreateOrUpdate(env.resourceGroup, resources.ResourceGroup{
			Location: to.StringPtr(location),
			Tags:     to.StringMapPtr(tags),
		})
		return group.Response, err
	}); err != nil {
		return errors.Annotate(err, "creating resource group")
	}

	// Create an internal network for all VMs in the
	// resource group to connect to.
	if _, err := createInternalVirtualNetwork(
		env.callAPI, networkClient,
		env.subscriptionId, env.resourceGroup,
		location, tags,
	); err != nil {
		return errors.Annotate(err, "creating virtual network")
	}

	// Create an network security group which will be
	// associated with all machines.
	if _, err := createInternalNetworkSecurityGroup(
		env.callAPI, networkClient,
		env.subscriptionId, env.resourceGroup,
		location, tags, apiPort,
	); err != nil {
		return errors.Annotate(err, "creating security group")
	}

	// Create a subnet to which all machines will be attached.
	if _, err := createInternalNetworkSubnet(
		env.callAPI, networkClient,
		env.subscriptionId, env.resourceGroup,
		internalSubnetName, internalSubnetPrefix,
		location, tags,
	); err != nil {
		return errors.Annotate(err, "creating subnet")
	}

	if apiPort != nil {
		// apiPort is non-nil only for the controller model. Create
		// a subnet to which controller machines will be attached.
		if _, err := createInternalNetworkSubnet(
			env.callAPI, networkClient,
			env.subscriptionId, env.resourceGroup,
			controllerSubnetName, controllerSubnetPrefix,
			location, tags,
		); err != nil {
			return errors.Annotate(err, "creating subnet")
		}
	}

	// Create a storage account for the resource group.
	if err := createStorageAccount(
		env.callAPI, storageAccountsClient, storageAccountType,
		env.resourceGroup, location, tags,
		env.provider.config.StorageAccountNameGenerator,
	); err != nil {
		return errors.Annotate(err, "creating storage account")
	}
	return nil
}

func createStorageAccount(
	callAPI callAPIFunc,
	client storage.AccountsClient,
	accountType string,
	resourceGroup string,
	location string,
	tags map[string]string,
	accountNameGenerator func() string,
) error {
	logger.Debugf("creating storage account (finding available name)")
	const maxAttempts = 10
	for remaining := maxAttempts; remaining > 0; remaining-- {
		accountName := accountNameGenerator()
		logger.Debugf("- checking storage account name %q", accountName)
		var result storage.CheckNameAvailabilityResult
		if err := callAPI(func() (autorest.Response, error) {
			var err error
			result, err = client.CheckNameAvailability(
				storage.AccountCheckNameAvailabilityParameters{
					Name: to.StringPtr(accountName),
					// Azure is a little inconsistent with when Type is
					// required. It's required here.
					Type: to.StringPtr("Microsoft.Storage/storageAccounts"),
				},
			)
			return result.Response, err
		}); err != nil {
			return errors.Annotate(err, "checking account name availability")
		}
		if !to.Bool(result.NameAvailable) {
			logger.Debugf(
				"%q is not available (%v): %v",
				accountName, result.Reason, result.Message,
			)
			continue
		}
		createParams := storage.AccountCreateParameters{
			Location: to.StringPtr(location),
			Tags:     to.StringMapPtr(tags),
			Sku: &storage.Sku{
				Name: storage.SkuName(accountType),
			},
		}
		logger.Debugf("- creating %q storage account %q", accountType, accountName)
		// TODO(axw) account creation can fail if the account name is
		// available, but contains profanity. We should retry a set
		// number of times even if creating fails.
		if err := callAPI(func() (autorest.Response, error) {
			return client.Create(resourceGroup, accountName, createParams, nil)
		}); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	return errors.New("could not find available storage account name")
}

// ControllerInstances is specified in the Environ interface.
func (env *azureEnviron) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
	// controllers are tagged with tags.JujuIsController, so just
	// list the instances in the controller resource group and pick
	// those ones out.
	instances, err := env.allInstances(env.resourceGroup, true)
	if err != nil {
		return nil, err
	}
	var ids []instance.Id
	for _, inst := range instances {
		azureInstance := inst.(*azureInstance)
		if toTags(azureInstance.Tags)[tags.JujuIsController] == "true" {
			ids = append(ids, inst.Id())
		}
	}
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
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
func (env *azureEnviron) ConstraintsValidator() (constraints.Validator, error) {
	instanceTypes, err := env.getInstanceTypes()
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
			constraints.CpuCores,
			constraints.Arch,
		},
	)
	return validator, nil
}

// PrecheckInstance is defined on the state.Prechecker interface.
func (env *azureEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" {
		return fmt.Errorf("unknown placement directive: %s", placement)
	}
	if !cons.HasInstanceType() {
		return nil
	}
	// Constraint has an instance-type constraint so let's see if it is valid.
	instanceTypes, err := env.getInstanceTypes()
	if err != nil {
		return err
	}
	for _, instanceType := range instanceTypes {
		if instanceType.Name == *cons.InstanceType {
			return nil
		}
	}
	return fmt.Errorf("invalid instance type %q", *cons.InstanceType)
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*azureEnviron) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *azureEnviron) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	if args.ControllerUUID == "" {
		return nil, errors.New("missing controller UUID")
	}

	location := env.location
	vmClient := compute.VirtualMachinesClient{env.compute}
	availabilitySetClient := compute.AvailabilitySetsClient{env.compute}
	networkClient := env.network
	vmImagesClient := compute.VirtualMachineImagesClient{env.compute}
	vmExtensionClient := compute.VirtualMachineExtensionsClient{env.compute}

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
	instanceTypes, err := env.getInstanceTypesLocked()
	if err != nil {
		env.mu.Unlock()
		return nil, errors.Trace(err)
	}
	storageAccount, err := env.getStorageAccountLocked(false)
	if err != nil {
		env.mu.Unlock()
		return nil, errors.Annotate(err, "getting storage account")
	}
	env.mu.Unlock()

	// If the user has not specified a root-disk size, then
	// set a sensible default.
	var rootDisk uint64
	if args.Constraints.RootDisk != nil {
		rootDisk = *args.Constraints.RootDisk
	} else {
		rootDisk = defaultRootDiskSize
		args.Constraints.RootDisk = &rootDisk
	}

	// Identify the instance type and image to provision.
	series := args.Tools.OneSeries()
	instanceSpec, err := findInstanceSpec(
		vmImagesClient,
		instanceTypes,
		&instances.InstanceConstraint{
			Region:      location,
			Series:      series,
			Arches:      args.Tools.Arches(),
			Constraints: args.Constraints,
		},
		imageStream,
	)
	if err != nil {
		return nil, err
	}
	if rootDisk < uint64(instanceSpec.InstanceType.RootDisk) {
		// The InstanceType's RootDisk is set to the maximum
		// OS disk size; override it with the user-specified
		// or default root disk size.
		instanceSpec.InstanceType.RootDisk = rootDisk
	}

	// Windows images are 127GiB, and cannot be made smaller.
	const windowsMinRootDiskMB = 127 * 1024
	seriesOS, err := jujuseries.GetOSFromSeries(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if seriesOS == os.Windows {
		if instanceSpec.InstanceType.RootDisk < windowsMinRootDiskMB {
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
	logger.Infof("picked tools %q", selectedTools[0].Version)

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

	vm, err := createVirtualMachine(
		env.subscriptionId, env.resourceGroup,
		location, vmName, vmTags, envTags,
		instanceSpec, args.InstanceConfig,
		args.DistributionGroup,
		env.Instances,
		storageAccount,
		networkClient, vmClient,
		availabilitySetClient, vmExtensionClient,
		env.callAPI,
	)
	if err != nil {
		logger.Errorf("creating instance failed, destroying: %v", err)
		if err := env.StopInstances(instance.Id(vmName)); err != nil {
			logger.Errorf("could not destroy failed virtual machine: %v", err)
		}
		return nil, errors.Annotatef(err, "creating virtual machine %q", vmName)
	}

	// Note: the instance is initialised without addresses to keep the
	// API chatter down. We will refresh the instance if we need to know
	// the addresses.
	inst := &azureInstance{vm, env, nil, nil}
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

// createVirtualMachine creates a virtual machine and related resources.
//
// All resources created are tagged with the specified "vmTags", so if
// this function fails then all resources can be deleted by tag.
func createVirtualMachine(
	subscriptionId, resourceGroup, location, vmName string,
	vmTags, envTags map[string]string,
	instanceSpec *instances.InstanceSpec,
	instanceConfig *instancecfg.InstanceConfig,
	distributionGroupFunc func() ([]instance.Id, error),
	instancesFunc func([]instance.Id) ([]instance.Instance, error),
	storageAccount *storage.Account,
	networkClient network.ManagementClient,
	vmClient compute.VirtualMachinesClient,
	availabilitySetClient compute.AvailabilitySetsClient,
	vmExtensionClient compute.VirtualMachineExtensionsClient,
	callAPI callAPIFunc,
) (compute.VirtualMachine, error) {

	storageProfile, err := newStorageProfile(
		vmName, instanceSpec, storageAccount,
	)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating storage profile")
	}

	osProfile, seriesOS, err := newOSProfile(vmName, instanceConfig)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating OS profile")
	}

	networkProfile, err := newNetworkProfile(
		callAPI, networkClient,
		vmName,
		instanceConfig.Controller != nil,
		subscriptionId,
		resourceGroup,
		location,
		vmTags,
	)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating network profile")
	}

	availabilitySetId, err := createAvailabilitySet(
		callAPI, availabilitySetClient,
		vmName, resourceGroup, location,
		vmTags, envTags,
		distributionGroupFunc, instancesFunc,
	)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating availability set")
	}

	logger.Debugf("- creating virtual machine")
	vm := compute.VirtualMachine{
		Location: to.StringPtr(location),
		Tags:     to.StringMapPtr(vmTags),
		Name:     to.StringPtr(vmName),
		Properties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: compute.VirtualMachineSizeTypes(
					instanceSpec.InstanceType.Name,
				),
			},
			StorageProfile: storageProfile,
			OsProfile:      osProfile,
			NetworkProfile: networkProfile,
			AvailabilitySet: &compute.SubResource{
				ID: to.StringPtr(availabilitySetId),
			},
		},
	}

	// NOTE(axw) VMs take a long time to go to "Succeeded", so we do not
	// block waiting for them to be fully provisioned. This means we won't
	// return an error from StartInstance if the VM fails provisioning;
	// we will instead report the error via the instance's status.
	vmClient.Client.ResponseInspector = asyncCreationRespondDecorator(
		vmClient.Client.ResponseInspector,
	)
	if err := callAPI(func() (autorest.Response, error) {
		return vmClient.CreateOrUpdate(resourceGroup, vmName, vm, nil)
	}); err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating virtual machine")
	}

	// On Windows and CentOS, we must add the CustomScript VM
	// extension to run the CustomData script.
	switch seriesOS {
	case os.Windows, os.CentOS:
		vmExtensionClient.Client.ResponseInspector = asyncCreationRespondDecorator(
			vmExtensionClient.Client.ResponseInspector,
		)
		if err := createVMExtension(
			callAPI, vmExtensionClient, seriesOS,
			resourceGroup, vmName, location, vmTags,
		); err != nil {
			return compute.VirtualMachine{}, errors.Annotate(
				err, "creating virtual machine extension",
			)
		}
	}
	return vm, nil
}

// createAvailabilitySet creates the availability set for a machine to use
// if it doesn't already exist, and returns the availability set's ID. The
// algorithm used for choosing the availability set is:
//  - if there is a distribution group, use the same availability set as
//    the instances in that group. Instances in the group may be in
//    different availability sets (when multiple services colocated on a
//    machine), so we pick one arbitrarily
//  - if there is no distribution group, create an availability name with
//    a name based on the value of the tags.JujuUnitsDeployed tag in vmTags,
//    if it exists
//  - if there are no units assigned to the machine, then use the "juju"
//    availability set
func createAvailabilitySet(
	callAPI callAPIFunc,
	client compute.AvailabilitySetsClient,
	vmName, resourceGroup, location string,
	vmTags, envTags map[string]string,
	distributionGroupFunc func() ([]instance.Id, error),
	instancesFunc func([]instance.Id) ([]instance.Instance, error),
) (string, error) {
	logger.Debugf("selecting availability set for %q", vmName)

	// First we check if there's a distribution group, and if so,
	// use the availability set of the first instance we find in it.
	var instanceIds []instance.Id
	if distributionGroupFunc != nil {
		var err error
		instanceIds, err = distributionGroupFunc()
		if err != nil {
			return "", errors.Annotate(
				err, "querying distribution group",
			)
		}
	}
	instances, err := instancesFunc(instanceIds)
	switch err {
	case nil, environs.ErrPartialInstances, environs.ErrNoInstances:
	default:
		return "", errors.Annotate(
			err, "querying distribution group instances",
		)
	}
	for _, instance := range instances {
		if instance == nil {
			continue
		}
		instance := instance.(*azureInstance)
		availabilitySetSubResource := instance.Properties.AvailabilitySet
		if availabilitySetSubResource == nil || availabilitySetSubResource.ID == nil {
			continue
		}
		logger.Debugf("- selecting availability set of %q", instance.Name)
		return to.String(availabilitySetSubResource.ID), nil
	}

	// We'll have to create an availability set. Use the name of one of the
	// services assigned to the machine.
	availabilitySetName := "juju"
	if unitNames, ok := vmTags[tags.JujuUnitsDeployed]; ok {
		for _, unitName := range strings.Fields(unitNames) {
			if !names.IsValidUnit(unitName) {
				continue
			}
			serviceName, err := names.UnitApplication(unitName)
			if err != nil {
				return "", errors.Annotate(
					err, "getting service name",
				)
			}
			availabilitySetName = serviceName
			break
		}
	}

	logger.Debugf("- creating availability set %q", availabilitySetName)
	var availabilitySet compute.AvailabilitySet
	if err := callAPI(func() (autorest.Response, error) {
		var err error
		availabilitySet, err = client.CreateOrUpdate(
			resourceGroup, availabilitySetName, compute.AvailabilitySet{
				Location: to.StringPtr(location),
				// NOTE(axw) we do *not* want to use vmTags here,
				// because an availability set is shared by machines.
				Tags: to.StringMapPtr(envTags),
			},
		)
		return availabilitySet.Response, err
	}); err != nil {
		return "", errors.Annotatef(
			err, "creating availability set %q", availabilitySetName,
		)
	}
	return to.String(availabilitySet.ID), nil
}

// newStorageProfile creates the storage profile for a virtual machine,
// based on the series and chosen instance spec.
func newStorageProfile(
	vmName string,
	instanceSpec *instances.InstanceSpec,
	storageAccount *storage.Account,
) (*compute.StorageProfile, error) {
	logger.Debugf("creating storage profile for %q", vmName)

	urnParts := strings.SplitN(instanceSpec.Image.Id, ":", 4)
	if len(urnParts) != 4 {
		return nil, errors.Errorf("invalid image ID %q", instanceSpec.Image.Id)
	}
	publisher := urnParts[0]
	offer := urnParts[1]
	sku := urnParts[2]
	version := urnParts[3]

	osDisksRoot := osDiskVhdRoot(storageAccount)
	osDiskName := vmName
	osDiskSizeGB := mibToGB(instanceSpec.InstanceType.RootDisk)
	osDisk := &compute.OSDisk{
		Name:         to.StringPtr(osDiskName),
		CreateOption: compute.FromImage,
		Caching:      compute.ReadWrite,
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(
				osDisksRoot + osDiskName + vhdExtension,
			),
		},
		DiskSizeGB: to.Int32Ptr(int32(osDiskSizeGB)),
	}
	return &compute.StorageProfile{
		ImageReference: &compute.ImageReference{
			Publisher: to.StringPtr(publisher),
			Offer:     to.StringPtr(offer),
			Sku:       to.StringPtr(sku),
			Version:   to.StringPtr(version),
		},
		OsDisk: osDisk,
	}, nil
}

func mibToGB(mib uint64) uint64 {
	b := float64(mib * 1024 * 1024)
	return uint64(b / (1000 * 1000 * 1000))
}

func newOSProfile(vmName string, instanceConfig *instancecfg.InstanceConfig) (*compute.OSProfile, os.OSType, error) {
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
	case os.Ubuntu, os.CentOS, os.Arch:
		// SSH keys are handled by custom data, but must also be
		// specified in order to forego providing a password, and
		// disable password authentication.
		publicKeys := []compute.SSHPublicKey{{
			Path:    to.StringPtr("/home/ubuntu/.ssh/authorized_keys"),
			KeyData: to.StringPtr(instanceConfig.AuthorizedKeys),
		}}
		osProfile.AdminUsername = to.StringPtr("ubuntu")
		osProfile.LinuxConfiguration = &compute.LinuxConfiguration{
			DisablePasswordAuthentication: to.BoolPtr(true),
			SSH: &compute.SSHConfiguration{PublicKeys: &publicKeys},
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
func (env *azureEnviron) StopInstances(ids ...instance.Id) error {
	computeClient := env.compute
	networkClient := env.network

	// Query the instances, so we can inspect the VirtualMachines
	// and delete related resources.
	instances, err := env.Instances(ids)
	switch err {
	case environs.ErrNoInstances:
		return nil
	default:
		return errors.Trace(err)
	case nil, environs.ErrPartialInstances:
		// handled below
		break
	}

	storageClient, err := env.getStorageClient()
	if err != nil {
		return errors.Trace(err)
	}

	for _, inst := range instances {
		if inst == nil {
			continue
		}
		if err := deleteInstance(
			inst.(*azureInstance),
			env.callAPI, computeClient, networkClient, storageClient,
		); err != nil {
			return errors.Annotatef(err, "deleting instance %q", inst.Id())
		}
	}
	return nil
}

// deleteInstances deletes a virtual machine and all of the resources that
// it owns, and any corresponding network security rules.
func deleteInstance(
	inst *azureInstance,
	callAPI callAPIFunc,
	computeClient compute.ManagementClient,
	networkClient network.ManagementClient,
	storageClient internalazurestorage.Client,
) error {
	vmName := string(inst.Id())
	vmClient := compute.VirtualMachinesClient{computeClient}
	nicClient := network.InterfacesClient{networkClient}
	nsgClient := network.SecurityGroupsClient{networkClient}
	securityRuleClient := network.SecurityRulesClient{networkClient}
	publicIPClient := network.PublicIPAddressesClient{networkClient}
	logger.Debugf("deleting instance %q", vmName)

	logger.Debugf("- deleting virtual machine")
	var deleteResult autorest.Response
	if err := callAPI(func() (autorest.Response, error) {
		var err error
		deleteResult, err = vmClient.Delete(inst.env.resourceGroup, vmName, nil)
		return deleteResult, err
	}); err != nil {
		if deleteResult.Response == nil || deleteResult.StatusCode != http.StatusNotFound {
			return errors.Annotate(err, "deleting virtual machine")
		}
	}

	// Delete the VM's OS disk VHD.
	logger.Debugf("- deleting OS VHD")
	blobClient := storageClient.GetBlobService()
	if _, err := blobClient.DeleteBlobIfExists(osDiskVHDContainer, vmName, nil); err != nil {
		return errors.Annotate(err, "deleting OS VHD")
	}

	// Delete network security rules that refer to the VM.
	logger.Debugf("- deleting security rules")
	if err := deleteInstanceNetworkSecurityRules(
		inst.env.resourceGroup, inst.Id(), nsgClient,
		securityRuleClient, inst.env.callAPI,
	); err != nil {
		return errors.Annotate(err, "deleting network security rules")
	}

	// Detach public IPs from NICs. This must be done before public
	// IPs can be deleted. In the future, VMs may not necessarily
	// have a public IP, so we don't use the presence of a public
	// IP to indicate the existence of an instance.
	logger.Debugf("- detaching public IP addresses")
	for _, nic := range inst.networkInterfaces {
		if nic.Properties.IPConfigurations == nil {
			continue
		}
		var detached bool
		for i, ipConfiguration := range *nic.Properties.IPConfigurations {
			if ipConfiguration.Properties.PublicIPAddress == nil {
				continue
			}
			ipConfiguration.Properties.PublicIPAddress = nil
			(*nic.Properties.IPConfigurations)[i] = ipConfiguration
			detached = true
		}
		if detached {
			if err := callAPI(func() (autorest.Response, error) {
				return nicClient.CreateOrUpdate(
					inst.env.resourceGroup,
					to.String(nic.Name), nic,
					nil, // abort channel
				)
			}); err != nil {
				return errors.Annotate(err, "detaching public IP addresses")
			}
		}
	}

	// Delete public IPs.
	logger.Debugf("- deleting public IPs")
	for _, pip := range inst.publicIPAddresses {
		pipName := to.String(pip.Name)
		logger.Tracef("deleting public IP %q", pipName)
		var result autorest.Response
		if err := callAPI(func() (autorest.Response, error) {
			var err error
			result, err = publicIPClient.Delete(inst.env.resourceGroup, pipName, nil)
			return result, err
		}); err != nil {
			if result.Response == nil || result.StatusCode != http.StatusNotFound {
				return errors.Annotate(err, "deleting public IP")
			}
		}
	}

	// Delete NICs.
	//
	// NOTE(axw) this *must* be deleted last, or we risk leaking resources.
	logger.Debugf("- deleting network interfaces")
	for _, nic := range inst.networkInterfaces {
		nicName := to.String(nic.Name)
		logger.Tracef("deleting NIC %q", nicName)
		var result autorest.Response
		if err := callAPI(func() (autorest.Response, error) {
			var err error
			result, err = nicClient.Delete(inst.env.resourceGroup, nicName, nil)
			return result, err
		}); err != nil {
			if result.Response == nil || result.StatusCode != http.StatusNotFound {
				return errors.Annotate(err, "deleting NIC")
			}
		}
	}

	return nil
}

// Instances is specified in the Environ interface.
func (env *azureEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return env.instances(env.resourceGroup, ids, true /* refresh addresses */)
}

func (env *azureEnviron) instances(
	resourceGroup string,
	ids []instance.Id,
	refreshAddresses bool,
) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	all, err := env.allInstances(resourceGroup, refreshAddresses)
	if err != nil {
		return nil, errors.Trace(err)
	}
	byId := make(map[instance.Id]instance.Instance)
	for _, inst := range all {
		byId[inst.Id()] = inst
	}
	var found int
	matching := make([]instance.Instance, len(ids))
	for i, id := range ids {
		inst, ok := byId[id]
		if !ok {
			continue
		}
		matching[i] = inst
		found++
	}
	if found == 0 {
		return nil, environs.ErrNoInstances
	} else if found < len(ids) {
		return matching, environs.ErrPartialInstances
	}
	return matching, nil
}

// AllInstances is specified in the InstanceBroker interface.
func (env *azureEnviron) AllInstances() ([]instance.Instance, error) {
	return env.allInstances(env.resourceGroup, true /* refresh addresses */)
}

// allInstances returns all of the instances in the given resource group,
// and optionally ensures that each instance's addresses are up-to-date.
func (env *azureEnviron) allInstances(
	resourceGroup string,
	refreshAddresses bool,
) ([]instance.Instance, error) {
	vmClient := compute.VirtualMachinesClient{env.compute}
	nicClient := network.InterfacesClient{env.network}
	pipClient := network.PublicIPAddressesClient{env.network}

	// Due to how deleting instances works, we have to get creative about
	// listing instances. We list NICs and return an instance for each
	// unique value of the jujuMachineNameTag tag.
	//
	// The machine provisioner will call AllInstances so it can delete
	// unknown instances. StopInstances must delete VMs before NICs and
	// public IPs, because a VM cannot have less than 1 NIC. Thus, we can
	// potentially delete a VM but then fail to delete its NIC.
	var nicsResult network.InterfaceListResult
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		nicsResult, err = nicClient.List(resourceGroup)
		return nicsResult.Response, err
	}); err != nil {
		if nicsResult.Response.Response != nil && nicsResult.StatusCode == http.StatusNotFound {
			// This will occur if the resource group does not
			// exist, e.g. in a fresh hosted environment.
			return nil, nil
		}
		return nil, errors.Trace(err)
	}
	if nicsResult.Value == nil || len(*nicsResult.Value) == 0 {
		return nil, nil
	}

	// Create an azureInstance for each VM.
	var result compute.VirtualMachineListResult
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		result, err = vmClient.List(resourceGroup)
		return result.Response, err
	}); err != nil {
		return nil, errors.Annotate(err, "listing virtual machines")
	}
	vmNames := make(set.Strings)
	var azureInstances []*azureInstance
	if result.Value != nil {
		azureInstances = make([]*azureInstance, len(*result.Value))
		for i, vm := range *result.Value {
			inst := &azureInstance{vm, env, nil, nil}
			azureInstances[i] = inst
			vmNames.Add(to.String(vm.Name))
		}
	}

	// Create additional azureInstances for NICs without machines. See
	// comments above for rationale. This needs to happen before calling
	// setInstanceAddresses, so we still associate the NICs/PIPs.
	for _, nic := range *nicsResult.Value {
		vmName, ok := toTags(nic.Tags)[jujuMachineNameTag]
		if !ok || vmNames.Contains(vmName) {
			continue
		}
		vm := compute.VirtualMachine{
			Name: to.StringPtr(vmName),
			Properties: &compute.VirtualMachineProperties{
				ProvisioningState: to.StringPtr("Partially Deleted"),
			},
		}
		inst := &azureInstance{vm, env, nil, nil}
		azureInstances = append(azureInstances, inst)
		vmNames.Add(to.String(vm.Name))
	}

	if len(azureInstances) > 0 && refreshAddresses {
		if err := setInstanceAddresses(
			pipClient, resourceGroup, azureInstances, nicsResult,
		); err != nil {
			return nil, errors.Trace(err)
		}
	}
	instances := make([]instance.Instance, len(azureInstances))
	for i, inst := range azureInstances {
		instances[i] = inst
	}
	return instances, nil
}

// Destroy is specified in the Environ interface.
func (env *azureEnviron) Destroy() error {
	logger.Debugf("destroying model %q", env.envName)
	logger.Debugf("- deleting resource group %q", env.resourceGroup)
	if err := env.deleteResourceGroup(env.resourceGroup); err != nil {
		return errors.Trace(err)
	}
	// Resource groups are self-contained and fully encompass
	// all environ resources. Once you delete the group, there
	// is nothing else to do.
	return nil
}

// DestroyController is specified in the Environ interface.
func (env *azureEnviron) DestroyController(controllerUUID string) error {
	logger.Debugf("destroying model %q", env.envName)
	logger.Debugf("- deleting resource groups")
	if err := env.deleteControllerManagedResourceGroups(controllerUUID); err != nil {
		return errors.Trace(err)
	}
	// Resource groups are self-contained and fully encompass
	// all environ resources. Once you delete the group, there
	// is nothing else to do.
	return nil
}

func (env *azureEnviron) deleteControllerManagedResourceGroups(controllerUUID string) error {
	filter := fmt.Sprintf(
		"tagname eq '%s' and tagvalue eq '%s'",
		tags.JujuController, controllerUUID,
	)
	client := resources.GroupsClient{env.resources}
	var result resources.ResourceGroupListResult
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		result, err = client.List(filter, nil)
		return result.Response, err
	}); err != nil {
		return errors.Annotate(err, "listing resource groups")
	}
	if result.Value == nil {
		return nil
	}

	// Deleting groups can take a long time, so make sure they are
	// deleted in parallel.
	var wg sync.WaitGroup
	errs := make([]error, len(*result.Value))
	for i, group := range *result.Value {
		groupName := to.String(group.Name)
		logger.Debugf("  - deleting resource group %q", groupName)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := env.deleteResourceGroup(groupName); err != nil {
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

func (env *azureEnviron) deleteResourceGroup(resourceGroup string) error {
	client := resources.GroupsClient{env.resources}
	var result autorest.Response
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		result, err = client.Delete(resourceGroup, nil)
		return result, err
	}); err != nil {
		if result.Response == nil || result.StatusCode != http.StatusNotFound {
			return errors.Annotatef(err, "deleting resource group %q", resourceGroup)
		}
	}
	return nil
}

var errNoFwGlobal = errors.New("global firewall mode is not supported")

// OpenPorts is specified in the Environ interface. However, Azure does not
// support the global firewall mode.
func (env *azureEnviron) OpenPorts(ports []jujunetwork.PortRange) error {
	return errNoFwGlobal
}

// ClosePorts is specified in the Environ interface. However, Azure does not
// support the global firewall mode.
func (env *azureEnviron) ClosePorts(ports []jujunetwork.PortRange) error {
	return errNoFwGlobal
}

// Ports is specified in the Environ interface.
func (env *azureEnviron) Ports() ([]jujunetwork.PortRange, error) {
	return nil, errNoFwGlobal
}

// Provider is specified in the Environ interface.
func (env *azureEnviron) Provider() environs.EnvironProvider {
	return env.provider
}

// resourceGroupName returns the name of the environment's resource group.
func resourceGroupName(modelTag names.ModelTag, modelName string) string {
	return fmt.Sprintf("juju-%s-%s", modelName, resourceName(modelTag))
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
func (env *azureEnviron) getInstanceTypes() (map[string]instances.InstanceType, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	instanceTypes, err := env.getInstanceTypesLocked()
	if err != nil {
		return nil, errors.Annotate(err, "getting instance types")
	}
	return instanceTypes, nil
}

// getInstanceTypesLocked returns the instance types for Azure, by listing the
// role sizes available to the subscription.
func (env *azureEnviron) getInstanceTypesLocked() (map[string]instances.InstanceType, error) {
	if env.instanceTypes != nil {
		return env.instanceTypes, nil
	}

	location := env.location
	client := compute.VirtualMachineSizesClient{env.compute}

	var result compute.VirtualMachineSizeListResult
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		result, err = client.List(location)
		return result.Response, err
	}); err != nil {
		return nil, errors.Annotate(err, "listing VM sizes")
	}
	instanceTypes := make(map[string]instances.InstanceType)
	if result.Value != nil {
		for _, size := range *result.Value {
			instanceType := newInstanceType(size)
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

// getStorageClient queries the storage account key, and uses it to construct
// a new storage client.
func (env *azureEnviron) getStorageClient() (internalazurestorage.Client, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	storageAccount, err := env.getStorageAccountLocked(false)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage account")
	}
	storageAccountKey, err := env.getStorageAccountKeyLocked(
		to.String(storageAccount.Name), false,
	)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage account key")
	}
	client, err := getStorageClient(
		env.provider.config.NewStorageClient,
		env.storageEndpoint,
		storageAccount,
		storageAccountKey,
	)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage client")
	}
	return client, nil
}

// getStorageAccount returns the storage account for this environment's
// resource group. If refresh is true, cached details will be refreshed.
func (env *azureEnviron) getStorageAccount(refresh bool) (*storage.Account, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	return env.getStorageAccountLocked(refresh)
}

func (env *azureEnviron) getStorageAccountLocked(refresh bool) (*storage.Account, error) {
	if !refresh && env.storageAccount != nil {
		return env.storageAccount, nil
	}
	client := storage.AccountsClient{env.storage}
	var result storage.AccountListResult
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		result, err = client.List()
		return result.Response, err
	}); err != nil {
		return nil, errors.Annotate(err, "listing storage accounts")
	}
	if result.Value == nil || len(*result.Value) == 0 {
		return nil, errors.NotFoundf("storage account")
	}
	for _, account := range *result.Value {
		if toTags(account.Tags)[tags.JujuModel] != env.config.UUID() {
			continue
		}
		env.storageAccount = &account
		return &account, nil
	}
	return nil, errors.NotFoundf("storage account")
}

// getStorageAccountKey returns a storage account key for this
// environment's storage account. If refresh is true, any cached
// key will be refreshed.
func (env *azureEnviron) getStorageAccountKeys(accountName string, refresh bool) (*storage.AccountKey, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	return env.getStorageAccountKeyLocked(accountName, refresh)
}

func (env *azureEnviron) getStorageAccountKeyLocked(accountName string, refresh bool) (*storage.AccountKey, error) {
	if !refresh && env.storageAccountKey != nil {
		return env.storageAccountKey, nil
	}
	client := storage.AccountsClient{env.storage}
	key, err := getStorageAccountKey(
		env.callAPI,
		client,
		env.resourceGroup,
		accountName,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	env.storageAccountKey = key
	return key, nil
}

// AgentMirror is specified in the tools.HasAgentMirror interface.
//
// TODO(axw) 2016-04-11 #1568715
// When we have image simplestreams, we should rename this to "Region",
// to implement simplestreams.HasRegion.
func (env *azureEnviron) AgentMirror() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region: env.location,
		// The endpoints published in simplestreams
		// data are the storage endpoints.
		Endpoint: fmt.Sprintf("https://%s/", env.storageEndpoint),
	}, nil
}

func (env *azureEnviron) callAPI(f func() (autorest.Response, error)) error {
	return backoffAPIRequestCaller{env.provider.config.RetryClock}.call(f)
}
