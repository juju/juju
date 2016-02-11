// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest"
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	azurestorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"
	"github.com/juju/utils/set"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	jujunetwork "github.com/juju/juju/network"
	internalazurestorage "github.com/juju/juju/provider/azure/internal/azurestorage"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
)

const jujuMachineNameTag = tags.JujuTagPrefix + "machine-name"

type azureEnviron struct {
	common.SupportsUnitPlacementPolicy

	// provider is the azureEnvironProvider used to open this environment.
	provider *azureEnvironProvider

	// resourceGroup is the name of the Resource Group in the Azure
	// subscription that corresponds to the environment.
	resourceGroup string

	// controllerResourceGroup is the name of the Resource Group in the
	// Azure subscription that corresponds to the Juju controller
	// environment.
	controllerResourceGroup string

	// envName is the name of the environment.
	envName string

	mu            sync.Mutex
	config        *azureModelConfig
	instanceTypes map[string]instances.InstanceType
	// azure management clients
	compute       compute.ManagementClient
	resources     resources.ManagementClient
	storage       storage.ManagementClient
	network       network.ManagementClient
	storageClient azurestorage.Client
}

var _ environs.Environ = (*azureEnviron)(nil)
var _ state.Prechecker = (*azureEnviron)(nil)

// newEnviron creates a new azureEnviron.
func newEnviron(provider *azureEnvironProvider, cfg *config.Config) (*azureEnviron, error) {
	env := azureEnviron{provider: provider}
	err := env.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	env.resourceGroup = resourceGroupName(cfg)
	env.controllerResourceGroup = env.config.controllerResourceGroup
	env.envName = cfg.Name()
	return &env, nil
}

