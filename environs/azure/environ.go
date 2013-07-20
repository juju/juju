// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"launchpad.net/gwacl"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

const (
	// In our initial implementation, each instance gets its own hosted
	// service, deployment and role in Azure.  The role always gets this
	// hostname (instance==service).
	roleHostname = "default"

	// Initially, this is the only location where Azure supports Linux.
	// TODO: This is to become a configuration item.
	// We currently use "North Europe" because the temporary Saucy image is
	// only supported there.
	serviceLocation = "North Europe"

	// deploymentSlot says in which slot to deploy instances.  Azure
	// supports 'Production' or 'Staging'.
	// This provider always deploys to Production.  Think twice about
	// changing that: DNS names in the staging slot work differently from
	// those in the production slot.  In Staging, Azure assigns an
	// arbitrary hostname that we can then extract from the deployment's
	// URL.  In Production, the hostname in the deployment URL does not
	// actually seem to resolve; instead, the service name is used as the
	// DNS name, with ".cloudapp.net" appended.
	deploymentSlot = "Production"

	// Address space of the virtual network used by the nodes in this
	// environement, in CIDR notation. This is the network used for
	// machine-to-machine communication.
	networkDefinition = "10.0.0.0/8"
)

type azureEnviron struct {
	// Except where indicated otherwise, all fields in this object should
	// only be accessed using a lock or a snapshot.
	sync.Mutex

	// name is immutable; it does not need locking.
	name string

	// ecfg is the environment's Azure-specific configuration.
	ecfg *azureEnvironConfig

	// storage is this environ's own private storage.
	storage environs.Storage

	// publicStorage is the public storage that this environ uses.
	publicStorage environs.StorageReader
}

// azureEnviron implements Environ.
var _ environs.Environ = (*azureEnviron)(nil)

// A request may fail to due "eventual consistency" semantics, which
// should resolve fairly quickly.  A request may also fail due to a slow
// state transition (for instance an instance taking a while to release
// a security group after termination).  The former failure mode is
// dealt with by shortAttempt, the latter by longAttempt.
// TODO: These settings may still need Azure-specific tuning.
var shortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// NewEnviron creates a new azureEnviron.
func NewEnviron(cfg *config.Config) (*azureEnviron, error) {
	env := azureEnviron{name: cfg.Name()}
	err := env.SetConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Set up storage.
	env.storage = &azureStorage{
		storageContext: &environStorageContext{environ: &env},
	}

	// Set up public storage.
	publicContext := publicEnvironStorageContext{environ: &env}
	if publicContext.getContainer() == "" {
		// No public storage configured.  Use EmptyStorage.
		env.publicStorage = environs.EmptyStorage
	} else {
		// Set up real public storage.
		env.publicStorage = &azureStorage{storageContext: &publicContext}
	}

	return &env, nil
}

// Name is specified in the Environ interface.
func (env *azureEnviron) Name() string {
	return env.name
}

// getSnapshot produces an atomic shallow copy of the environment object.
// Whenever you need to access the environment object's fields without
// modifying them, get a snapshot and read its fields instead.  You will
// get a consistent view of the fields without any further locking.
// If you do need to modify the environment's fields, do not get a snapshot
// but lock the object throughout the critical section.
func (env *azureEnviron) getSnapshot() *azureEnviron {
	env.Lock()
	defer env.Unlock()

	// Copy the environment.  (Not the pointer, the environment itself.)
	// This is a shallow copy.
	snap := *env
	// Reset the snapshot's mutex, because we just copied it while we
	// were holding it.  The snapshot will have a "clean," unlocked mutex.
	snap.Mutex = sync.Mutex{}
	return &snap
}

