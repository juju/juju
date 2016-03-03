// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"gopkg.in/goose.v1/client"
	gooseerrors "gopkg.in/goose.v1/errors"
	"gopkg.in/goose.v1/identity"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.provider.openstack")

type EnvironProvider struct {
	environs.ProviderCredentials
	Configurator      ProviderConfigurator
	FirewallerFactory FirewallerFactory
}

var _ environs.EnvironProvider = (*EnvironProvider)(nil)

var providerInstance *EnvironProvider = &EnvironProvider{
	OpenstackCredentials{},
	&defaultConfigurator{},
	&firewallerFactory{},
}

var makeServiceURL = client.AuthenticatingClient.MakeServiceURL

// Use shortAttempt to poll for short-term events.
// TODO: This was kept to a long timeout because Nova needs more time than EC2.
// For example, HP Cloud takes around 9.1 seconds (10 samples) to return a
// BUILD(spawning) status. But storage delays are handled separately now, and
// perhaps other polling attempts can time out faster.
var shortAttempt = utils.AttemptStrategy{
	Total: 15 * time.Second,
	Delay: 200 * time.Millisecond,
}

func (p EnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening model %q", cfg.Name())
	e := new(Environ)

	e.firewaller = p.FirewallerFactory.GetFirewaller(e)
	e.configurator = p.Configurator
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	e.name = cfg.Name()
	return e, nil
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p EnvironProvider) RestrictedConfigAttributes() []string {
	return []string{"region", "auth-url", "auth-mode"}
}

// DetectRegions implements environs.CloudRegionDetector.
func (EnvironProvider) DetectRegions() ([]cloud.Region, error) {
	// If OS_REGION_NAME and OS_AUTH_URL are both set,
	// return return a region using them.
	creds := identity.CredentialsFromEnv()
	if creds.Region == "" {
		return nil, errors.NewNotFound(nil, "OS_REGION_NAME environment variable not set")
	}
	if creds.URL == "" {
		return nil, errors.NewNotFound(nil, "OS_AUTH_URL environment variable not set")
	}
	return []cloud.Region{{
		Name:     creds.Region,
		Endpoint: creds.URL,
	}}, nil
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p EnvironProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

func (p EnvironProvider) PrepareForBootstrap(
	ctx environs.BootstrapContext,
	args environs.PrepareForBootstrapParams,
) (environs.Environ, error) {

	// Add credentials to the configuration.
	attrs := map[string]interface{}{
		"region":   args.CloudRegion,
		"auth-url": args.CloudEndpoint,
	}
	credentialAttrs := args.Credentials.Attributes()
	switch authType := args.Credentials.AuthType(); authType {
	case cloud.UserPassAuthType:
		// TODO(axw) we need a way of saying to use legacy auth.
		attrs["username"] = credentialAttrs["username"]
		attrs["password"] = credentialAttrs["password"]
		attrs["tenant-name"] = credentialAttrs["tenant-name"]
		attrs["auth-mode"] = AuthUserPass
	case cloud.AccessKeyAuthType:
		attrs["access-key"] = credentialAttrs["access-key"]
		attrs["secret-key"] = credentialAttrs["secret-key"]
		attrs["tenant-name"] = credentialAttrs["tenant-name"]
		attrs["auth-mode"] = AuthKeyPair
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}
	cfg, err := args.Config.Apply(attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg, err = p.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, err
	}
	e, err := p.Open(cfg)
	if err != nil {
		return nil, err
	}
	// Verify credentials.
	if err := authenticateClient(e.(*Environ)); err != nil {
		return nil, err
	}
	return e, nil
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p EnvironProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, fmt.Errorf("region must be specified")
	}
	return &simplestreams.MetadataLookupParams{
		Region:        region,
		Architectures: arch.AllSupportedArches,
	}, nil
}

func (p EnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	m := make(map[string]string)
	ecfg, err := p.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	m["username"] = ecfg.username()
	m["password"] = ecfg.password()
	m["tenant-name"] = ecfg.tenantName()
	return m, nil
}

func (p EnvironProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func retryGet(uri string) (data []byte, err error) {
	for a := shortAttempt.Start(); a.Next(); {
		var resp *http.Response
		resp, err = http.Get(uri)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("bad http response %v", resp.Status)
			continue
		}
		var data []byte
		data, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		return data, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get %q: %v", uri, err)
	}
	return
}

type Environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	// archMutex gates access to supportedArchitectures
	archMutex sync.Mutex
	// supportedArchitectures caches the architectures
	// for which images can be instantiated.
	supportedArchitectures []string

	ecfgMutex    sync.Mutex
	ecfgUnlocked *environConfig
	client       client.AuthenticatingClient
	novaUnlocked *nova.Client

	// keystoneImageDataSource caches the result of getKeystoneImageSource.
	keystoneImageDataSourceMutex sync.Mutex
	keystoneImageDataSource      simplestreams.DataSource

	// keystoneToolsDataSource caches the result of getKeystoneToolsSource.
	keystoneToolsDataSourceMutex sync.Mutex
	keystoneToolsDataSource      simplestreams.DataSource

	availabilityZonesMutex sync.Mutex
	availabilityZones      []common.AvailabilityZone
	firewaller             Firewaller
	configurator           ProviderConfigurator
}

var _ environs.Environ = (*Environ)(nil)
var _ simplestreams.HasRegion = (*Environ)(nil)
var _ state.Prechecker = (*Environ)(nil)
var _ state.InstanceDistributor = (*Environ)(nil)
var _ environs.InstanceTagger = (*Environ)(nil)

type openstackInstance struct {
	e        *Environ
	instType *instances.InstanceType
	arch     *string

	mu           sync.Mutex
	serverDetail *nova.ServerDetail
	// floatingIP is non-nil iff use-floating-ip is true.
	floatingIP *nova.FloatingIP
}

func (inst *openstackInstance) String() string {
	return string(inst.Id())
}

var _ instance.Instance = (*openstackInstance)(nil)

func (inst *openstackInstance) Refresh() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	server, err := inst.e.nova().GetServer(inst.serverDetail.Id)
	if err != nil {
		return err
	}
	inst.serverDetail = server
	return nil
}

func (inst *openstackInstance) getServerDetail() *nova.ServerDetail {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.serverDetail
}

func (inst *openstackInstance) Id() instance.Id {
	return instance.Id(inst.getServerDetail().Id)
}

func (inst *openstackInstance) Status() string {
	return inst.getServerDetail().Status
}

func (inst *openstackInstance) hardwareCharacteristics() *instance.HardwareCharacteristics {
	hc := &instance.HardwareCharacteristics{Arch: inst.arch}
	if inst.instType != nil {
		hc.Mem = &inst.instType.Mem
		// openstack is special in that a 0-size root disk means that
		// the root disk will result in an instance with a root disk
		// the same size as the image that created it, so we just set
		// the HardwareCharacteristics to nil to signal that we don't
		// know what the correct size is.
		if inst.instType.RootDisk == 0 {
			hc.RootDisk = nil
		} else {
			hc.RootDisk = &inst.instType.RootDisk
		}
		hc.CpuCores = &inst.instType.CpuCores
		hc.CpuPower = inst.instType.CpuPower
		// tags not currently supported on openstack
	}
	hc.AvailabilityZone = &inst.serverDetail.AvailabilityZone
	return hc
}

// getAddresses returns the existing server information on addresses,
// but fetches the details over the api again if no addresses exist.
func (inst *openstackInstance) getAddresses() (map[string][]nova.IPAddress, error) {
	addrs := inst.getServerDetail().Addresses
	if len(addrs) == 0 {
		server, err := inst.e.nova().GetServer(string(inst.Id()))
		if err != nil {
			return nil, err
		}
		addrs = server.Addresses
	}
	return addrs, nil
}

// Addresses implements network.Addresses() returning generic address
// details for the instances, and calling the openstack api if needed.
func (inst *openstackInstance) Addresses() ([]network.Address, error) {
	addresses, err := inst.getAddresses()
	if err != nil {
		return nil, err
	}
	var floatingIP string
	if inst.floatingIP != nil && inst.floatingIP.IP != "" {
		floatingIP = inst.floatingIP.IP
		logger.Debugf("instance %v has floating IP address: %v", inst.Id(), floatingIP)
	}
	return convertNovaAddresses(floatingIP, addresses), nil
}