// Bootstrap is specified in the Environ interface.
func (env *azureEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {

	cfg, err := env.initResourceGroup()
	if err != nil {
		return nil, errors.Annotate(err, "creating controller resource group")
	}
	if err := env.SetConfig(cfg); err != nil {
		return nil, errors.Annotate(err, "updating config")
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
// environment. The resource group will have a storage account and a
// subnet associated with it (but not necessarily contained within:
// see subnet creation).
func (env *azureEnviron) initResourceGroup() (*config.Config, error) {
	location := env.config.location
	tags, _ := env.config.ResourceTags()
	resourceGroupsClient := resources.GroupsClient{env.resources}

	logger.Debugf("creating resource group %q", env.resourceGroup)
	_, err := resourceGroupsClient.CreateOrUpdate(env.resourceGroup, resources.Group{
		Location: to.StringPtr(location),
		Tags:     toTagsPtr(tags),
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating resource group")
	}

	var vnetPtr *network.VirtualNetwork
	if env.resourceGroup == env.controllerResourceGroup {
		// Create an internal network for all VMs to connect to.
		vnetPtr, err = createInternalVirtualNetwork(
			env.network, env.controllerResourceGroup, location, tags,
		)
		if err != nil {
			return nil, errors.Annotate(err, "creating virtual network")
		}
	} else {
		// We're creating a hosted environment, so we need to fetch
		// the virtual network to create a subnet below.
		vnetClient := network.VirtualNetworksClient{env.network}
		vnet, err := vnetClient.Get(env.controllerResourceGroup, internalNetworkName)
		if err != nil {
			return nil, errors.Annotate(err, "getting virtual network")
		}
		vnetPtr = &vnet
	}

	_, err = createInternalSubnet(
		env.network, env.resourceGroup, env.controllerResourceGroup,
		vnetPtr, location, tags,
	)
	if err != nil {
		return nil, errors.Annotate(err, "creating subnet")
	}

	// Create a storage account for the resource group.
	storageAccountsClient := storage.AccountsClient{env.storage}
	storageAccountName, storageAccountKey, err := createStorageAccount(
		storageAccountsClient, env.config.storageAccountType,
		env.resourceGroup, location, tags,
		env.provider.config.StorageAccountNameGenerator,
	)
	if err != nil {
		return nil, errors.Annotate(err, "creating storage account")
	}
	return env.config.Config.Apply(map[string]interface{}{
		configAttrStorageAccount:    storageAccountName,
		configAttrStorageAccountKey: storageAccountKey,
	})
}

func createStorageAccount(
	client storage.AccountsClient,
	accountType storage.AccountType,
	resourceGroup string,
	location string,
	tags map[string]string,
	accountNameGenerator func() string,
) (string, string, error) {
	logger.Debugf("creating storage account (finding available name)")
	const maxAttempts = 10
	for remaining := maxAttempts; remaining > 0; remaining-- {
		accountName := accountNameGenerator()
		logger.Debugf("- checking storage account name %q", accountName)
		result, err := client.CheckNameAvailability(
			storage.AccountCheckNameAvailabilityParameters{
				Name: to.StringPtr(accountName),
				// Azure is a little inconsistent with when Type is
				// required. It's required here.
				Type: to.StringPtr("Microsoft.Storage/storageAccounts"),
			},
		)
		if err != nil {
			return "", "", errors.Annotate(err, "checking account name availability")
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
			Tags:     toTagsPtr(tags),
			Properties: &storage.AccountPropertiesCreateParameters{
				AccountType: accountType,
			},
		}
		logger.Debugf("- creating %q storage account %q", accountType, accountName)
		// TODO(axw) account creation can fail if the account name is
		// available, but contains profanity. We should retry a set
		// number of times even if creating fails.
		if _, err := client.Create(resourceGroup, accountName, createParams); err != nil {
			return "", "", errors.Trace(err)
		}
		logger.Debugf("- listing storage account keys")
		listKeysResult, err := client.ListKeys(resourceGroup, accountName)
		if err != nil {
			return "", "", errors.Annotate(err, "listing storage account keys")
		}
		return accountName, to.String(listKeysResult.Key1), nil
	}
	return "", "", errors.New("could not find available storage account name")
}

// ControllerInstances is specified in the Environ interface.
func (env *azureEnviron) ControllerInstances() ([]instance.Id, error) {
	// controllers are tagged with tags.JujuController, so just
	// list the instances in the controller resource group and pick
	// those ones out.
	instances, err := env.allInstances(env.controllerResourceGroup, true)
	if err != nil {
		return nil, err
	}
	var ids []instance.Id
	for _, inst := range instances {
		azureInstance := inst.(*azureInstance)
		if toTags(azureInstance.Tags)[tags.JujuController] == "true" {
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

	// Initialise clients.
	env.compute = compute.NewWithBaseURI(ecfg.endpoint, env.config.subscriptionId)
	env.resources = resources.NewWithBaseURI(ecfg.endpoint, env.config.subscriptionId)
	env.storage = storage.NewWithBaseURI(ecfg.endpoint, env.config.subscriptionId)
	env.network = network.NewWithBaseURI(ecfg.endpoint, env.config.subscriptionId)
	clients := map[string]*autorest.Client{
		"azure.compute":   &env.compute.Client,
		"azure.resources": &env.resources.Client,
		"azure.storage":   &env.storage.Client,
		"azure.network":   &env.network.Client,
	}
	if env.provider.config.Sender != nil {
		env.config.token.SetSender(env.provider.config.Sender)
	}
	for id, client := range clients {
		client.Authorizer = env.config.token
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

	// Invalidate instance types when the location changes.
	if old != nil {
		oldLocation := old.UnknownAttrs()["location"].(string)
		if env.config.location != oldLocation {
			env.instanceTypes = nil
		}
	}

	return nil
}

// SupportedArchitectures is specified on the EnvironCapability interface.
func (env *azureEnviron) SupportedArchitectures() ([]string, error) {
	return env.supportedArchitectures(), nil
}

func (env *azureEnviron) supportedArchitectures() []string {
	return []string{arch.AMD64}
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
	})
	validator.RegisterVocabulary(
		constraints.Arch,
		env.supportedArchitectures(),
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
			constraints.RootDisk,
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
	if args.InstanceConfig.HasNetworks() {
		return nil, errors.New("starting instances with networks is not supported yet")
	}

	err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.Config())
	if err != nil {
		return nil, err
	}

	// Pick envtools.  Needed for the custom data (which is what we normally
	// call userdata).
	args.InstanceConfig.Tools = args.Tools[0]
	logger.Infof("picked tools %q", args.InstanceConfig.Tools)

	// Get the required configuration and config-dependent information
	// required to create the instance. We take the lock just once, to
	// ensure we obtain all information based on the same configuration.
	env.mu.Lock()
	location := env.config.location
	envTags, _ := env.config.ResourceTags()
	apiPort := env.config.APIPort()
	vmClient := compute.VirtualMachinesClient{env.compute}
	availabilitySetClient := compute.AvailabilitySetsClient{env.compute}
	networkClient := env.network
	vmImagesClient := compute.VirtualMachineImagesClient{env.compute}
	vmExtensionClient := compute.VirtualMachineExtensionsClient{env.compute}
	subscriptionId := env.config.subscriptionId
	imageStream := env.config.ImageStream()
	storageEndpoint := env.config.storageEndpoint
	storageAccountName := env.config.storageAccount
	instanceTypes, err := env.getInstanceTypesLocked()
	if err != nil {
		env.mu.Unlock()
		return nil, errors.Trace(err)
	}
	internalNetworkSubnet, err := env.getInternalSubnetLocked()
	if err != nil {
		env.mu.Unlock()
		return nil, errors.Trace(err)
	}
	env.mu.Unlock()

	// Identify the instance type and image to provision.
	instanceSpec, err := findInstanceSpec(
		vmImagesClient,
		instanceTypes,
		&instances.InstanceConstraint{
			Region:      location,
			Series:      args.Tools.OneSeries(),
			Arches:      args.Tools.Arches(),
			Constraints: args.Constraints,
		},
		imageStream,
	)
	if err != nil {
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

	// If the machine will run a controller, then we need to open the
	// API port for it.
	var apiPortPtr *int
	if multiwatcher.AnyJobNeedsState(args.InstanceConfig.Jobs...) {
		apiPortPtr = &apiPort
	}

	// Construct the network security group ID for the environment.
	nsgID := path.Join(
		"/subscriptions", subscriptionId, "resourceGroups",
		env.resourceGroup, "providers", "Microsoft.Network",
		"networkSecurityGroups", internalSecurityGroupName,
	)

	vm, err := createVirtualMachine(
		env.resourceGroup, location, vmName,
		vmTags, envTags,
		instanceSpec, args.InstanceConfig,
		args.DistributionGroup,
		env.Instances,
		apiPortPtr, internalNetworkSubnet, nsgID,
		storageEndpoint, storageAccountName,
		networkClient, vmClient,
		availabilitySetClient, vmExtensionClient,
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
	resourceGroup, location, vmName string,
	vmTags, envTags map[string]string,
	instanceSpec *instances.InstanceSpec,
	instanceConfig *instancecfg.InstanceConfig,
	distributionGroupFunc func() ([]instance.Id, error),
	instancesFunc func([]instance.Id) ([]instance.Instance, error),
	apiPort *int,
	internalNetworkSubnet *network.Subnet,
	nsgID, storageEndpoint, storageAccountName string,
	networkClient network.ManagementClient,
	vmClient compute.VirtualMachinesClient,
	availabilitySetClient compute.AvailabilitySetsClient,
	vmExtensionClient compute.VirtualMachineExtensionsClient,
) (compute.VirtualMachine, error) {

	storageProfile, err := newStorageProfile(
		vmName, instanceConfig.Series,
		instanceSpec, storageEndpoint, storageAccountName,
	)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating storage profile")
	}

	osProfile, seriesOS, err := newOSProfile(vmName, instanceConfig)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating OS profile")
	}

	networkProfile, err := newNetworkProfile(
		networkClient, vmName, apiPort,
		internalNetworkSubnet, nsgID,
		resourceGroup, location, vmTags,
	)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating network profile")
	}

	availabilitySetId, err := createAvailabilitySet(
		availabilitySetClient,
		vmName, resourceGroup, location,
		vmTags, envTags,
		distributionGroupFunc, instancesFunc,
	)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating availability set")
	}

	vmArgs := compute.VirtualMachine{
		Location: to.StringPtr(location),
		Tags:     toTagsPtr(vmTags),
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
	vm, err := vmClient.CreateOrUpdate(resourceGroup, vmName, vmArgs)
	if err != nil {
		return compute.VirtualMachine{}, errors.Annotate(err, "creating virtual machine")
	}

	// On Windows and CentOS, we must add the CustomScript VM
	// extension to run the CustomData script.
	switch seriesOS {
	case os.Windows, os.CentOS:
		if err := createVMExtension(
			vmExtensionClient, seriesOS,
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
			serviceName, err := names.UnitService(unitName)
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
	availabilitySet, err := client.CreateOrUpdate(
		resourceGroup, availabilitySetName, compute.AvailabilitySet{
			Location: to.StringPtr(location),
			// NOTE(axw) we do *not* want to use vmTags here,
			// because an availability set is shared by machines.
			Tags: toTagsPtr(envTags),
		},
	)
	if err != nil {
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
	series string,
	instanceSpec *instances.InstanceSpec,
	storageEndpoint, storageAccountName string,
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

	osDisksRoot := osDiskVhdRoot(storageEndpoint, storageAccountName)
	osDiskName := vmName
	osDisk := &compute.OSDisk{
		Name:         to.StringPtr(osDiskName),
		CreateOption: compute.FromImage,
		Caching:      compute.ReadWrite,
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(
				osDisksRoot + osDiskName + vhdExtension,
			),
		},
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
	env.mu.Lock()
	computeClient := env.compute
	networkClient := env.network
	env.mu.Unlock()
	storageClient, err := env.getStorageClient()
	if err != nil {
		return errors.Trace(err)
	}

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

	for _, inst := range instances {
		if inst == nil {
			continue
		}
		if err := deleteInstance(
			inst.(*azureInstance), computeClient, networkClient, storageClient,
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
	deleteResult, err := vmClient.Delete(inst.env.resourceGroup, vmName)
	if err != nil {
		if deleteResult.Response == nil || deleteResult.StatusCode != http.StatusNotFound {
			return errors.Annotate(err, "deleting virtual machine")
		}
	}

	// Delete the VM's OS disk VHD.
	logger.Debugf("- deleting OS VHD")
	blobClient := storageClient.GetBlobService()
	if _, err := blobClient.DeleteBlobIfExists(osDiskVHDContainer, vmName); err != nil {
		return errors.Annotate(err, "deleting OS VHD")
	}

	// Delete network security rules that refer to the VM.
	logger.Debugf("- deleting security rules")
	if err := deleteInstanceNetworkSecurityRules(
		inst.env.resourceGroup, inst.Id(), nsgClient, securityRuleClient,
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
			if _, err := nicClient.CreateOrUpdate(
				inst.env.resourceGroup, to.String(nic.Name), nic,
			); err != nil {
				return errors.Annotate(err, "detaching public IP addresses")
			}
		}
	}

	// Delete public IPs.
	logger.Debugf("- deleting public IPs")
	for _, pip := range inst.publicIPAddresses {
		pipName := to.String(pip.Name)
		logger.Tracef("deleting public IP %q", pipName)
		result, err := publicIPClient.Delete(inst.env.resourceGroup, pipName)
		if err != nil {
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
		result, err := nicClient.Delete(inst.env.resourceGroup, nicName)
		if err != nil {
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
	env.mu.Lock()
	vmClient := compute.VirtualMachinesClient{env.compute}
	nicClient := network.InterfacesClient{env.network}
	pipClient := network.PublicIPAddressesClient{env.network}
	env.mu.Unlock()

	// Due to how deleting instances works, we have to get creative about
	// listing instances. We list NICs and return an instance for each
	// unique value of the jujuMachineNameTag tag.
	//
	// The machine provisioner will call AllInstances so it can delete
	// unknown instances. StopInstances must delete VMs before NICs and
	// public IPs, because a VM cannot have less than 1 NIC. Thus, we can
	// potentially delete a VM but then fail to delete its NIC.
	nicsResult, err := nicClient.List(resourceGroup)
	if err != nil {
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
	result, err := vmClient.List(resourceGroup)
	if err != nil {
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
	logger.Debugf("- deleting resource group")
	if err := env.deleteResourceGroup(); err != nil {
		return errors.Trace(err)
	}
	if env.resourceGroup == env.controllerResourceGroup {
		// This is the controller resource group; once it has been
		// deleted, there's nothing left.
		return nil
	}
	logger.Debugf("- deleting internal subnet")
	if err := env.deleteInternalSubnet(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (env *azureEnviron) deleteResourceGroup() error {
	client := resources.GroupsClient{env.resources}
	result, err := client.Delete(env.resourceGroup)
	if err != nil {
		if result.Response == nil || result.StatusCode != http.StatusNotFound {
			return errors.Annotatef(err, "deleting resource group %q", env.resourceGroup)
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
func resourceGroupName(cfg *config.Config) string {
	uuid, _ := cfg.UUID()
	// UUID is always available for azure environments, since the (new)
	// provider was introduced after environment UUIDs.
	modelTag := names.NewModelTag(uuid)
	return fmt.Sprintf(
		"juju-%s-%s", cfg.Name(),
		resourceName(modelTag),
	)
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

	location := env.config.location
	client := compute.VirtualMachineSizesClient{env.compute}

	result, err := client.List(location)
	if err != nil {
		return nil, errors.Trace(err)
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

// getInternalSubnetLocked queries the internal subnet for the environment.
func (env *azureEnviron) getInternalSubnetLocked() (*network.Subnet, error) {
	client := network.SubnetsClient{env.network}
	vnetName := internalNetworkName
	subnetName := env.resourceGroup
	subnet, err := client.Get(env.controllerResourceGroup, vnetName, subnetName)
	if err != nil {
		return nil, errors.Annotate(err, "getting internal subnet")
	}
	return &subnet, nil
}

// getStorageClient queries the storage account key, and uses it to construct
// a new storage client.
func (env *azureEnviron) getStorageClient() (internalazurestorage.Client, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	client, err := getStorageClient(env.provider.config.NewStorageClient, env.config)
	if err != nil {
		return nil, errors.Annotate(err, "getting storage client")
	}
	return client, nil
}
