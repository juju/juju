// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	// service and deployment in Azure.  The deployment always gets this
	// name (instance==service).
	DeploymentName = "default"

	// Initially, this is the only location where Azure supports Linux.
	// TODO: This is to become a configuration item.
	serviceLocation = "East US"
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
	mcfg := env.makeMachineConfig(machineID, state.BootstrapNonce, nil, nil)
	mcfg.StateServer = true

	logger.Debugf("bootstrapping environment %q", env.Name())
	possibleTools, err := environs.FindBootstrapTools(env, cons)
	if err != nil {
		return nil, err
	}
	inst, err := env.internalStartInstance(machineID, cons, possibleTools, mcfg)
	if err != nil {
		return nil, fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	return inst, nil
}

// Bootstrap is specified in the Environ interface.
// TODO(bug 1199847): This work can be shared between providers.
func (env *azureEnviron) Bootstrap(cons constraints.Value) error {
	if err := environs.VerifyBootstrapInit(env, shortAttempt); err != nil {
		return err
	}

	inst, err := env.startBootstrapInstance(cons)
	if err != nil {
		return err
	}
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

// makeProvisionalServiceLabel generates a label for a new Hosted Service of
// the given name.  The label can be identified as provisional using
// isProvisionalDeploymentLabel().  (Empty labels are not allowed).
// In our initial implementation, each instance gets its own Azure hosted
// service.  Once we have a DNS name for the deployment, we write it into the
// Label field on the hosted service as a shortcut.
// This will have to change once we suppport multiple instances per hosted
// service (instance==service).
func makeProvisionalServiceLabel(serviceName string) string {
	return fmt.Sprintf("-(creating: %s)-", serviceName)
}

// isProvisionalDeploymentLabel tells you whether the given label is a
// provisional one.  If not, the provider has set it to the DNS name for the
// service's deployment.
func isProvisionalServiceLabel(label string) bool {
	return strings.HasPrefix(label, "-(") && strings.HasSuffix(label, ")-")
}

// attemptCreateService tries to create a new hosted service on Azure, with a
// name it chooses, but recognizes that the name may not be available.  If
// the name is not available, it does not treat that as an error but just
// returns nil.
func attemptCreateService(azure *gwacl.ManagementAPI) (*gwacl.CreateHostedService, error) {
	name := gwacl.MakeRandomHostedServiceName("juju")
	label := makeProvisionalServiceLabel(name)
	req := gwacl.NewCreateHostedServiceWithLocation(name, label, serviceLocation)
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

// newHostedService creates a hosted service.  It will make up a unique name.
func newHostedService(azure *gwacl.ManagementAPI) (*gwacl.CreateHostedService, error) {
	var err error
	var svc *gwacl.CreateHostedService
	for tries := 10; tries > 0 && err == nil && svc == nil; tries-- {
		svc, err = attemptCreateService(azure)
	}
	if err != nil {
		return nil, fmt.Errorf("could not create hosted service: %v", err)
	}
	if svc == nil {
		return nil, fmt.Errorf("could not come up with a unique hosted service name - is your randomizer initialized?")
	}
	return svc, nil
}

// extractDeploymentDNS extracts an instance's DNS name from its URL.
func extractDeploymentDNS(instanceURL string) (string, error) {
	parsedURL, err := url.Parse(instanceURL)
	if err != nil {
		return "", fmt.Errorf("parse error in instance URL: %v", err)
	}
	// net.url.URL.Host actually includes a port spec if the URL has one,
	// but luckily a port wouldn't make sense on these URLs.
	return parsedURL.Host, nil
}

// setServiceDNSName updates the hosted service's label to match the DNS name
// for the Deployment.
func setServiceDNSName(azure *gwacl.ManagementAPI, serviceName, deploymentName string) error {
	deployment, err := azure.GetDeployment(&gwacl.GetDeploymentRequest{
		ServiceName:    serviceName,
		DeploymentName: deploymentName,
	})
	if err != nil {
		return fmt.Errorf("could not read newly created deployment: %v", err)
	}
	host, err := extractDeploymentDNS(deployment.URL)
	if err != nil {
		return fmt.Errorf("could not parse instance URL %q: %v", deployment.URL, err)
	}

	update := gwacl.NewUpdateHostedService(host, "Juju instance", nil)
	return azure.UpdateHostedService(serviceName, update)
}

// internalStartInstance does the provider-specific work of starting an
// instance.  The code in StartInstance is actually largely agnostic across
// the EC2/OpenStack/MAAS/Azure providers.
// TODO(bug 1199847): Some of this work can be shared between providers.
func (env *azureEnviron) internalStartInstance(machineID string, cons constraints.Value, possibleTools tools.List, mcfg *cloudinit.MachineConfig) (_ instance.Instance, err error) {
	// Declaring "err" in the function signature so that we can "defer"
	// any cleanup that needs to run during error returns.

	series := possibleTools.Series()
	if len(series) != 1 {
		return nil, fmt.Errorf("expected single series, got %v", series)
	}

	err = environs.FinishMachineConfig(mcfg, env.Config(), cons)
	if err != nil {
		return nil, err
	}

	// TODO: Compose userdata.

	azure, err := env.getManagementAPI()
	if err != nil {
		return nil, err
	}
	defer env.releaseManagementAPI(azure)

	service, err := newHostedService(azure.ManagementAPI)
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

	// The virtual network to which the deployment will belong.  We'll
	// want to build this out later to support private communication
	// between instances.
	virtualNetworkName := ""

	// TODO: Create or find role.
	var roles []gwacl.Role
	deployment := gwacl.NewDeploymentForCreateVMDeployment(DeploymentName, "Production", serviceName, roles, virtualNetworkName)
	err = azure.AddDeployment(deployment, serviceName)

	// TODO: Create inst.
	var inst instance.Instance
	// TODO: Make sure at least the ssh port is open.

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

	err = setServiceDNSName(azure.ManagementAPI, serviceName, deployment.Name)
	if err != nil {
		return nil, fmt.Errorf("could not set instance DNS name as service label: %v", err)
	}

	return inst, nil
}

// makeMachineConfig sets up a basic machine configuration for use with
// userData().  You may still need to supply more information, but this takes
// care of the fixed entries and the ones that are always needed.
// TODO(bug 1199847): This work can be shared between providers.
func (env *azureEnviron) makeMachineConfig(machineID, machineNonce string,
	stateInfo *state.Info, apiInfo *api.Info) *cloudinit.MachineConfig {
	return &cloudinit.MachineConfig{
		// Fixed entries.
		// TODO: Unify instances of this path, so tests can fake it.
		DataDir: "/var/lib/juju",

		// Parameter entries.
		MachineId:    machineID,
		MachineNonce: machineNonce,
		StateInfo:    stateInfo,
		APIInfo:      apiInfo,
	}
}

// StartInstance is specified in the Environ interface.
// TODO(bug 1199847): This work can be shared between providers.
func (env *azureEnviron) StartInstance(machineID, machineNonce string, series string, cons constraints.Value,
	stateInfo *state.Info, apiInfo *api.Info) (instance.Instance, *instance.HardwareCharacteristics, error) {
	possibleTools, err := environs.FindInstanceTools(env, series, cons)
	if err != nil {
		return nil, nil, err
	}
	mcfg := env.makeMachineConfig(machineID, machineNonce, stateInfo, apiInfo)
	// TODO(bug 1193998) - return instance hardware characteristics as well.
	inst, err := env.internalStartInstance(machineID, cons, possibleTools, mcfg)
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
	panic("unimplemented")
}

// ClosePorts is specified in the Environ interface.
func (env *azureEnviron) ClosePorts(ports []instance.Port) error {
	panic("unimplemented")
}

// Ports is specified in the Environ interface.
func (env *azureEnviron) Ports() ([]instance.Port, error) {
	panic("unimplemented")
}

// Provider is specified in the Environ interface.
func (env *azureEnviron) Provider() environs.EnvironProvider {
	panic("unimplemented")
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