// convertNovaAddresses returns nova addresses in generic format
func convertNovaAddresses(publicIP string, addresses map[string][]nova.IPAddress) []network.Address {
	var machineAddresses []network.Address
	if publicIP != "" {
		publicAddr := network.NewScopedAddress(publicIP, network.ScopePublic)
		publicAddr.NetworkName = "public"
		machineAddresses = append(machineAddresses, publicAddr)
	}
	// TODO(gz) Network ordering may be significant but is not preserved by
	// the map, see lp:1188126 for example. That could potentially be fixed
	// in goose, or left to be derived by other means.
	for netName, ips := range addresses {
		networkScope := network.ScopeUnknown
		if netName == "public" {
			networkScope = network.ScopePublic
		}
		for _, address := range ips {
			// If this address has already been added as a floating IP, skip it.
			if publicIP == address.Address {
				continue
			}
			// Assume IPv4 unless specified otherwise
			addrtype := network.IPv4Address
			if address.Version == 6 {
				addrtype = network.IPv6Address
			}
			machineAddr := network.NewScopedAddress(address.Address, networkScope)
			machineAddr.NetworkName = netName
			if machineAddr.Type != addrtype {
				logger.Warningf("derived address type %v, nova reports %v", machineAddr.Type, addrtype)
			}
			machineAddresses = append(machineAddresses, machineAddr)
		}
	}
	return machineAddresses
}

func (inst *openstackInstance) OpenPorts(machineId string, ports []network.PortRange) error {
	return inst.e.firewaller.OpenInstancePorts(inst, machineId, ports)
}

func (inst *openstackInstance) ClosePorts(machineId string, ports []network.PortRange) error {
	return inst.e.firewaller.CloseInstancePorts(inst, machineId, ports)
}

func (inst *openstackInstance) Ports(machineId string) ([]network.PortRange, error) {
	return inst.e.firewaller.InstancePorts(inst, machineId)
}

func (e *Environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *Environ) nova() *nova.Client {
	e.ecfgMutex.Lock()
	nova := e.novaUnlocked
	e.ecfgMutex.Unlock()
	return nova
}

// SupportedArchitectures is specified on the EnvironCapability interface.
func (e *Environ) SupportedArchitectures() ([]string, error) {
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

var unsupportedConstraints = []string{
	constraints.Tags,
	constraints.CpuPower,
}

// ConstraintsValidator is defined on the Environs interface.
func (e *Environ) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterConflicts(
		[]string{constraints.InstanceType},
		[]string{constraints.Mem, constraints.Arch, constraints.RootDisk, constraints.CpuCores})
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := e.SupportedArchitectures()
	if err != nil {
		return nil, err
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)
	novaClient := e.nova()
	flavors, err := novaClient.ListFlavorsDetail()
	if err != nil {
		return nil, err
	}
	instTypeNames := make([]string, len(flavors))
	for i, flavor := range flavors {
		instTypeNames[i] = flavor.Name
	}
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)
	return validator, nil
}

var novaListAvailabilityZones = (*nova.Client).ListAvailabilityZones

type openstackAvailabilityZone struct {
	nova.AvailabilityZone
}

func (z *openstackAvailabilityZone) Name() string {
	return z.AvailabilityZone.Name
}

func (z *openstackAvailabilityZone) Available() bool {
	return z.AvailabilityZone.State.Available
}

// AvailabilityZones returns a slice of availability zones.
func (e *Environ) AvailabilityZones() ([]common.AvailabilityZone, error) {
	e.availabilityZonesMutex.Lock()
	defer e.availabilityZonesMutex.Unlock()
	if e.availabilityZones == nil {
		zones, err := novaListAvailabilityZones(e.nova())
		if gooseerrors.IsNotImplemented(err) {
			return nil, errors.NotImplementedf("availability zones")
		}
		if err != nil {
			return nil, err
		}
		e.availabilityZones = make([]common.AvailabilityZone, len(zones))
		for i, z := range zones {
			e.availabilityZones[i] = &openstackAvailabilityZone{z}
		}
	}
	return e.availabilityZones, nil
}

// InstanceAvailabilityZoneNames returns the availability zone names for each
// of the specified instances.
func (e *Environ) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	instances, err := e.Instances(ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}
	zones := make([]string, len(instances))
	for i, inst := range instances {
		if inst == nil {
			continue
		}
		zones[i] = inst.(*openstackInstance).serverDetail.AvailabilityZone
	}
	return zones, err
}

type openstackPlacement struct {
	availabilityZone nova.AvailabilityZone
}

func (e *Environ) parsePlacement(placement string) (*openstackPlacement, error) {
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
				return &openstackPlacement{
					z.(*openstackAvailabilityZone).AvailabilityZone,
				}, nil
			}
		}
		return nil, fmt.Errorf("invalid availability zone %q", availabilityZone)
	}
	return nil, fmt.Errorf("unknown placement directive: %v", placement)
}

