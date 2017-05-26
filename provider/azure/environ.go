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
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	azurestorage "github.com/Azure/azure-sdk-for-go/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/os"
	jujuseries "github.com/juju/utils/series"
	"github.com/juju/version"
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
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	internalazurestorage "github.com/juju/juju/provider/azure/internal/azurestorage"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	"github.com/juju/juju/provider/azure/internal/tracing"
	"github.com/juju/juju/provider/azure/internal/useragent"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
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

	// controllerAvailabilitySet is the name of the availability set
	// used for controller machines.
	controllerAvailabilitySet = "juju-controller"
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

	// authorizer is the authorizer we use for Azure.
	authorizer *cloudSpecAuth

	compute            compute.ManagementClient
	resources          resources.ManagementClient
	storage            storage.ManagementClient
	network            network.ManagementClient
	storageClient      azurestorage.Client
	storageAccountName string

	mu                     sync.Mutex
	config                 *azureModelConfig
	instanceTypes          map[string]instances.InstanceType
	storageAccount         *storage.Account
	storageAccountKey      *storage.AccountKey
	commonResourcesCreated bool
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

	return &env, nil
}

func (env *azureEnviron) initEnviron() error {
	credAttrs := env.cloud.Credential.Attributes()
	env.subscriptionId = credAttrs[credAttrSubscriptionId]
	env.authorizer = &cloudSpecAuth{
		cloud:  env.cloud,
		sender: env.provider.config.Sender,
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
		useragent.UpdateClient(client)
		client.Authorizer = env.authorizer
		logger := loggo.GetLogger(id)
		if env.provider.config.Sender != nil {
			client.Sender = env.provider.config.Sender
		}
		client.ResponseInspector = tracing.RespondDecorator(logger)
		client.RequestInspector = tracing.PrepareDecorator(logger)
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
	return errors.Trace(env.initResourceGroup(args.ControllerUUID, false))
}

// Bootstrap is part of the Environ interface.
func (env *azureEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	if err := env.initResourceGroup(args.ControllerConfig.ControllerUUID(), true); err != nil {
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

// initResourceGroup creates a resource group for this environment.
func (env *azureEnviron) initResourceGroup(controllerUUID string, controller bool) error {
	resourceGroupsClient := resources.GroupsClient{env.resources}

	env.mu.Lock()
	tags := tags.ResourceTags(
		names.NewModelTag(env.config.Config.UUID()),
		names.NewControllerTag(controllerUUID),
		env.config,
	)
	storageAccountType := env.config.storageAccountType
	env.mu.Unlock()

	logger.Debugf("creating resource group %q", env.resourceGroup)
	err := env.callAPI(func() (autorest.Response, error) {
		group, err := resourceGroupsClient.CreateOrUpdate(env.resourceGroup, resources.ResourceGroup{
			Location: to.StringPtr(env.location),
			Tags:     to.StringMapPtr(tags),
		})
		return group.Response, err
	})
	if err != nil {
		return errors.Annotate(err, "creating resource group")
	}

	if !controller {
		// When we create a resource group for a non-controller model,
		// we must create the common resources up-front. This is so
		// that parallel deployments do not affect dynamic changes,
		// e.g. those made by the firewaller. For the controller model,
		// we fold the creation of these resources into the bootstrap
		// machine's deployment.
		if err := env.createCommonResourceDeployment(
			tags, storageAccountType, nil,
		); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (env *azureEnviron) createCommonResourceDeployment(
	tags map[string]string,
	storageAccountType string,
	rules []network.SecurityRule,
) error {
	const apiPort = -1
	commonResources := networkTemplateResources(
		env.location, tags, apiPort, rules,
	)
	commonResources = append(commonResources, storageAccountTemplateResource(
		env.location, tags,
		env.storageAccountName,
		storageAccountType,
	))

	// We perform this deployment asynchronously, to avoid blocking
	// the "juju add-model" command; Create is called synchronously.
	// Eventually we should have Create called asynchronously, but
	// until then we do this, and ensure that the deployment has
	// completed before we schedule additional deployments.
	deploymentsClient := resources.DeploymentsClient{env.resources}
	deploymentsClient.ResponseInspector = asyncCreationRespondDecorator(
		deploymentsClient.ResponseInspector,
	)
	template := armtemplates.Template{Resources: commonResources}
	if err := createDeployment(
		env.callAPI,
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
func (env *azureEnviron) ControllerInstances(controllerUUID string) ([]instance.Id, error) {
	instances, err := env.allInstances(env.resourceGroup, false, true)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, environs.ErrNoInstances
	}
	ids := make([]instance.Id, len(instances))
	for i, inst := range instances {
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
			constraints.Cores,
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

	// Get the required configuration and config-dependent information
	// required to create the instance. We take the lock just once, to
	// ensure we obtain all information based on the same configuration.
	env.mu.Lock()
	envTags := tags.ResourceTags(
		names.NewModelTag(env.config.Config.UUID()),
		names.NewControllerTag(args.ControllerUUID),
		env.config,
	)
	storageAccountType := env.config.storageAccountType
	imageStream := env.config.ImageStream()
	instanceTypes, err := env.getInstanceTypesLocked()
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
	series := args.Tools.OneSeries()
	instanceSpec, err := findInstanceSpec(
		compute.VirtualMachineImagesClient{env.compute},
		instanceTypes,
		&instances.InstanceConstraint{
			Region:      env.location,
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

	if err := env.createVirtualMachine(
		vmName, vmTags, envTags,
		instanceSpec, args.InstanceConfig,
		storageAccountType,
	); err != nil {
		logger.Errorf("creating instance failed, destroying: %v", err)
		if err := env.StopInstances(instance.Id(vmName)); err != nil {
			logger.Errorf("could not destroy failed virtual machine: %v", err)
		}
		return nil, errors.Annotatef(err, "creating virtual machine %q", vmName)
	}

	// Note: the instance is initialised without addresses to keep the
	// API chatter down. We will refresh the instance if we need to know
	// the addresses.
	inst := &azureInstance{vmName, "Creating", env, nil, nil}
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
func (env *azureEnviron) createVirtualMachine(
	vmName string,
	vmTags, envTags map[string]string,
	instanceSpec *instances.InstanceSpec,
	instanceConfig *instancecfg.InstanceConfig,
	storageAccountType string,
) error {

	deploymentsClient := resources.DeploymentsClient{env.resources}

	var apiPort int
	if instanceConfig.Controller != nil {
		apiPortValue := instanceConfig.Controller.Config.APIPort()
		apiPort = apiPortValue
	} else {
		apiPorts := instanceConfig.APIInfo.Ports()
		if len(apiPorts) != 1 {
			return errors.Errorf("expected one API port, found %v", apiPorts)
		}
		apiPort = apiPorts[0]
	}

	var nicDependsOn, vmDependsOn []string
	var resources []armtemplates.Resource
	createCommonResources := instanceConfig.Bootstrap != nil
	if createCommonResources {
		// We're starting the bootstrap machine, so we will create the
		// common resources in the same deployment.
		commonResources := networkTemplateResources(env.location, envTags, apiPort, nil)
		commonResources = append(commonResources, storageAccountTemplateResource(
			env.location, envTags,
			env.storageAccountName, storageAccountType,
		))
		resources = append(resources, commonResources...)
		nicDependsOn = append(nicDependsOn, fmt.Sprintf(
			`[resourceId('Microsoft.Network/virtualNetworks', '%s')]`,
			internalNetworkName,
		))
		vmDependsOn = append(vmDependsOn, fmt.Sprintf(
			`[resourceId('Microsoft.Storage/storageAccounts', '%s')]`,
			env.storageAccountName,
		))
	} else {
		// Wait for the common resource deployment to complete.
		if err := env.waitCommonResourcesCreated(); err != nil {
			return errors.Annotate(
				err, "waiting for common resources to be created",
			)
		}
	}

	osProfile, seriesOS, err := newOSProfile(
		vmName, instanceConfig,
		env.provider.config.RandomWindowsAdminPassword,
		env.provider.config.GenerateSSHKey,
	)
	if err != nil {
		return errors.Annotate(err, "creating OS profile")
	}
	storageProfile, err := newStorageProfile(vmName, env.storageAccountName, instanceSpec)
	if err != nil {
		return errors.Annotate(err, "creating storage profile")
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
		resources = append(resources, armtemplates.Resource{
			APIVersion: compute.APIVersion,
			Type:       "Microsoft.Compute/availabilitySets",
			Name:       availabilitySetName,
			Location:   env.location,
			Tags:       envTags,
		})
		availabilitySetSubResource = &compute.SubResource{
			ID: to.StringPtr(availabilitySetId),
		}
		vmDependsOn = append(vmDependsOn, availabilitySetId)
	}

	publicIPAddressName := vmName + "-public-ip"
	publicIPAddressId := fmt.Sprintf(`[resourceId('Microsoft.Network/publicIPAddresses', '%s')]`, publicIPAddressName)
	resources = append(resources, armtemplates.Resource{
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/publicIPAddresses",
		Name:       publicIPAddressName,
		Location:   env.location,
		Tags:       vmTags,
		Properties: &network.PublicIPAddressPropertiesFormat{
			PublicIPAllocationMethod: network.Dynamic,
		},
	})

	// Controller and non-controller machines are assigned to separate
	// subnets. This enables us to create controller-specific NSG rules
	// just by targeting the controller subnet.
	subnetName := internalSubnetName
	subnetPrefix := internalSubnetPrefix
	if instanceConfig.Controller != nil {
		subnetName = controllerSubnetName
		subnetPrefix = controllerSubnetPrefix
	}
	subnetId := fmt.Sprintf(
		`[concat(resourceId('Microsoft.Network/virtualNetworks', '%s'), '/subnets/%s')]`,
		internalNetworkName, subnetName,
	)

	privateIP, err := machineSubnetIP(subnetPrefix, instanceConfig.MachineId)
	if err != nil {
		return errors.Annotatef(err, "computing private IP address")
	}
	nicName := vmName + "-primary"
	nicId := fmt.Sprintf(`[resourceId('Microsoft.Network/networkInterfaces', '%s')]`, nicName)
	nicDependsOn = append(nicDependsOn, publicIPAddressId)
	ipConfigurations := []network.InterfaceIPConfiguration{{
		Name: to.StringPtr("primary"),
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			Primary:                   to.BoolPtr(true),
			PrivateIPAddress:          to.StringPtr(privateIP.String()),
			PrivateIPAllocationMethod: network.Static,
			Subnet: &network.Subnet{ID: to.StringPtr(subnetId)},
			PublicIPAddress: &network.PublicIPAddress{
				ID: to.StringPtr(publicIPAddressId),
			},
		},
	}}
	resources = append(resources, armtemplates.Resource{
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/networkInterfaces",
		Name:       nicName,
		Location:   env.location,
		Tags:       vmTags,
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: &ipConfigurations,
		},
		DependsOn: nicDependsOn,
	})

	nics := []compute.NetworkInterfaceReference{{
		ID: to.StringPtr(nicId),
		Properties: &compute.NetworkInterfaceReferenceProperties{
			Primary: to.BoolPtr(true),
		},
	}}
	vmDependsOn = append(vmDependsOn, nicId)
	resources = append(resources, armtemplates.Resource{
		APIVersion: compute.APIVersion,
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
			StorageProfile: storageProfile,
			OsProfile:      osProfile,
			NetworkProfile: &compute.NetworkProfile{
				&nics,
			},
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
		resources = append(resources, armtemplates.Resource{
			APIVersion: compute.APIVersion,
			Type:       "Microsoft.Compute/virtualMachines/extensions",
			Name:       vmName + "/" + extensionName,
			Location:   env.location,
			Tags:       vmTags,
			Properties: properties,
			DependsOn:  []string{"Microsoft.Compute/virtualMachines/" + vmName},
		})
	}

	logger.Debugf("- creating virtual machine deployment")
	template := armtemplates.Template{Resources: resources}
	// NOTE(axw) VMs take a long time to go to "Succeeded", so we do not
	// block waiting for them to be fully provisioned. This means we won't
	// return an error from StartInstance if the VM fails provisioning;
	// we will instead report the error via the instance's status.
	deploymentsClient.ResponseInspector = asyncCreationRespondDecorator(
		deploymentsClient.ResponseInspector,
	)
	if err := createDeployment(
		env.callAPI,
		deploymentsClient,
		env.resourceGroup,
		vmName, // deployment name
		template,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// waitCommonResourcesCreated waits for the "common" deployment to complete.
func (env *azureEnviron) waitCommonResourcesCreated() error {
	env.mu.Lock()
	defer env.mu.Unlock()
	if env.commonResourcesCreated {
		return nil
	}
	if err := env.waitCommonResourcesCreatedLocked(); err != nil {
		return errors.Trace(err)
	}
	env.commonResourcesCreated = true
	return nil
}

type deploymentIncompleteError struct {
	error
}

func (env *azureEnviron) waitCommonResourcesCreatedLocked() error {
	deploymentsClient := resources.DeploymentsClient{env.resources}

	// Release the lock while we're waiting, to avoid blocking others.
	env.mu.Unlock()
	defer env.mu.Lock()

	// Wait for up to 5 minutes, with a 5 second polling interval,
	// for the "common" deployment to be in one of the terminal
	// states. The deployment typically takes only around 30 seconds,
	// but we allow for a longer duration to be defensive.
	waitDeployment := func() error {
		var result resources.DeploymentExtended
		if err := env.callAPI(func() (autorest.Response, error) {
			var err error
			result, err = deploymentsClient.Get(env.resourceGroup, "common")
			return result.Response, err
		}); err != nil {
			if result.StatusCode == http.StatusNotFound {
				// The controller model does not have a "common"
				// deployment, as its common resources are created
				// in the machine-0 deployment to keep bootstrap times
				// optimal. Treat lack of a common deployment as an
				// indication that the model is the controller model.
				return nil
			}
			return errors.Annotate(err, "querying common deployment")
		}
		if result.Properties == nil {
			return deploymentIncompleteError{errors.New("deployment incomplete")}
		}

		state := to.String(result.Properties.ProvisioningState)
		if state == "Succeeded" {
			// The deployment has succeeded, so the resources are
			// ready for use.
			return nil
		}
		err := errors.Errorf("common resource deployment status is %q", state)
		switch state {
		case "Canceled", "Failed", "Deleted":
		default:
			err = deploymentIncompleteError{err}
		}
		return err
	}
	return retry.Call(retry.CallArgs{
		Func: waitDeployment,
		IsFatalError: func(err error) bool {
			_, ok := err.(deploymentIncompleteError)
			return !ok
		},
		Attempts:    -1,
		Delay:       5 * time.Second,
		MaxDuration: 5 * time.Minute,
		Clock:       env.provider.config.RetryClock,
	})
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
	storageAccountName string,
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
	version := urnParts[3]

	osDisksRoot := fmt.Sprintf(
		`reference(resourceId('Microsoft.Storage/storageAccounts', '%s'), '%s').primaryEndpoints.blob`,
		storageAccountName, storage.APIVersion,
	)
	osDiskName := vmName
	osDiskURI := fmt.Sprintf(
		`[concat(%s, '%s/%s%s')]`,
		osDisksRoot, osDiskVHDContainer, osDiskName, vhdExtension,
	)
	osDiskSizeGB := mibToGB(instanceSpec.InstanceType.RootDisk)
	osDisk := &compute.OSDisk{
		Name:         to.StringPtr(osDiskName),
		CreateOption: compute.FromImage,
		Caching:      compute.ReadWrite,
		Vhd:          &compute.VirtualHardDisk{URI: to.StringPtr(osDiskURI)},
		DiskSizeGB:   to.Int32Ptr(int32(osDiskSizeGB)),
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
				env.cancelDeployment(string(id)),
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

	maybeStorageClient, err := env.getStorageClient()
	if errors.IsNotFound(err) {
		// It is possible, if unlikely, that the first deployment for a
		// hosted model will fail or be canceled before the model's
		// storage account is created. We must therefore cater for the
		// account being missing or incomplete here.
		maybeStorageClient = nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// List network interfaces and public IP addresses.
	instanceNics, err := instanceNetworkInterfaces(
		env.callAPI, env.resourceGroup,
		network.InterfacesClient{env.network},
	)
	if err != nil {
		return errors.Trace(err)
	}
	instancePips, err := instancePublicIPAddresses(
		env.callAPI, env.resourceGroup,
		network.PublicIPAddressesClient{env.network},
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
func (env *azureEnviron) cancelDeployment(name string) error {
	deploymentsClient := resources.DeploymentsClient{env.resources}
	logger.Debugf("- canceling deployment %q", name)
	var cancelResult autorest.Response
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		cancelResult, err = deploymentsClient.Cancel(env.resourceGroup, name)
		return cancelResult, err
	}); err != nil {
		if cancelResult.Response != nil {
			switch cancelResult.StatusCode {
			case http.StatusNotFound:
				return errors.NewNotFound(err, fmt.Sprintf("deployment %q not found", name))
			case http.StatusConflict:
				if err, ok := errorutils.ServiceError(err); ok {
					if err.Code == serviceErrorCodeDeploymentCannotBeCancelled {
						// Deployments can only canceled while they're running.
						return nil
					}
				}
			}
		}
		return errors.Annotatef(err, "canceling deployment %q", name)
	}
	return nil
}

// deleteVirtualMachine deletes a virtual machine and all of the resources that
// it owns, and any corresponding network security rules.
func (env *azureEnviron) deleteVirtualMachine(
	instId instance.Id,
	maybeStorageClient internalazurestorage.Client,
	networkInterfaces []network.Interface,
	publicIPAddresses []network.PublicIPAddress,
) error {
	vmClient := compute.VirtualMachinesClient{env.compute}
	nicClient := network.InterfacesClient{env.network}
	nsgClient := network.SecurityGroupsClient{env.network}
	securityRuleClient := network.SecurityRulesClient{env.network}
	pipClient := network.PublicIPAddressesClient{env.network}
	deploymentsClient := resources.DeploymentsClient{env.resources}
	vmName := string(instId)

	logger.Debugf("- deleting virtual machine (%s)", vmName)
	if err := deleteResource(env.callAPI, vmClient, env.resourceGroup, vmName); err != nil {
		if !errors.IsNotFound(err) {
			return errors.Annotate(err, "deleting virtual machine")
		}
	}

	if maybeStorageClient != nil {
		logger.Debugf("- deleting OS VHD (%s)", vmName)
		blobClient := maybeStorageClient.GetBlobService()
		if _, err := blobClient.DeleteBlobIfExists(osDiskVHDContainer, vmName, nil); err != nil {
			return errors.Annotate(err, "deleting OS VHD")
		}
	}

	logger.Debugf("- deleting security rules (%s)", vmName)
	if err := deleteInstanceNetworkSecurityRules(
		env.resourceGroup, instId, nsgClient,
		securityRuleClient, env.callAPI,
	); err != nil {
		return errors.Annotate(err, "deleting network security rules")
	}

	logger.Debugf("- deleting network interfaces (%s)", vmName)
	for _, nic := range networkInterfaces {
		nicName := to.String(nic.Name)
		logger.Tracef("deleting NIC %q", nicName)
		if err := deleteResource(env.callAPI, nicClient, env.resourceGroup, nicName); err != nil {
			if !errors.IsNotFound(err) {
				return errors.Annotate(err, "deleting NIC")
			}
		}
	}

	logger.Debugf("- deleting public IPs (%s)", vmName)
	for _, pip := range publicIPAddresses {
		pipName := to.String(pip.Name)
		logger.Tracef("deleting public IP %q", pipName)
		if err := deleteResource(env.callAPI, pipClient, env.resourceGroup, pipName); err != nil {
			if !errors.IsNotFound(err) {
				return errors.Annotate(err, "deleting public IP")
			}
		}
	}

	// The deployment must be deleted last, or we risk leaking resources.
	logger.Debugf("- deleting deployment (%s)", vmName)
	if err := deleteResource(env.callAPI, deploymentsClient, env.resourceGroup, vmName); err != nil {
		if !errors.IsNotFound(err) {
			return errors.Annotate(err, "deleting deployment")
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
	all, err := env.allInstances(resourceGroup, refreshAddresses, false)
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

// AdoptResources is part of the Environ interface.
func (env *azureEnviron) AdoptResources(controllerUUID string, fromVersion version.Number) error {
	groupClient := resources.GroupsClient{env.resources}

	err := env.updateGroupControllerTag(&groupClient, env.resourceGroup, controllerUUID)
	if err != nil {
		// If we can't update the group there's no point updating the
		// contained resources - the group will be killed if the
		// controller is destroyed, taking the other things with it.
		return errors.Trace(err)
	}

	resourceClient := resources.Client{env.resources}
	var failed []string

	apiVersions, err := collectAPIVersions(env.callAPI, env.resources)
	if err != nil {
		return errors.Trace(err)
	}

	var res resources.ResourceListResult
	err = env.callAPI(func() (autorest.Response, error) {
		var err error
		res, err = groupClient.ListResources(env.resourceGroup, "", nil)
		return res.Response, err
	})
	if err != nil {
		return errors.Annotate(err, "listing resources")
	}
	for res.Value != nil {
		for _, resource := range *res.Value {
			// We need to set the API version to a value that's
			// correct for the specific resource type. If we leave it
			// as the version for the Microsoft.Resources provider we
			// get NoRegisteredProviderFound errors.
			resourceClient.APIVersion = apiVersions[to.String(resource.Type)]
			err := env.updateResourceControllerTag(&resourceClient, resource, controllerUUID)
			if err != nil {
				name := to.String(resource.Name)
				logger.Errorf("error updating resource tags for %q: %v", name, err)
				failed = append(failed, name)
			}
		}
		err = env.callAPI(func() (autorest.Response, error) {
			var err error
			res, err = groupClient.ListResourcesNextResults(res)
			return res.Response, err
		})
		if err != nil {
			return errors.Annotate(err, "getting next page of resources")
		}
	}

	if len(failed) > 0 {
		return errors.Errorf("failed to update controller for some resources: %v", failed)
	}
	return nil
}

func (env *azureEnviron) updateGroupControllerTag(client *resources.GroupsClient, groupName, controllerUUID string) error {
	var group resources.ResourceGroup
	err := env.callAPI(func() (autorest.Response, error) {
		var err error
		group, err = client.Get(groupName)
		return group.Response, err
	})
	if err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("updating resource group %s juju controller uuid to %s",
		to.String(group.Name), controllerUUID)
	groupTags := toTags(group.Tags)
	groupTags[tags.JujuController] = controllerUUID
	group.Tags = to.StringMapPtr(groupTags)

	// The Azure API forbids specifying ProvisioningState on the update.
	if group.Properties != nil {
		(*group.Properties).ProvisioningState = nil
	}

	err = env.callAPI(func() (autorest.Response, error) {
		res, err := client.CreateOrUpdate(groupName, group)
		return res.Response, err
	})
	return errors.Annotatef(err, "updating controller for resource group %q", groupName)
}

func (env *azureEnviron) updateResourceControllerTag(client *resources.Client, stubResource resources.GenericResource, controllerUUID string) error {
	stubTags := toTags(stubResource.Tags)
	if stubTags[tags.JujuController] == controllerUUID {
		// No update needed.
		return nil
	}

	namespace, parentPath, subtype, err := splitResourceType(to.String(stubResource.Type))
	if err != nil {
		return errors.Annotatef(err, "splitting resource type")
	}

	// Need to get the resource individually to ensure that the
	// properties are populated.
	var resource resources.GenericResource
	err = env.callAPI(func() (autorest.Response, error) {
		var err error
		resource, err = client.Get(
			env.resourceGroup,
			namespace,
			parentPath,
			subtype,
			to.String(stubResource.Name),
		)
		return resource.Response, err
	})
	if err != nil {
		return errors.Annotatef(err, "getting full resource %q", to.String(stubResource.Name))
	}

	logger.Debugf("updating %s (%s) juju controller uuid to %s",
		to.String(resource.Name), to.String(resource.Type), controllerUUID)
	resourceTags := toTags(resource.Tags)
	resourceTags[tags.JujuController] = controllerUUID
	resource.Tags = to.StringMapPtr(resourceTags)

	err = env.callAPI(func() (autorest.Response, error) {
		res, err := client.CreateOrUpdate(
			env.resourceGroup,
			namespace,
			parentPath,
			subtype,
			to.String(resource.Name),
			resource,
		)
		return res.Response, err
	})
	return errors.Annotatef(err, "updating controller for %q", to.String(resource.Name))
}

// splitResourceType breaks the resource type into provider namespace,
// parent path and subtype so we can pass the components to the
// resource CreateOrUpdate method.
func splitResourceType(resourceType string) (string, string, string, error) {
	parts := strings.Split(resourceType, "/")
	if len(parts) < 2 {
		return "", "", "", errors.Errorf("expected at least 2 parts in resource type %q", resourceType)
	}
	namespace := parts[0]
	subtype := parts[len(parts)-1]
	parentPath := strings.Join(parts[1:len(parts)-1], "/")
	return namespace, parentPath, subtype, nil
}

// AllInstances is specified in the InstanceBroker interface.
func (env *azureEnviron) AllInstances() ([]instance.Instance, error) {
	return env.allInstances(env.resourceGroup, true /* refresh addresses */, false /* all instances */)
}

// allInstances returns all of the instances in the given resource group,
// and optionally ensures that each instance's addresses are up-to-date.
func (env *azureEnviron) allInstances(
	resourceGroup string,
	refreshAddresses bool,
	controllerOnly bool,
) ([]instance.Instance, error) {
	deploymentsClient := resources.DeploymentsClient{env.resources}
	var deploymentsResult resources.DeploymentListResult
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		deploymentsResult, err = deploymentsClient.List(resourceGroup, "", nil)
		return deploymentsResult.Response, err
	}); err != nil {
		if deploymentsResult.Response.Response != nil && deploymentsResult.StatusCode == http.StatusNotFound {
			// This will occur if the resource group does not
			// exist, e.g. in a fresh hosted environment.
			return nil, nil
		}
		return nil, errors.Trace(err)
	}
	if deploymentsResult.Value == nil || len(*deploymentsResult.Value) == 0 {
		return nil, nil
	}

	azureInstances := make([]*azureInstance, 0, len(*deploymentsResult.Value))
	for _, deployment := range *deploymentsResult.Value {
		name := to.String(deployment.Name)
		if deployment.Properties == nil || deployment.Properties.Dependencies == nil {
			continue
		}
		if controllerOnly && !isControllerDeployment(deployment) {
			continue
		}
		provisioningState := to.String(deployment.Properties.ProvisioningState)
		inst := &azureInstance{name, provisioningState, env, nil, nil}
		azureInstances = append(azureInstances, inst)
	}

	if len(azureInstances) > 0 && refreshAddresses {
		if err := setInstanceAddresses(
			env.callAPI,
			resourceGroup,
			network.InterfacesClient{env.network},
			network.PublicIPAddressesClient{env.network},
			azureInstances,
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
func (env *azureEnviron) OpenPorts(ports []jujunetwork.IngressRule) error {
	return errNoFwGlobal
}

// ClosePorts is specified in the Environ interface. However, Azure does not
// support the global firewall mode.
func (env *azureEnviron) ClosePorts(ports []jujunetwork.IngressRule) error {
	return errNoFwGlobal
}

// Ports is specified in the Environ interface.
func (env *azureEnviron) IngressRules() ([]jujunetwork.IngressRule, error) {
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
	var account storage.Account
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		account, err = client.GetProperties(env.resourceGroup, env.storageAccountName)
		return account.Response, err
	}); err != nil {
		if account.Response.Response != nil && account.Response.StatusCode == http.StatusNotFound {
			return nil, errors.NewNotFound(err, fmt.Sprintf("storage account not found"))
		}
		return nil, errors.Annotate(err, "getting storage account")
	}
	env.storageAccount = &account
	return env.storageAccount, nil
}

// getStorageAccountKeysLocked returns a storage account key for this
// environment's storage account. If refresh is true, any cached key
// will be refreshed. This method assumes that env.mu is held.
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