// startBootstrapInstance starts the bootstrap instance for this environment.
func (env *azureEnviron) startBootstrapInstance(cons constraints.Value) (instance.Instance, error) {
	// The bootstrap instance gets machine id "0".  This is not related to
	// instance ids or anything in Azure.  Juju assigns the machine ID.
	const machineID = "0"

	// Create an empty bootstrap state file so we can get its URL.
	// It will be updated with the instance id and hardware characteristics
	// after the bootstrap instance is started.
	stateFileURL, err := environs.CreateStateFile(env.Storage())
	if err != nil {
		return nil, err
	}
	machineConfig := environs.NewBootstrapMachineConfig(machineID, stateFileURL)

	logger.Debugf("bootstrapping environment %q", env.Name())
	possibleTools, err := environs.FindBootstrapTools(env, cons)
	if err != nil {
		return nil, err
	}
	inst, err := env.internalStartInstance(cons, possibleTools, machineConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	return inst, nil
}

// getAffinityGroupName returns the name of the affinity group used by all
// the Services in this environment.
func (env *azureEnviron) getAffinityGroupName() string {
	return env.getEnvPrefix() + "-ag"
}

func (env *azureEnviron) createAffinityGroup() error {
	affinityGroupName := env.getAffinityGroupName()
	azure, err := env.getManagementAPI()
	if err != nil {
		return nil
	}
	defer env.releaseManagementAPI(azure)
	cag := gwacl.NewCreateAffinityGroup(affinityGroupName, affinityGroupName, affinityGroupName, serviceLocation)
	return azure.CreateAffinityGroup(&gwacl.CreateAffinityGroupRequest{
		CreateAffinityGroup: cag})
}

func (env *azureEnviron) deleteAffinityGroup() error {
	affinityGroupName := env.getAffinityGroupName()
	azure, err := env.getManagementAPI()
	if err != nil {
		return nil
	}
	defer env.releaseManagementAPI(azure)
	return azure.DeleteAffinityGroup(&gwacl.DeleteAffinityGroupRequest{
		Name: affinityGroupName})
}

// getVirtualNetworkName returns the name of the virtual network used by all
// the VMs in this environment.
func (env *azureEnviron) getVirtualNetworkName() string {
	return env.getEnvPrefix() + "-vnet"
}

func (env *azureEnviron) createVirtualNetwork() error {
	vnetName := env.getVirtualNetworkName()
	affinityGroupName := env.getAffinityGroupName()
	azure, err := env.getManagementAPI()
	if err != nil {
		return nil
	}
	defer env.releaseManagementAPI(azure)
	virtualNetwork := gwacl.VirtualNetworkSite{
		Name:          vnetName,
		AffinityGroup: affinityGroupName,
		AddressSpacePrefixes: []string{
			networkDefinition,
		},
	}
	return azure.AddVirtualNetworkSite(&virtualNetwork)
}

func (env *azureEnviron) deleteVirtualNetwork() error {
	azure, err := env.getManagementAPI()
	if err != nil {
		return nil
	}
	defer env.releaseManagementAPI(azure)
	vnetName := env.getVirtualNetworkName()
	return azure.RemoveVirtualNetworkSite(vnetName)
}

// Bootstrap is specified in the Environ interface.
// TODO(bug 1199847): This work can be shared between providers.
func (env *azureEnviron) Bootstrap(cons constraints.Value) (err error) {
	if err := environs.VerifyBootstrapInit(env, shortAttempt); err != nil {
		return err
	}

	// TODO(bug 1199847). The creation of the affinity group and the
	// virtual network is specific to the Azure provider.
	err = env.createAffinityGroup()
	if err != nil {
		return err
	}
	// If we fail after this point, clean up the affinity group.
	defer func() {
		if err != nil {
			env.deleteAffinityGroup()
		}
	}()
	err = env.createVirtualNetwork()
	if err != nil {
		return err
	}
	// If we fail after this point, clean up the virtual network.
	defer func() {
		if err != nil {
			env.deleteVirtualNetwork()
		}
	}()

	inst, err := env.startBootstrapInstance(cons)
	if err != nil {
		return err
	}
	// TODO(wallyworld) - save hardware characteristics
	err = environs.SaveState(
		env.Storage(),
		&environs.BootstrapState{StateInstances: []instance.Id{inst.Id()}})
	if err != nil {
		err2 := env.StopInstances([]instance.Instance{inst})
		if err2 != nil {
			// Failure upon failure.  Log it, but return the
			// original error.
			logger.Errorf("cannot release failed bootstrap instance: %v", err2)
		}
		return fmt.Errorf("cannot save state: %v", err)
	}

	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.
	return nil
}

// StateInfo is specified in the Environ interface.
func (env *azureEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return environs.StateInfo(env)
}

// Config is specified in the Environ interface.
func (env *azureEnviron) Config() *config.Config {
	snap := env.getSnapshot()
	return snap.ecfg.Config
}

// SetConfig is specified in the Environ interface.
func (env *azureEnviron) SetConfig(cfg *config.Config) error {
	ecfg, err := azureEnvironProvider{}.newConfig(cfg)
	if err != nil {
		return err
	}

	env.Lock()
	defer env.Unlock()

	if env.ecfg != nil {
		_, err = azureEnvironProvider{}.Validate(cfg, env.ecfg.Config)
		if err != nil {
			return err
		}
	}

	env.ecfg = ecfg
	return nil
}

// attemptCreateService tries to create a new hosted service on Azure, with a
// name it chooses (based on the given prefix), but recognizes that the name
// may not be available.  If the name is not available, it does not treat that
// as an error but just returns nil.
func attemptCreateService(azure *gwacl.ManagementAPI, prefix string, affinityGroupName string) (*gwacl.CreateHostedService, error) {
	name := gwacl.MakeRandomHostedServiceName(prefix)
	req := gwacl.NewCreateHostedServiceWithLocation(name, name, serviceLocation)
	req.AffinityGroup = affinityGroupName
	err := azure.AddHostedService(req)
	azErr, isAzureError := err.(*gwacl.AzureError)
	if isAzureError && azErr.HTTPStatus == http.StatusConflict {
		// Conflict.  As far as we can see, this only happens if the
		// name was already in use.  It's still dangerous to assume
		// that we know it can't be anything else, but there's nothing
		// else in the error that we can use for closer identifcation.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return req, nil
}

// newHostedService creates a hosted service.  It will make up a unique name,
// starting with the given prefix.
func newHostedService(azure *gwacl.ManagementAPI, prefix string, affinityGroupName string) (*gwacl.CreateHostedService, error) {
	var err error
	var svc *gwacl.CreateHostedService
	for tries := 10; tries > 0 && err == nil && svc == nil; tries-- {
		svc, err = attemptCreateService(azure, prefix, affinityGroupName)
	}
	if err != nil {
		return nil, fmt.Errorf("could not create hosted service: %v", err)
	}
	if svc == nil {
		return nil, fmt.Errorf("could not come up with a unique hosted service name - is your randomizer initialized?")
	}
	return svc, nil
}

// internalStartInstance does the provider-specific work of starting an
// instance.  The code in StartInstance is actually largely agnostic across
// the EC2/OpenStack/MAAS/Azure providers.
// The instance will be set up for the same series for which you pass tools.
// All tools in possibleTools must be for the same series.
// machineConfig will be filled out with further details, but should contain
// MachineID, MachineNonce, StateInfo, and APIInfo.
// TODO(bug 1199847): Some of this work can be shared between providers.
func (env *azureEnviron) internalStartInstance(cons constraints.Value, possibleTools tools.List, machineConfig *cloudinit.MachineConfig) (_ instance.Instance, err error) {
	// Declaring "err" in the function signature so that we can "defer"
	// any cleanup that needs to run during error returns.

	series := possibleTools.Series()
	if len(series) != 1 {
		panic(fmt.Errorf("should have gotten tools for one series, got %v", series))
	}

	err = environs.FinishMachineConfig(machineConfig, env.Config(), cons)
	if err != nil {
		return nil, err
	}

	// Pick tools.  Needed for the custom data (which is what we normally
	// call userdata).
	machineConfig.Tools = possibleTools[0]
	logger.Infof("picked tools %q", machineConfig.Tools)

	// Compose userdata.
	userData, err := makeCustomData(machineConfig)
	if err != nil {
		return nil, fmt.Errorf("custom data: %v", err)
	}

	azure, err := env.getManagementAPI()
	if err != nil {
		return nil, err
	}
	defer env.releaseManagementAPI(azure)

	service, err := newHostedService(azure.ManagementAPI, env.getEnvPrefix(), env.getAffinityGroupName())
	if err != nil {
		return nil, err
	}
	serviceName := service.ServiceName

	// If we fail after this point, clean up the hosted service.
	defer func() {
		if err != nil {
			azure.DestroyHostedService(
				&gwacl.DestroyHostedServiceRequest{
					ServiceName: serviceName,
				})
		}
	}()

	// TODO: use simplestreams to get the name of the image given
	// the constraints provided by Juju.
	// In the meantime we use a temporary Saucy image containing a
	// cloud-init package which supports Azure.
	sourceImageName := "b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-13_10-amd64-server-DEVELOPMENT-20130713-Juju_ALPHA-en-us-30GB"

	// virtualNetworkName is the virtual network to which all the
	// deployments in this environment belong.
	virtualNetworkName := env.getVirtualNetworkName()

	// 1. Create an OS Disk.
	vhd := env.newOSDisk(sourceImageName)

	// 2. Create a Role for a Linux machine.
	role := env.newRole(vhd, userData, roleHostname)

	// 3. Create the Deployment object.
	deployment := env.newDeployment(role, serviceName, serviceName, virtualNetworkName)

	err = azure.AddDeployment(deployment, serviceName)
	if err != nil {
		return nil, err
	}

	var inst instance.Instance

	// From here on, remember to shut down the instance before returning
	// any error.
	defer func() {
		if err != nil && inst != nil {
			err2 := env.StopInstances([]instance.Instance{inst})
			if err2 != nil {
				// Failure upon failure.  Log it, but return
				// the original error.
				logger.Errorf("error releasing failed instance: %v", err)
			}
		}
	}()

	// Assign the returned instance to 'inst' so that the deferred method
	// above can perform its check.
	inst, err = env.getInstance(serviceName)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// getInstance returns an up-to-date version of the instance with the given
// name.
func (env *azureEnviron) getInstance(instanceName string) (instance.Instance, error) {
	context, err := env.getManagementAPI()
	if err != nil {
		return nil, err
	}
	defer env.releaseManagementAPI(context)
	service, err := context.GetHostedServiceProperties(instanceName, false)
	if err != nil {
		return nil, fmt.Errorf("could not get instance %q: %v", instanceName, err)
	}
	instance := &azureInstance{service.HostedServiceDescriptor}
	return instance, nil
}

// newOSDisk creates a gwacl.OSVirtualHardDisk object suitable for an
// Azure Virtual Machine.
func (env *azureEnviron) newOSDisk(sourceImageName string) *gwacl.OSVirtualHardDisk {
	vhdName := gwacl.MakeRandomDiskName("juju")
	vhdPath := fmt.Sprintf("vhds/%s", vhdName)
	snap := env.getSnapshot()
	storageAccount := snap.ecfg.StorageAccountName()
	mediaLink := gwacl.CreateVirtualHardDiskMediaLink(storageAccount, vhdPath)
	// The disk label is optional and the disk name can be omitted if
	// mediaLink is provided.
	return gwacl.NewOSVirtualHardDisk("", "", "", mediaLink, sourceImageName, "Linux")
}

// newRole creates a gwacl.Role object (an Azure Virtual Machine) which uses
// the given Virtual Hard Drive.
// The VM will have:
// - an 'ubuntu' user defined with an unguessable (randomly generated) password
// - its ssh port (TCP 22) open
// - its state port (TCP mongoDB) port open
// - its API port (TCP) open
func (env *azureEnviron) newRole(vhd *gwacl.OSVirtualHardDisk, userData string, roleHostname string) *gwacl.Role {
	// TODO: Derive the role size from the constraints.
	// ExtraSmall|Small|Medium|Large|ExtraLarge
	roleSize := "Small"
	// Create a Linux Configuration with the username and the password
	// empty and disable SSH with password authentication.
	hostname := roleHostname
	username := "ubuntu"
	password := gwacl.MakeRandomPassword()
	linuxConfigurationSet := gwacl.NewLinuxProvisioningConfigurationSet(hostname, username, password, userData, "true")
	config := env.Config()
	// Generate a Network Configuration with the initially required ports
	// open.
	networkConfigurationSet := gwacl.NewNetworkConfigurationSet([]gwacl.InputEndpoint{
		{
			LocalPort: 22,
			Name:      "sshport",
			Port:      22,
			Protocol:  "TCP",
		},
		// TODO: Ought to have this only for state servers.
		{
			LocalPort: config.StatePort(),
			Name:      "stateport",
			Port:      config.StatePort(),
			Protocol:  "TCP",
		},
		// TODO: Ought to have this only for API servers.
		{
			LocalPort: config.APIPort(),
			Name:      "apiport",
			Port:      config.APIPort(),
			Protocol:  "TCP",
		},
	}, nil)
	roleName := gwacl.MakeRandomRoleName("juju")
	// The ordering of these configuration sets is significant for the tests.
	return gwacl.NewRole(
		roleSize, roleName,
		[]gwacl.ConfigurationSet{*linuxConfigurationSet, *networkConfigurationSet},
		[]gwacl.OSVirtualHardDisk{*vhd})
}

// newDeployment creates and returns a gwacl Deployment object.
func (env *azureEnviron) newDeployment(role *gwacl.Role, deploymentName string, deploymentLabel string, virtualNetworkName string) *gwacl.Deployment {
	// Use the service name as the label for the deployment.
	return gwacl.NewDeploymentForCreateVMDeployment(deploymentName, deploymentSlot, deploymentLabel, []gwacl.Role{*role}, virtualNetworkName)
}

// StartInstance is specified in the Environ interface.
// TODO(bug 1199847): This work can be shared between providers.
func (env *azureEnviron) StartInstance(machineID, machineNonce string, series string, cons constraints.Value,
	stateInfo *state.Info, apiInfo *api.Info) (instance.Instance, *instance.HardwareCharacteristics, error) {
	possibleTools, err := environs.FindInstanceTools(env, series, cons)
	if err != nil {
		return nil, nil, err
	}
	err = environs.CheckToolsSeries(possibleTools, series)
	if err != nil {
		return nil, nil, err
	}
	machineConfig := environs.NewMachineConfig(machineID, machineNonce, stateInfo, apiInfo)
	// TODO(bug 1193998) - return instance hardware characteristics as well.
	inst, err := env.internalStartInstance(cons, possibleTools, machineConfig)
	return inst, nil, err
}

// StopInstances is specified in the Environ interface.
func (env *azureEnviron) StopInstances(instances []instance.Instance) error {
	// Each Juju instance is an Azure Service (instance==service), destroy
	// all the Azure services.
	// Acquire management API object.
	context, err := env.getManagementAPI()
	if err != nil {
		return err
	}
	defer env.releaseManagementAPI(context)
	// Shut down all the instances; if there are errors, return only the
	// first one (but try to shut down all instances regardless).
	var firstErr error
	for _, instance := range instances {
		request := &gwacl.DestroyHostedServiceRequest{ServiceName: string(instance.Id())}
		err := context.DestroyHostedService(request)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Instances is specified in the Environ interface.
func (env *azureEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	// The instance list is built using the list of all the relevant
	// Azure Services (instance==service).
	// Acquire management API object.
	context, err := env.getManagementAPI()
	if err != nil {
		return nil, err
	}
	defer env.releaseManagementAPI(context)

	// Prepare gwacl request object.
	serviceNames := make([]string, len(ids))
	for i, id := range ids {
		serviceNames[i] = string(id)
	}
	request := &gwacl.ListSpecificHostedServicesRequest{ServiceNames: serviceNames}

	// Issue 'ListSpecificHostedServices' request with gwacl.
	services, err := context.ListSpecificHostedServices(request)
	if err != nil {
		return nil, err
	}

	// If no instances were found, return ErrNoInstances.
	if len(services) == 0 {
		return nil, environs.ErrNoInstances
	}

	instances := convertToInstances(services)

	// Check if we got a partial result.
	if len(ids) != len(instances) {
		return instances, environs.ErrPartialInstances
	}
	return instances, nil
}

// AllInstances is specified in the Environ interface.
func (env *azureEnviron) AllInstances() ([]instance.Instance, error) {
	// The instance list is built using the list of all the Azure
	// Services (instance==service).
	// Acquire management API object.
	context, err := env.getManagementAPI()
	if err != nil {
		return nil, err
	}
	defer env.releaseManagementAPI(context)

	request := &gwacl.ListPrefixedHostedServicesRequest{ServiceNamePrefix: env.getEnvPrefix()}
	services, err := context.ListPrefixedHostedServices(request)
	if err != nil {
		return nil, err
	}
	return convertToInstances(services), nil
}

// getEnvPrefix returns the prefix used to name the objects specific to this
// environment.
func (env *azureEnviron) getEnvPrefix() string {
	return fmt.Sprintf("juju-%s", env.Name())
}

// convertToInstances converts a slice of gwacl.HostedServiceDescriptor objects
// into a slice of instance.Instance objects.
func convertToInstances(services []gwacl.HostedServiceDescriptor) []instance.Instance {
	instances := make([]instance.Instance, len(services))
	for i, service := range services {
		instances[i] = &azureInstance{service}
	}
	return instances
}

// Storage is specified in the Environ interface.
func (env *azureEnviron) Storage() environs.Storage {
	return env.getSnapshot().storage
}

// PublicStorage is specified in the Environ interface.
func (env *azureEnviron) PublicStorage() environs.StorageReader {
	return env.getSnapshot().publicStorage
}

// Destroy is specified in the Environ interface.
func (env *azureEnviron) Destroy(ensureInsts []instance.Instance) error {
	logger.Debugf("destroying environment %q", env.name)

	// Delete storage.
	err := env.Storage().RemoveAll()
	if err != nil {
		return fmt.Errorf("cannot clean up storage: %v", err)
	}

	// Stop all instances.
	insts, err := env.AllInstances()
	if err != nil {
		return fmt.Errorf("cannot get instances: %v", err)
	}
	found := make(map[instance.Id]bool)
	for _, inst := range insts {
		found[inst.Id()] = true
	}

	// Add any instances we've been told about but haven't yet shown
	// up in the instance list.
	for _, inst := range ensureInsts {
		id := inst.Id()
		if !found[id] {
			insts = append(insts, inst)
			found[id] = true
		}
	}
	return env.StopInstances(insts)
}

// OpenPorts is specified in the Environ interface.
func (env *azureEnviron) OpenPorts(ports []instance.Port) error {
	// TODO: implement this.
	return nil
}

// ClosePorts is specified in the Environ interface.
func (env *azureEnviron) ClosePorts(ports []instance.Port) error {
	// TODO: implement this.
	return nil
}

// Ports is specified in the Environ interface.
func (env *azureEnviron) Ports() ([]instance.Port, error) {
	// TODO: implement this.
	return []instance.Port{}, nil
}

// Provider is specified in the Environ interface.
func (env *azureEnviron) Provider() environs.EnvironProvider {
	return azureEnvironProvider{}
}

// azureManagementContext wraps two things: a gwacl.ManagementAPI (effectively
// a session on the Azure management API) and a tempCertFile, which keeps track
// of the temporary certificate file that needs to be deleted once we're done
// with this particular session.
// Since it embeds *gwacl.ManagementAPI, you can use it much as if it were a
// pointer to a ManagementAPI object.  Just don't forget to release it after
// use.
type azureManagementContext struct {
	*gwacl.ManagementAPI
	certFile *tempCertFile
}

// getManagementAPI obtains a context object for interfacing with Azure's
// management API.
// For now, each invocation just returns a separate object.  This is probably
// wasteful (each context gets its own SSL connection) and may need optimizing
// later.
func (env *azureEnviron) getManagementAPI() (*azureManagementContext, error) {
	snap := env.getSnapshot()
	subscription := snap.ecfg.ManagementSubscriptionId()
	certData := snap.ecfg.ManagementCertificate()
	certFile, err := newTempCertFile([]byte(certData))
	if err != nil {
		return nil, err
	}
	// After this point, if we need to leave prematurely, we should clean
	// up that certificate file.
	mgtAPI, err := gwacl.NewManagementAPI(subscription, certFile.Path())
	if err != nil {
		certFile.Delete()
		return nil, err
	}
	context := azureManagementContext{
		ManagementAPI: mgtAPI,
		certFile:      certFile,
	}
	return &context, nil
}

// releaseManagementAPI frees up a context object obtained through
// getManagementAPI.
func (env *azureEnviron) releaseManagementAPI(context *azureManagementContext) {
	// Be tolerant to incomplete context objects, in case we ever get
	// called during cleanup of a failed attempt to create one.
	if context == nil || context.certFile == nil {
		return
	}
	// For now, all that needs doing is to delete the temporary certificate
	// file.  We may do cleverer things later, such as connection pooling
	// where this method returns a context to the pool.
	context.certFile.Delete()
}

// getStorageContext obtains a context object for interfacing with Azure's
// storage API.
// For now, each invocation just returns a separate object.  This is probably
// wasteful (each context gets its own SSL connection) and may need optimizing
// later.
func (env *azureEnviron) getStorageContext() (*gwacl.StorageContext, error) {
	ecfg := env.getSnapshot().ecfg
	context := gwacl.StorageContext{
		Account: ecfg.StorageAccountName(),
		Key:     ecfg.StorageAccountKey(),
	}
	// There is currently no way for this to fail.
	return &context, nil
}

// getPublicStorageContext obtains a context object for interfacing with
// Azure's storage API (public storage).
func (env *azureEnviron) getPublicStorageContext() (*gwacl.StorageContext, error) {
	ecfg := env.getSnapshot().ecfg
	context := gwacl.StorageContext{
		Account: ecfg.PublicStorageAccountName(),
		Key:     "", // Empty string means anonymous access.
	}
	// There is currently no way for this to fail.
	return &context, nil
}