// PrecheckInstance is defined on the state.Prechecker interface.
func (e *Environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" {
		if _, err := e.parsePlacement(placement); err != nil {
			return err
		}
	}
	if !cons.HasInstanceType() {
		return nil
	}
	// Constraint has an instance-type constraint so let's see if it is valid.
	novaClient := e.nova()
	flavors, err := novaClient.ListFlavorsDetail()
	if err != nil {
		return err
	}
	for _, flavor := range flavors {
		if flavor.Name == *cons.InstanceType {
			return nil
		}
	}
	return fmt.Errorf("invalid Openstack flavour %q specified", *cons.InstanceType)
}

func (e *Environ) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	// The client's authentication may have been reset when finding tools if the agent-version
	// attribute was updated so we need to re-authenticate. This will be a no-op if already authenticated.
	// An authenticated client is needed for the URL() call below.
	if err := authenticateClient(e); err != nil {
		return nil, err
	}
	return common.Bootstrap(ctx, e, args)
}

func (e *Environ) ControllerInstances() ([]instance.Id, error) {
	// Find all instances tagged with tags.JujuController.
	instances, err := e.AllInstances()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ids := make([]instance.Id, 0, 1)
	for _, instance := range instances {
		detail := instance.(*openstackInstance).getServerDetail()
		if detail.Metadata[tags.JujuController] == "true" {
			ids = append(ids, instance.Id())
		}
	}
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}
	return ids, nil
}

func (e *Environ) Config() *config.Config {
	return e.ecfg().Config
}

func authClient(ecfg *environConfig) client.AuthenticatingClient {
	cred := &identity.Credentials{
		User:       ecfg.username(),
		Secrets:    ecfg.password(),
		Region:     ecfg.region(),
		TenantName: ecfg.tenantName(),
		URL:        ecfg.authURL(),
	}
	// authModeCfg has already been validated so we know it's one of the values below.
	var authMode identity.AuthMode
	switch AuthMode(ecfg.authMode()) {
	case AuthLegacy:
		authMode = identity.AuthLegacy
	case AuthUserPass:
		authMode = identity.AuthUserPass
	case AuthKeyPair:
		authMode = identity.AuthKeyPair
		cred.User = ecfg.accessKey()
		cred.Secrets = ecfg.secretKey()
	}
	newClient := client.NewClient
	if !ecfg.SSLHostnameVerification() {
		newClient = client.NewNonValidatingClient
	}
	client := newClient(cred, authMode, nil)
	// By default, the client requires "compute" and
	// "object-store". Juju only requires "compute".
	client.SetRequiredServiceTypes([]string{"compute"})
	return client
}

var authenticateClient = func(e *Environ) error {
	err := e.client.Authenticate()
	if err != nil {
		// Log the error in case there are any useful hints,
		// but provide a readable and helpful error message
		// to the user.
		logger.Debugf("authentication failed: %v", err)
		return errors.New(`authentication failed.

Please ensure the credentials are correct. A common mistake is
to specify the wrong tenant. Use the OpenStack "project" name
for tenant-name in your model configuration.`)
	}
	return nil
}

func (e *Environ) SetConfig(cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}
	// At this point, the authentication method config value has been validated so we extract it's value here
	// to avoid having to validate again each time when creating the OpenStack client.
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	e.ecfgUnlocked = ecfg

	e.client = authClient(ecfg)

	e.novaUnlocked = nova.New(e.client)
	return nil
}

// getKeystoneImageSource is an imagemetadata.ImageDataSourceFunc that
// returns a DataSource using the "product-streams" keystone URL.
func getKeystoneImageSource(env environs.Environ) (simplestreams.DataSource, error) {
	e, ok := env.(*Environ)
	if !ok {
		return nil, errors.NotSupportedf("non-openstack model")
	}
	return e.getKeystoneDataSource(&e.keystoneImageDataSourceMutex, &e.keystoneImageDataSource, "product-streams")
}

// getKeystoneToolsSource is a tools.ToolsDataSourceFunc that
// returns a DataSource using the "juju-tools" keystone URL.
func getKeystoneToolsSource(env environs.Environ) (simplestreams.DataSource, error) {
	e, ok := env.(*Environ)
	if !ok {
		return nil, errors.NotSupportedf("non-openstack model")
	}
	return e.getKeystoneDataSource(&e.keystoneToolsDataSourceMutex, &e.keystoneToolsDataSource, "juju-tools")
}

func (e *Environ) getKeystoneDataSource(mu *sync.Mutex, datasource *simplestreams.DataSource, keystoneName string) (simplestreams.DataSource, error) {
	mu.Lock()
	defer mu.Unlock()
	if *datasource != nil {
		return *datasource, nil
	}
	if !e.client.IsAuthenticated() {
		if err := authenticateClient(e); err != nil {
			return nil, err
		}
	}

	url, err := makeServiceURL(e.client, keystoneName, nil)
	if err != nil {
		return nil, errors.NewNotSupported(err, fmt.Sprintf("cannot make service URL: %v", err))
	}
	verify := utils.VerifySSLHostnames
	if !e.Config().SSLHostnameVerification() {
		verify = utils.NoVerifySSLHostnames
	}
	*datasource = simplestreams.NewURLDataSource("keystone catalog", url, verify, simplestreams.SPECIFIC_CLOUD_DATA, false)
	return *datasource, nil
}

// resolveNetwork takes either a network id or label and returns a network id
func (e *Environ) resolveNetwork(networkName string) (string, error) {
	if utils.IsValidUUIDString(networkName) {
		// Network id supplied, assume valid as boot will fail if not
		return networkName, nil
	}
	// Network label supplied, resolve to a network id
	networks, err := e.nova().ListNetworks()
	if err != nil {
		return "", err
	}
	var networkIds = []string{}
	for _, network := range networks {
		if network.Label == networkName {
			networkIds = append(networkIds, network.Id)
		}
	}
	switch len(networkIds) {
	case 1:
		return networkIds[0], nil
	case 0:
		return "", fmt.Errorf("No networks exist with label %q", networkName)
	}
	return "", fmt.Errorf("Multiple networks with label %q: %v", networkName, networkIds)
}

// allocatePublicIP tries to find an available floating IP address, or
// allocates a new one, returning it, or an error
func (e *Environ) allocatePublicIP() (*nova.FloatingIP, error) {
	fips, err := e.nova().ListFloatingIPs()
	if err != nil {
		return nil, err
	}
	var newfip *nova.FloatingIP
	for _, fip := range fips {
		newfip = &fip
		if fip.InstanceId != nil && *fip.InstanceId != "" {
			// unavailable, skip
			newfip = nil
			continue
		} else {
			logger.Debugf("found unassigned public ip: %v", newfip.IP)
			// unassigned, we can use it
			return newfip, nil
		}
	}
	if newfip == nil {
		// allocate a new IP and use it
		newfip, err = e.nova().AllocateFloatingIP()
		if err != nil {
			return nil, err
		}
		logger.Debugf("allocated new public IP: %v", newfip.IP)
	}
	return newfip, nil
}

// assignPublicIP tries to assign the given floating IP address to the
// specified server, or returns an error.
func (e *Environ) assignPublicIP(fip *nova.FloatingIP, serverId string) (err error) {
	if fip == nil {
		return fmt.Errorf("cannot assign a nil public IP to %q", serverId)
	}
	if fip.InstanceId != nil && *fip.InstanceId == serverId {
		// IP already assigned, nothing to do
		return nil
	}
	// At startup nw_info is not yet cached so this may fail
	// temporarily while the server is being built
	for a := common.LongAttempt.Start(); a.Next(); {
		err = e.nova().AddServerFloatingIP(serverId, fip.IP)
		if err == nil {
			return nil
		}
	}
	return err
}

// DistributeInstances implements the state.InstanceDistributor policy.
func (e *Environ) DistributeInstances(candidates, distributionGroup []instance.Id) ([]instance.Id, error) {
	return common.DistributeInstances(e, candidates, distributionGroup)
}

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

// MaintainInstance is specified in the InstanceBroker interface.
func (*Environ) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (e *Environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	var availabilityZones []string
	if args.Placement != "" {
		placement, err := e.parsePlacement(args.Placement)
		if err != nil {
			return nil, err
		}
		if !placement.availabilityZone.State.Available {
			return nil, fmt.Errorf("availability zone %q is unavailable", placement.availabilityZone.Name)
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
		if errors.IsNotImplemented(err) {
			// Availability zones are an extension, so we may get a
			// not implemented error; ignore these.
		} else if err != nil {
			return nil, err
		} else {
			for _, zone := range zoneInstances {
				availabilityZones = append(availabilityZones, zone.ZoneName)
			}
		}
		if len(availabilityZones) == 0 {
			// No explicitly selectable zones available, so use an unspecified zone.
			availabilityZones = []string{""}
		}
	}

	if args.InstanceConfig.HasNetworks() {
		return nil, fmt.Errorf("starting instances with networks is not supported yet.")
	}

	series := args.Tools.OneSeries()
	arches := args.Tools.Arches()
	spec, err := findInstanceSpec(e, &instances.InstanceConstraint{
		Region:      e.ecfg().region(),
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
	}, args.ImageMetadata)
	if err != nil {
		return nil, err
	}
	tools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, fmt.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	args.InstanceConfig.Tools = tools[0]

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, e.Config()); err != nil {
		return nil, err
	}
	cloudcfg, err := e.configurator.GetCloudConfig(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, OpenstackRenderer{})
	if err != nil {
		return nil, fmt.Errorf("cannot make user data: %v", err)
	}
	logger.Debugf("openstack user data; %d bytes", len(userData))

	var networks = e.firewaller.InitialNetworks()
	usingNetwork := e.ecfg().network()
	if usingNetwork != "" {
		networkId, err := e.resolveNetwork(usingNetwork)
		if err != nil {
			return nil, err
		}
		logger.Debugf("using network id %q", networkId)
		networks = append(networks, nova.ServerNetworks{NetworkId: networkId})
	}
	withPublicIP := e.ecfg().useFloatingIP()
	var publicIP *nova.FloatingIP
	if withPublicIP {
		logger.Debugf("allocating public IP address for openstack node")
		if fip, err := e.allocatePublicIP(); err != nil {
			return nil, fmt.Errorf("cannot allocate a public IP as needed: %v", err)
		} else {
			publicIP = fip
			logger.Infof("allocated public IP %s", publicIP.IP)
		}
	}

	cfg := e.Config()
	var groupNames = make([]nova.SecurityGroupName, 0)
	groups, err := e.firewaller.SetUpGroups(args.InstanceConfig.MachineId, cfg.APIPort())
	if err != nil {
		return nil, fmt.Errorf("cannot set up groups: %v", err)
	}

	for _, g := range groups {
		groupNames = append(groupNames, nova.SecurityGroupName{g.Name})
	}
	eUUID, ok := e.Config().UUID()
	if !ok {
		return nil, errors.NotFoundf("UUID in environ config")
	}
	machineName := resourceName(
		names.NewMachineTag(args.InstanceConfig.MachineId),
		eUUID,
	)

	var server *nova.Entity
	for _, availZone := range availabilityZones {
		var opts = nova.RunServerOpts{
			Name:               machineName,
			FlavorId:           spec.InstanceType.Id,
			ImageId:            spec.Image.Id,
			UserData:           userData,
			SecurityGroupNames: groupNames,
			Networks:           networks,
			AvailabilityZone:   availZone,
			Metadata:           args.InstanceConfig.Tags,
		}
		e.configurator.ModifyRunServerOptions(&opts)
		for a := shortAttempt.Start(); a.Next(); {
			server, err = e.nova().RunServer(opts)
			if err == nil || !gooseerrors.IsNotFound(err) {
				break
			}
		}
		if isNoValidHostsError(err) {
			logger.Infof("no valid hosts available in zone %q, trying another availability zone", availZone)
		} else {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("cannot run instance: %v", err)
	}
	detail, err := e.nova().GetServer(server.Id)
	if err != nil {
		return nil, fmt.Errorf("cannot get started instance: %v", err)
	}
	inst := &openstackInstance{
		e:            e,
		serverDetail: detail,
		arch:         &spec.Image.Arch,
		instType:     &spec.InstanceType,
	}
	logger.Infof("started instance %q", inst.Id())
	if withPublicIP {
		if err := e.assignPublicIP(publicIP, string(inst.Id())); err != nil {
			if err := e.terminateInstances([]instance.Id{inst.Id()}); err != nil {
				// ignore the failure at this stage, just log it
				logger.Debugf("failed to terminate instance %q: %v", inst.Id(), err)
			}
			return nil, fmt.Errorf("cannot assign public address %s to instance %q: %v", publicIP.IP, inst.Id(), err)
		}
		inst.floatingIP = publicIP
		logger.Infof("assigned public IP %s to %q", publicIP.IP, inst.Id())
	}
	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: inst.hardwareCharacteristics(),
	}, nil
}

func isNoValidHostsError(err error) bool {
	gooseErr, ok := err.(gooseerrors.Error)
	return ok && strings.Contains(gooseErr.Cause().Error(), "No valid host was found")
}

func (e *Environ) StopInstances(ids ...instance.Id) error {
	// If in instance firewall mode, gather the security group names.
	securityGroupNames, err := e.firewaller.GetSecurityGroups(ids...)
	if err == environs.ErrNoInstances {
		return nil
	}
	if err != nil {
		return err
	}
	logger.Debugf("terminating instances %v", ids)
	if err := e.terminateInstances(ids); err != nil {
		return err
	}
	if securityGroupNames != nil {
		return e.deleteSecurityGroups(securityGroupNames)
	}
	return nil
}

func (e *Environ) isAliveServer(server nova.ServerDetail) bool {
	switch server.Status {
	// HPCloud uses "BUILD(spawning)" as an intermediate BUILD state
	// once networking is available.
	case nova.StatusActive, nova.StatusBuild, nova.StatusBuildSpawning, nova.StatusShutoff, nova.StatusSuspended:
		return true
	}
	return false
}

func (e *Environ) listServers(ids []instance.Id) ([]nova.ServerDetail, error) {
	wantedServers := make([]nova.ServerDetail, 0, len(ids))
	if len(ids) == 1 {
		// Common case, single instance, may return NotFound
		var maybeServer *nova.ServerDetail
		maybeServer, err := e.nova().GetServer(string(ids[0]))
		if err != nil {
			return nil, err
		}
		// Only return server details if it is currently alive
		if maybeServer != nil && e.isAliveServer(*maybeServer) {
			wantedServers = append(wantedServers, *maybeServer)
		}
		return wantedServers, nil
	}
	// List all servers that may be in the environment
	servers, err := e.nova().ListServersDetail(e.machinesFilter())
	if err != nil {
		return nil, err
	}
	// Create a set of the ids of servers that are wanted
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[string(id)] = struct{}{}
	}
	// Return only servers with the wanted ids that are currently alive
	for _, server := range servers {
		if _, ok := idSet[server.Id]; ok && e.isAliveServer(server) {
			wantedServers = append(wantedServers, server)
		}
	}
	return wantedServers, nil
}

// updateFloatingIPAddresses updates the instances with any floating IP address
// that have been assigned to those instances.
func (e *Environ) updateFloatingIPAddresses(instances map[string]instance.Instance) error {
	fips, err := e.nova().ListFloatingIPs()
	if err != nil {
		return err
	}
	for _, fip := range fips {
		if fip.InstanceId != nil && *fip.InstanceId != "" {
			instId := *fip.InstanceId
			if inst, ok := instances[instId]; ok {
				instFip := fip
				inst.(*openstackInstance).floatingIP = &instFip
			}
		}
	}
	return nil
}

func (e *Environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Make a series of requests to cope with eventual consistency.
	// Each request will attempt to add more instances to the requested
	// set.
	var foundServers []nova.ServerDetail
	for a := shortAttempt.Start(); a.Next(); {
		var err error
		foundServers, err = e.listServers(ids)
		if err != nil {
			logger.Debugf("error listing servers: %v", err)
			if !gooseerrors.IsNotFound(err) {
				return nil, err
			}
		}
		if len(foundServers) == len(ids) {
			break
		}
	}
	logger.Tracef("%d/%d live servers found", len(foundServers), len(ids))
	if len(foundServers) == 0 {
		return nil, environs.ErrNoInstances
	}

	instsById := make(map[string]instance.Instance, len(foundServers))
	for i, server := range foundServers {
		// TODO(wallyworld): lookup the flavor details to fill in the
		// instance type data
		instsById[server.Id] = &openstackInstance{
			e:            e,
			serverDetail: &foundServers[i],
		}
	}

	// Update the instance structs with any floating IP address that has been assigned to the instance.
	if e.ecfg().useFloatingIP() {
		if err := e.updateFloatingIPAddresses(instsById); err != nil {
			return nil, err
		}
	}

	insts := make([]instance.Instance, len(ids))
	var err error
	for i, id := range ids {
		if inst := instsById[string(id)]; inst != nil {
			insts[i] = inst
		} else {
			err = environs.ErrPartialInstances
		}
	}
	return insts, err
}

func (e *Environ) AllInstances() (insts []instance.Instance, err error) {
	servers, err := e.nova().ListServersDetail(e.machinesFilter())
	if err != nil {
		return nil, err
	}
	instsById := make(map[string]instance.Instance)
	cfg := e.Config()
	eUUID, ok := cfg.UUID()
	if !ok {
		return nil, errors.NotFoundf("model UUID")
	}
	for _, server := range servers {
		modelUUID, ok := server.Metadata[tags.JujuModel]
		if !ok || modelUUID != eUUID {
			continue
		}
		if e.isAliveServer(server) {
			var s = server
			// TODO(wallyworld): lookup the flavor details to fill in the instance type data
			instsById[s.Id] = &openstackInstance{e: e, serverDetail: &s}
		}
	}

	if e.ecfg().useFloatingIP() {
		if err := e.updateFloatingIPAddresses(instsById); err != nil {
			return nil, err
		}
	}

	for _, inst := range instsById {
		insts = append(insts, inst)
	}
	return insts, err
}

func (e *Environ) Destroy() error {
	err := common.Destroy(e)
	if err != nil {
		return errors.Trace(err)
	}
	return e.firewaller.DeleteGlobalGroups()
}

func resourceName(tag names.Tag, envName string) string {
	return fmt.Sprintf("juju-%s-%s", envName, tag)
}

// machinesFilter returns a nova.Filter matching all machines in the environment.
func (e *Environ) machinesFilter() *nova.Filter {
	filter := nova.NewFilter()
	eUUID, _ := e.Config().UUID()
	filter.Set(nova.FilterServer, fmt.Sprintf("juju-%s-machine-\\d*", eUUID))
	return filter
}

// portsToRuleInfo maps port ranges to nova rules
func portsToRuleInfo(groupId string, ports []network.PortRange) []nova.RuleInfo {
	rules := make([]nova.RuleInfo, len(ports))
	for i, portRange := range ports {
		rules[i] = nova.RuleInfo{
			ParentGroupId: groupId,
			FromPort:      portRange.FromPort,
			ToPort:        portRange.ToPort,
			IPProtocol:    portRange.Protocol,
			Cidr:          "0.0.0.0/0",
		}
	}
	return rules
}

func (e *Environ) OpenPorts(ports []network.PortRange) error {
	return e.firewaller.OpenPorts(ports)
}

func (e *Environ) ClosePorts(ports []network.PortRange) error {
	return e.firewaller.ClosePorts(ports)
}

func (e *Environ) Ports() ([]network.PortRange, error) {
	return e.firewaller.Ports()
}

func (e *Environ) Provider() environs.EnvironProvider {
	return providerInstance
}

// deleteSecurityGroups deletes the given security groups. If a security
// group is also used by another environment (see bug #1300755), an attempt
// to delete this group fails. A warning is logged in this case.
func (e *Environ) deleteSecurityGroups(securityGroupNames []string) error {
	novaclient := e.nova()
	allSecurityGroups, err := novaclient.ListSecurityGroups()
	if err != nil {
		return err
	}
	for _, securityGroup := range allSecurityGroups {
		for _, name := range securityGroupNames {
			if securityGroup.Name == name {
				deleteSecurityGroup(novaclient, name, securityGroup.Id)
				break
			}
		}
	}
	return nil
}

func (e *Environ) terminateInstances(ids []instance.Id) error {
	if len(ids) == 0 {
		return nil
	}
	var firstErr error
	novaClient := e.nova()
	for _, id := range ids {
		err := novaClient.DeleteServer(string(id))
		if gooseerrors.IsNotFound(err) {
			err = nil
		}
		if err != nil && firstErr == nil {
			logger.Debugf("error terminating instance %q: %v", id, err)
			firstErr = err
		}
	}
	return firstErr
}

// MetadataLookupParams returns parameters which are used to query simplestreams metadata.
func (e *Environ) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
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
func (e *Environ) Region() (simplestreams.CloudSpec, error) {
	return e.cloudSpec(e.ecfg().region())
}

func (e *Environ) cloudSpec(region string) (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   region,
		Endpoint: e.ecfg().authURL(),
	}, nil
}

// TagInstance implements environs.InstanceTagger.
func (e *Environ) TagInstance(id instance.Id, tags map[string]string) error {
	if err := e.nova().SetServerMetadata(string(id), tags); err != nil {
		return errors.Annotate(err, "setting server metadata")
	}
	return nil
}
