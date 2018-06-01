// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/version"
	"gopkg.in/goose.v2/cinder"
	"gopkg.in/goose.v2/client"
	gooseerrors "gopkg.in/goose.v2/errors"
	"gopkg.in/goose.v2/identity"
	gooselogging "gopkg.in/goose.v2/logging"
	"gopkg.in/goose.v2/neutron"
	"gopkg.in/goose.v2/nova"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.provider.openstack")

type EnvironProvider struct {
	environs.ProviderCredentials
	Configurator      ProviderConfigurator
	FirewallerFactory FirewallerFactory
	FlavorFilter      FlavorFilter

	// NetworkingDecorator, if non-nil, will be used to
	// decorate the default networking implementation.
	// This can be used to override behaviour.
	NetworkingDecorator NetworkingDecorator

	// ClientFromEndpoint returns an Openstack client for the given endpoint.
	ClientFromEndpoint func(endpoint string) client.AuthenticatingClient
}

var (
	_ environs.CloudEnvironProvider = (*EnvironProvider)(nil)
	_ environs.ProviderSchema       = (*EnvironProvider)(nil)
)

var providerInstance *EnvironProvider = &EnvironProvider{
	ProviderCredentials: OpenstackCredentials{},
	Configurator:        &defaultConfigurator{},
	FirewallerFactory:   &firewallerFactory{},
	FlavorFilter:        FlavorFilterFunc(AcceptAllFlavors),
	NetworkingDecorator: nil,
	ClientFromEndpoint:  newGooseClient,
}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Order:    []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the API endpoint url for the cloud",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
		cloud.AuthTypesKey: {
			Singular:    "auth type",
			Plural:      "auth types",
			Type:        []jsonschema.Type{jsonschema.ArrayType},
			UniqueItems: jsonschema.Bool(true),
			Items: &jsonschema.ItemSpec{
				Schemas: []*jsonschema.Schema{{
					Type: []jsonschema.Type{jsonschema.StringType},
					Enum: []interface{}{
						string(cloud.AccessKeyAuthType),
						string(cloud.UserPassAuthType),
					},
				}},
			},
		},
		cloud.RegionsKey: {
			Type:     []jsonschema.Type{jsonschema.ObjectType},
			Singular: "region",
			Plural:   "regions",
			AdditionalProperties: &jsonschema.Schema{
				Type:          []jsonschema.Type{jsonschema.ObjectType},
				Required:      []string{cloud.EndpointKey},
				MaxProperties: jsonschema.Int(1),
				Properties: map[string]*jsonschema.Schema{
					cloud.EndpointKey: &jsonschema.Schema{
						Singular:      "the API endpoint url for the region",
						Type:          []jsonschema.Type{jsonschema.StringType},
						Format:        jsonschema.FormatURI,
						Default:       "",
						PromptDefault: "use cloud api url",
					},
				},
			},
		},
	},
}

var makeServiceURL = client.AuthenticatingClient.MakeServiceURL

// TODO: shortAttempt was kept to a long timeout because Nova needs
// more time than EC2.  Storage delays are handled separately now, and
// perhaps other polling attempts can time out faster.

// shortAttempt is used when polling for short-term events in tests.
var shortAttempt = utils.AttemptStrategy{
	Total: 15 * time.Second,
	Delay: 200 * time.Millisecond,
}

// Version is part of the EnvironProvider interface.
func (EnvironProvider) Version() int {
	return 0
}

func (p EnvironProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	logger.Infof("opening model %q", args.Config.Name())
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	uuid := args.Config.UUID()
	namespace, err := instance.NewNamespace(uuid)
	if err != nil {
		return nil, errors.Annotate(err, "creating instance namespace")
	}

	e := &Environ{
		name:         args.Config.Name(),
		uuid:         uuid,
		cloud:        args.Cloud,
		namespace:    namespace,
		clock:        clock.WallClock,
		configurator: p.Configurator,
		flavorFilter: p.FlavorFilter,
	}
	e.firewaller = p.FirewallerFactory.GetFirewaller(e)

	var networking Networking = &switchingNetworking{env: e}
	if p.NetworkingDecorator != nil {
		var err error
		networking, err = p.NetworkingDecorator.DecorateNetworking(networking)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	e.networking = networking

	if err := e.SetConfig(args.Config); err != nil {
		return nil, err
	}

	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	client, err := authClient(e.cloud, e.ecfgUnlocked)
	if err != nil {
		return nil, errors.Annotate(err, "cannot set config")
	}
	e.clientUnlocked = client
	e.novaUnlocked = nova.New(e.clientUnlocked)
	e.neutronUnlocked = neutron.New(e.clientUnlocked)

	return e, nil
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

// CloudSchema returns the schema for adding new clouds of this type.
func (p EnvironProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p EnvironProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	c := p.ClientFromEndpoint(endpoint)
	if _, err := c.IdentityAuthOptions(); err != nil {
		return errors.Wrap(err, errors.Errorf("No Openstack server running at %s", endpoint))
	}
	return nil
}

// newGooseClient is the default function in EnvironProvider.ClientFromEndpoint.
func newGooseClient(endpoint string) client.AuthenticatingClient {
	return client.NewClient(&identity.Credentials{URL: endpoint}, 0, nil)
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p EnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}

	// Set the default block-storage source.
	attrs := make(map[string]interface{})
	if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
		attrs[config.StorageDefaultBlockSourceKey] = CinderProviderType
	}

	cfg, err := args.Config.Apply(attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cfg, nil
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p EnvironProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, errors.Errorf("region must be specified")
	}
	return &simplestreams.MetadataLookupParams{
		Region: region,
	}, nil
}

func (p EnvironProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

type Environ struct {
	name      string
	uuid      string
	cloud     environs.CloudSpec
	namespace instance.Namespace

	ecfgMutex       sync.Mutex
	ecfgUnlocked    *environConfig
	clientUnlocked  client.AuthenticatingClient
	novaUnlocked    *nova.Client
	neutronUnlocked *neutron.Client
	volumeURL       *url.URL

	// keystoneImageDataSource caches the result of getKeystoneImageSource.
	keystoneImageDataSourceMutex sync.Mutex
	keystoneImageDataSource      simplestreams.DataSource

	// keystoneToolsDataSource caches the result of getKeystoneToolsSource.
	keystoneToolsDataSourceMutex sync.Mutex
	keystoneToolsDataSource      simplestreams.DataSource

	availabilityZonesMutex sync.Mutex
	availabilityZones      []common.AvailabilityZone
	firewaller             Firewaller
	networking             Networking
	configurator           ProviderConfigurator
	flavorFilter           FlavorFilter

	// Clock is defined so it can be replaced for testing
	clock clock.Clock

	publicIPMutex sync.Mutex
}

var _ environs.Environ = (*Environ)(nil)
var _ environs.NetworkingEnviron = (*Environ)(nil)
var _ simplestreams.HasRegion = (*Environ)(nil)
var _ instance.Distributor = (*Environ)(nil)
var _ environs.InstanceTagger = (*Environ)(nil)

type openstackInstance struct {
	e        *Environ
	instType *instances.InstanceType
	arch     *string

	mu           sync.Mutex
	serverDetail *nova.ServerDetail
	// floatingIP is non-nil iff use-floating-ip is true.
	floatingIP *string
}

func (inst *openstackInstance) String() string {
	return string(inst.Id())
}

var _ instance.Instance = (*openstackInstance)(nil)

func (inst *openstackInstance) Refresh(ctx context.ProviderCallContext) error {
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

func (inst *openstackInstance) Status(ctx context.ProviderCallContext) instance.InstanceStatus {
	instStatus := inst.getServerDetail().Status
	jujuStatus := status.Pending
	switch instStatus {
	case nova.StatusActive:
		jujuStatus = status.Running
	case nova.StatusError:
		jujuStatus = status.ProvisioningError
	case nova.StatusBuild, nova.StatusBuildSpawning,
		nova.StatusDeleted, nova.StatusHardReboot,
		nova.StatusPassword, nova.StatusReboot,
		nova.StatusRebuild, nova.StatusRescue,
		nova.StatusResize, nova.StatusShutoff,
		nova.StatusSuspended, nova.StatusVerifyResize:
		jujuStatus = status.Empty
	case nova.StatusUnknown:
		jujuStatus = status.Unknown
	default:
		jujuStatus = status.Empty
	}
	return instance.InstanceStatus{
		Status:  jujuStatus,
		Message: instStatus,
	}
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
func (inst *openstackInstance) getAddresses(ctx context.ProviderCallContext) (map[string][]nova.IPAddress, error) {
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
func (inst *openstackInstance) Addresses(ctx context.ProviderCallContext) ([]network.Address, error) {
	addresses, err := inst.getAddresses(ctx)
	if err != nil {
		return nil, err
	}
	var floatingIP string
	if inst.floatingIP != nil {
		floatingIP = *inst.floatingIP
		logger.Debugf("instance %v has floating IP address: %v", inst.Id(), floatingIP)
	}
	return convertNovaAddresses(floatingIP, addresses), nil
}

// convertNovaAddresses returns nova addresses in generic format
func convertNovaAddresses(publicIP string, addresses map[string][]nova.IPAddress) []network.Address {
	var machineAddresses []network.Address
	if publicIP != "" {
		publicAddr := network.NewScopedAddress(publicIP, network.ScopePublic)
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
			if machineAddr.Type != addrtype {
				logger.Warningf("derived address type %v, nova reports %v", machineAddr.Type, addrtype)
			}
			machineAddresses = append(machineAddresses, machineAddr)
		}
	}
	return machineAddresses
}

func (inst *openstackInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	return inst.e.firewaller.OpenInstancePorts(ctx, inst, machineId, rules)
}

func (inst *openstackInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	return inst.e.firewaller.CloseInstancePorts(ctx, inst, machineId, rules)
}

func (inst *openstackInstance) IngressRules(ctx context.ProviderCallContext, machineId string) ([]network.IngressRule, error) {
	return inst.e.firewaller.InstanceIngressRules(ctx, inst, machineId)
}

func (e *Environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *Environ) client() client.AuthenticatingClient {
	e.ecfgMutex.Lock()
	client := e.clientUnlocked
	e.ecfgMutex.Unlock()
	return client
}

func (e *Environ) nova() *nova.Client {
	e.ecfgMutex.Lock()
	nova := e.novaUnlocked
	e.ecfgMutex.Unlock()
	return nova
}

func (e *Environ) neutron() *neutron.Client {
	e.ecfgMutex.Lock()
	neutron := e.neutronUnlocked
	e.ecfgMutex.Unlock()
	return neutron
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
		[]string{constraints.Mem, constraints.RootDisk, constraints.Cores})
	validator.RegisterUnsupported(unsupportedConstraints)
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
	validator.RegisterVocabulary(constraints.VirtType, []string{"kvm", "lxd"})
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
func (e *Environ) AvailabilityZones(ctx context.ProviderCallContext) ([]common.AvailabilityZone, error) {
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
func (e *Environ) InstanceAvailabilityZoneNames(ctx context.ProviderCallContext, ids []instance.Id) ([]string, error) {
	instances, err := e.Instances(ctx, ids)
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
	zoneName string
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (e *Environ) DeriveAvailabilityZones(ctx context.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	availabilityZone, err := e.deriveAvailabilityZone(ctx, args.Placement, args.VolumeAttachments)
	if err != nil && !errors.IsNotImplemented(err) {
		return nil, errors.Trace(err)
	}
	if availabilityZone != "" {
		return []string{availabilityZone}, nil
	}
	return nil, nil
}

func (e *Environ) parsePlacement(ctx context.ProviderCallContext, placement string) (*openstackPlacement, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, errors.Errorf("unknown placement directive: %v", placement)
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		availabilityZone := value
		err := common.ValidateAvailabilityZone(e, ctx, availabilityZone)
		if err != nil {
			return nil, err
		}
		return &openstackPlacement{zoneName: availabilityZone}, nil
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

// PrecheckInstance is defined on the environs.InstancePrechecker interface.
func (e *Environ) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	if _, err := e.deriveAvailabilityZone(ctx, args.Placement, args.VolumeAttachments); err != nil {
		return errors.Trace(err)
	}
	if !args.Constraints.HasInstanceType() {
		return nil
	}
	// Constraint has an instance-type constraint so let's see if it is valid.
	novaClient := e.nova()
	flavors, err := novaClient.ListFlavorsDetail()
	if err != nil {
		return err
	}
	for _, flavor := range flavors {
		if flavor.Name == *args.Constraints.InstanceType {
			return nil
		}
	}
	return errors.Errorf("invalid Openstack flavour %q specified", *args.Constraints.InstanceType)
}

// PrepareForBootstrap is part of the Environ interface.
func (e *Environ) PrepareForBootstrap(ctx environs.BootstrapContext) error {
	// Verify credentials.
	if err := authenticateClient(e.client()); err != nil {
		return err
	}
	if !e.supportsNeutron() {
		logger.Warningf(`Using deprecated OpenStack APIs.

  Neutron networking is not supported by this OpenStack cloud.
  Falling back to deprecated Nova networking.

  Support for deprecated Nova networking APIs will be removed
  in a future Juju release. Please upgrade to OpenStack Icehouse
  or newer to maintain compatibility.

`,
		)
	}
	return nil
}

// Create is part of the Environ interface.
func (e *Environ) Create(context.ProviderCallContext, environs.CreateParams) error {
	// Verify credentials.
	if err := authenticateClient(e.client()); err != nil {
		return err
	}
	// TODO(axw) 2016-08-04 #1609643
	// Create global security group(s) here.
	return nil
}

func (e *Environ) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	// The client's authentication may have been reset when finding tools if the agent-version
	// attribute was updated so we need to re-authenticate. This will be a no-op if already authenticated.
	// An authenticated client is needed for the URL() call below.
	if err := authenticateClient(e.client()); err != nil {
		return nil, err
	}
	return common.Bootstrap(ctx, e, callCtx, args)
}

func (e *Environ) supportsNeutron() bool {
	client := e.client()
	endpointMap := client.EndpointsForRegion(e.cloud.Region)
	_, ok := endpointMap["network"]
	return ok
}

func (e *Environ) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	// Find all instances tagged with tags.JujuIsController.
	instances, err := e.allControllerManagedInstances(controllerUUID, e.ecfg().useFloatingIP())
	if err != nil {
		return nil, errors.Trace(err)
	}
	ids := make([]instance.Id, 0, 1)
	for _, instance := range instances {
		detail := instance.(*openstackInstance).getServerDetail()
		if detail.Metadata[tags.JujuIsController] == "true" {
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

func newCredentials(spec environs.CloudSpec) (identity.Credentials, identity.AuthMode) {
	credAttrs := spec.Credential.Attributes()
	cred := identity.Credentials{
		Region:     spec.Region,
		URL:        spec.Endpoint,
		TenantName: credAttrs[CredAttrTenantName],
	}

	// AuthType is validated when the environment is opened, so it's known
	// to be one of these values.
	var authMode identity.AuthMode
	switch spec.Credential.AuthType() {
	case cloud.UserPassAuthType:
		// TODO(axw) we need a way of saying to use legacy auth.
		cred.User = credAttrs[CredAttrUserName]
		cred.Secrets = credAttrs[CredAttrPassword]
		cred.ProjectDomain = credAttrs[CredAttrProjectDomainName]
		cred.UserDomain = credAttrs[CredAttrUserDomainName]
		cred.Domain = credAttrs[CredAttrDomainName]
		authMode = identity.AuthUserPass
		if cred.Domain != "" || cred.UserDomain != "" || cred.ProjectDomain != "" {
			authMode = identity.AuthUserPassV3
		}
	case cloud.AccessKeyAuthType:
		cred.User = credAttrs[CredAttrAccessKey]
		cred.Secrets = credAttrs[CredAttrSecretKey]
		authMode = identity.AuthKeyPair
	}
	return cred, authMode
}

func determineBestClient(
	options identity.AuthOptions,
	client client.AuthenticatingClient,
	cred identity.Credentials,
	newClient func(*identity.Credentials, identity.AuthMode, gooselogging.CompatLogger) client.AuthenticatingClient,
	logger gooselogging.CompatLogger,
) client.AuthenticatingClient {
	for _, option := range options {
		if option.Mode != identity.AuthUserPassV3 {
			continue
		}
		cred.URL = option.Endpoint
		v3client := newClient(&cred, identity.AuthUserPassV3, logger)
		// V3 being advertised is not necessaritly a guarantee that it will
		// work.
		err := v3client.Authenticate()
		if err == nil {
			return v3client
		}
	}
	return client
}

func authClient(spec environs.CloudSpec, ecfg *environConfig) (client.AuthenticatingClient, error) {
	identityClientVersion, err := identityClientVersion(spec.Endpoint)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create a client")
	}
	cred, authMode := newCredentials(spec)

	gooseLogger := gooselogging.LoggoLogger{loggo.GetLogger("goose")}
	newClient := client.NewClient
	if ecfg.SSLHostnameVerification() == false {
		newClient = client.NewNonValidatingClient
	}
	client := newClient(&cred, authMode, gooseLogger)

	// before returning, lets make sure that we want to have AuthMode
	// AuthUserPass instead of its V3 counterpart.
	if authMode == identity.AuthUserPass && (identityClientVersion == -1 || identityClientVersion == 3) {
		options, err := client.IdentityAuthOptions()
		if err != nil {
			logger.Errorf("cannot determine available auth versions %v", err)
		} else {
			client = determineBestClient(
				options,
				client,
				cred,
				newClient,
				gooseLogger,
			)
		}
	}

	// Juju requires "compute" at a minimum. We'll use "network" if it's
	// available in preference to the Nova network APIs; and "volume" or
	// "volume2" for storage if either one is available.
	client.SetRequiredServiceTypes([]string{"compute"})
	return client, nil
}

type authenticator interface {
	Authenticate() error
}

var authenticateClient = func(auth authenticator) error {
	err := auth.Authenticate()
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

	return nil
}

func identityClientVersion(authURL string) (int, error) {
	url, err := url.Parse(authURL)
	if err != nil {
		// Return 0 as this is the lowest invalid number according to openstack codebase:
		// -1 is reserved and has special handling; 1, 2, 3, etc are valid identity client versions.
		return 0, err
	}
	if url.Path == authURL {
		// This means we could not parse URL into url structure
		// with protocols, domain, port, etc.
		// For example, specifying "keystone.foo" instead of "https://keystone.foo:443/v3/"
		// falls into this category.
		return 0, errors.Errorf("url %s is malformed", authURL)
	}
	if url.Path == "" || url.Path == "/" {
		// User explicitly did not provide any version, it is empty.
		return -1, nil
	}
	// The last part of the path should be the version #, prefixed with a 'v' or 'V'
	// Example: https://keystone.foo:443/v3/
	// Example: https://sharedhost.foo:443/identity/v3/
	urlpath := strings.ToLower(url.Path)
	urlpath, tail := path.Split(urlpath)
	if len(tail) == 0 && len(urlpath) > 2 {
		// trailing /, remove it and split again
		urlpath, tail = path.Split(strings.TrimRight(urlpath, "/"))
	}
	versionNumStr := strings.TrimPrefix(tail, "v")
	logger.Tracef("authURL: %s", authURL)
	major, _, err := version.ParseMajorMinor(versionNumStr)
	if len(tail) < 2 || tail[0] != 'v' || err != nil {
		// There must be a '/v' in the URL path.
		// At this stage only '/Vxxx' and '/vxxx' are valid where xxx is major.minor version.
		// Return 0 as this is the lowest invalid number according to openstack codebase:
		// -1 is reserved and has special handling; 1, 2, 3, etc are valid identity client versions.
		return 0, errors.NotValidf("version part of identity url %s", authURL)
	}
	return major, err
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

	client := e.client()
	if !client.IsAuthenticated() {
		if err := authenticateClient(client); err != nil {
			return nil, err
		}
	}

	url, err := makeServiceURL(client, keystoneName, "", nil)
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

// assignPublicIP tries to assign the given floating IP address to the
// specified server, or returns an error.
func (e *Environ) assignPublicIP(fip *string, serverId string) (err error) {
	if *fip == "" {
		return errors.Errorf("cannot assign a nil public IP to %q", serverId)
	}
	// At startup nw_info is not yet cached so this may fail
	// temporarily while the server is being built
	for a := common.LongAttempt.Start(); a.Next(); {
		err = e.nova().AddServerFloatingIP(serverId, *fip)
		if err == nil {
			return nil
		}
	}
	return err
}

// DistributeInstances implements the state.InstanceDistributor policy.
func (e *Environ) DistributeInstances(ctx context.ProviderCallContext, candidates, distributionGroup []instance.Id) ([]instance.Id, error) {
	return common.DistributeInstances(e, ctx, candidates, distributionGroup)
}

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

// MaintainInstance is specified in the InstanceBroker interface.
func (*Environ) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (e *Environ) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (_ *environs.StartInstanceResult, err error) {
	if args.AvailabilityZone != "" {
		// args.AvailabilityZone should only be set if this OpenStack
		// supports zones; validate the zone.
		volumeAttachmentsZone, err := e.volumeAttachmentsZone(args.VolumeAttachments)
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
		if err := validateAvailabilityZoneConsistency(args.AvailabilityZone, volumeAttachmentsZone); err != nil {
			return nil, common.ZoneIndependentError(err)
		}
		if err := common.ValidateAvailabilityZone(e, ctx, args.AvailabilityZone); err != nil {
			return nil, errors.Trace(err)
		}
	}

	series := args.Tools.OneSeries()
	arches := args.Tools.Arches()
	spec, err := findInstanceSpec(e, &instances.InstanceConstraint{
		Region:      e.cloud.Region,
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
	}, args.ImageMetadata)
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}
	tools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, common.ZoneIndependentError(
			errors.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches),
		)
	}

	if err := args.InstanceConfig.SetTools(tools); err != nil {
		return nil, common.ZoneIndependentError(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, e.Config()); err != nil {
		return nil, common.ZoneIndependentError(err)
	}
	cloudcfg, err := e.configurator.GetCloudConfig(args)
	if err != nil {
		return nil, common.ZoneIndependentError(err)
	}
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, OpenstackRenderer{})
	if err != nil {
		return nil, common.ZoneIndependentError(errors.Annotate(err, "cannot make user data"))
	}
	logger.Debugf("openstack user data; %d bytes", len(userData))

	networks, err := e.networking.DefaultNetworks()
	if err != nil {
		return nil, common.ZoneIndependentError(errors.Annotate(err, "getting initial networks"))
	}
	usingNetwork := e.ecfg().network()
	if usingNetwork != "" {
		networkId, err := e.networking.ResolveNetwork(usingNetwork, false)
		if err != nil {
			return nil, common.ZoneIndependentError(err)
		}
		logger.Debugf("using network id %q", networkId)
		networks = append(networks, nova.ServerNetworks{NetworkId: networkId})
	}

	machineName := resourceName(
		e.namespace,
		e.name,
		args.InstanceConfig.MachineId,
	)

	if e.ecfg().useOpenstackGBP() {
		client := e.neutron()
		ptArg := neutron.PolicyTargetV2{
			Name:                fmt.Sprintf("juju-policytarget-%s", machineName),
			PolicyTargetGroupId: e.ecfg().policyTargetGroup(),
		}
		pt, err := client.CreatePolicyTargetV2(ptArg)
		if err != nil {
			return nil, errors.Trace(err)
		}
		networks = append(networks, nova.ServerNetworks{PortId: pt.PortId})
	}

	// For BUG 1680787: openstack: add support for neutron networks where port
	// security is disabled.
	// If any network specified for instance boot has PortSecurityEnabled equals
	// false, don't create security groups, instance boot will fail.
	createSecurityGroups := true
	if len(networks) > 0 && e.supportsNeutron() {
		client := e.neutron()
		for _, n := range networks {
			if n.NetworkId == "" {
				// It's a GBP network.
				continue
			}
			net, err := client.GetNetworkV2(n.NetworkId)
			if err != nil {
				return nil, common.ZoneIndependentError(err)
			}
			if net.PortSecurityEnabled != nil &&
				*net.PortSecurityEnabled == false {
				createSecurityGroups = *net.PortSecurityEnabled
				logger.Infof("network %q has port_security_enabled set to false. Not using security groups.", net.Id)
				break
			}
		}
	}

	var novaGroupNames = []nova.SecurityGroupName{}
	if createSecurityGroups {
		var apiPort int
		if args.InstanceConfig.Controller != nil {
			apiPort = args.InstanceConfig.Controller.Config.APIPort()
		} else {
			// All ports are the same so pick the first.
			apiPort = args.InstanceConfig.APIInfo.Ports()[0]
		}
		groupNames, err := e.firewaller.SetUpGroups(ctx, args.ControllerUUID, args.InstanceConfig.MachineId, apiPort)
		if err != nil {
			return nil, common.ZoneIndependentError(errors.Annotate(err, "cannot set up groups"))
		}
		novaGroupNames = make([]nova.SecurityGroupName, len(groupNames))
		for i, name := range groupNames {
			novaGroupNames[i].Name = name
		}
	}

	waitForActiveServerDetails := func(
		client *nova.Client,
		id string,
		timeout time.Duration,
	) (server *nova.ServerDetail, err error) {

		var errStillBuilding = errors.Errorf("instance %q has status BUILD", id)
		err = retry.Call(retry.CallArgs{
			Clock:       e.clock,
			Delay:       10 * time.Second,
			MaxDuration: timeout,
			Func: func() error {
				server, err = client.GetServer(id)
				if err != nil {
					return err
				}
				if server.Status == nova.StatusBuild {
					return errStillBuilding
				}
				return nil
			},
			NotifyFunc: func(lastError error, attempt int) {
				args.StatusCallback(status.Provisioning, fmt.Sprintf("%s, wait 10 seconds before retry, attempt %d", lastError, attempt), nil)
			},
			IsFatalError: func(err error) bool {
				return err != errStillBuilding
			},
		})
		if err != nil {
			return nil, err
		}
		return server, nil
	}

	tryStartNovaInstance := func(
		attempts utils.AttemptStrategy,
		client *nova.Client,
		instanceOpts nova.RunServerOpts,
	) (server *nova.Entity, err error) {
		for a := attempts.Start(); a.Next(); {
			server, err = client.RunServer(instanceOpts)
			if err != nil {
				break
			}
			var serverDetail *nova.ServerDetail
			serverDetail, err = waitForActiveServerDetails(client, server.Id, 5*time.Minute)
			if err != nil {
				server = nil
				break
			} else if serverDetail.Status == nova.StatusActive {
				break
			} else if serverDetail.Status == nova.StatusError {
				// Perhaps there is an error case where a retry in the same AZ
				// is a good idea.
				logger.Infof("Instance %q in ERROR state with fault %q", server.Id, serverDetail.Fault.Message)
				logger.Infof("Deleting instance %q in ERROR state", server.Id)
				if err = e.terminateInstances([]instance.Id{instance.Id(server.Id)}); err != nil {
					logger.Debugf("Failed to delete instance in ERROR state, %q", err)
				}
				server = nil
				err = errors.New(serverDetail.Fault.Message)
				break
			}
		}
		return server, err
	}

	var opts = nova.RunServerOpts{
		Name:               machineName,
		FlavorId:           spec.InstanceType.Id,
		ImageId:            spec.Image.Id,
		UserData:           userData,
		SecurityGroupNames: novaGroupNames,
		Networks:           networks,
		Metadata:           args.InstanceConfig.Tags,
		AvailabilityZone:   args.AvailabilityZone,
	}
	e.configurator.ModifyRunServerOptions(&opts)

	server, err := tryStartNovaInstance(shortAttempt, e.nova(), opts)
	if err != nil {
		// 'No valid host available' is typically a resource error,
		// let the provisioner know it is a good idea to try another
		// AZ if available.
		err := errors.Annotate(err, "cannot run instance")
		zoneSpecific := isNoValidHostsError(err)
		if !zoneSpecific {
			err = common.ZoneIndependentError(err)
		}
		return nil, err
	}

	detail, err := e.nova().GetServer(server.Id)
	if err != nil {
		return nil, common.ZoneIndependentError(errors.Annotate(err, "cannot get started instance"))
	}

	inst := &openstackInstance{
		e:            e,
		serverDetail: detail,
		arch:         &spec.Image.Arch,
		instType:     &spec.InstanceType,
	}
	logger.Infof("started instance %q", inst.Id())
	withPublicIP := e.ecfg().useFloatingIP()
	if withPublicIP {
		// If we don't lock here, AllocatePublicIP() can return the same
		// public IP for 2 different instances.  Only one will successfully
		// be assigned the public IP, the other will not have one.
		e.publicIPMutex.Lock()
		defer e.publicIPMutex.Unlock()
		var publicIP *string
		logger.Debugf("allocating public IP address for openstack node")
		if fip, err := e.networking.AllocatePublicIP(inst.Id()); err != nil {
			return nil, common.ZoneIndependentError(errors.Annotate(err, "cannot allocate a public IP as needed"))
		} else {
			publicIP = fip
			logger.Infof("allocated public IP %s", *publicIP)
		}
		if err := e.assignPublicIP(publicIP, string(inst.Id())); err != nil {
			if err := e.terminateInstances([]instance.Id{inst.Id()}); err != nil {
				// ignore the failure at this stage, just log it
				logger.Debugf("failed to terminate instance %q: %v", inst.Id(), err)
			}
			return nil, common.ZoneIndependentError(errors.Annotatef(err,
				"cannot assign public address %s to instance %q",
				publicIP, inst.Id(),
			))
		}
		inst.floatingIP = publicIP
	}

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: inst.hardwareCharacteristics(),
	}, nil
}

func (e *Environ) deriveAvailabilityZone(
	ctx context.ProviderCallContext,
	placement string,
	volumeAttachments []storage.VolumeAttachmentParams,
) (string, error) {
	volumeAttachmentsZone, err := e.volumeAttachmentsZone(volumeAttachments)
	if err != nil {
		return "", errors.Trace(err)
	}
	if placement == "" {
		return volumeAttachmentsZone, nil
	}
	instPlacement, err := e.parsePlacement(ctx, placement)
	if err != nil {
		return "", err
	}
	if err := validateAvailabilityZoneConsistency(instPlacement.zoneName, volumeAttachmentsZone); err != nil {
		return "", errors.Annotatef(err, "cannot create instance with placement %q", placement)
	}
	return instPlacement.zoneName, nil
}

func validateAvailabilityZoneConsistency(instanceZone, volumeAttachmentsZone string) error {
	if volumeAttachmentsZone != "" && instanceZone != volumeAttachmentsZone {
		return errors.Errorf(
			"cannot create instance in zone %q, as this will prevent attaching the requested disks in zone %q",
			instanceZone, volumeAttachmentsZone,
		)
	}
	return nil
}

// volumeAttachmentsZone determines the availability zone for each volume
// identified in the volume attachment parameters, checking that they are
// all the same, and returns the availability zone name.
func (e *Environ) volumeAttachmentsZone(volumeAttachments []storage.VolumeAttachmentParams) (string, error) {
	if len(volumeAttachments) == 0 {
		return "", nil
	}
	cinderProvider, err := e.cinderProvider()
	if err != nil {
		return "", errors.Trace(err)
	}
	volumes, err := modelCinderVolumes(cinderProvider.storageAdapter, cinderProvider.modelUUID)
	if err != nil {
		return "", errors.Trace(err)
	}
	var zone string
	for i, a := range volumeAttachments {
		var v *cinder.Volume
		for i := range volumes {
			if volumes[i].ID == a.VolumeId {
				v = &volumes[i]
				break
			}
		}
		if v == nil {
			return "", errors.Errorf("cannot find volume %q to attach to new instance", a.VolumeId)
		}
		if zone == "" {
			zone = v.AvailabilityZone
		} else if v.AvailabilityZone != zone {
			return "", errors.Errorf(
				"cannot attach volumes from multiple availability zones: %s is in %s, %s is in %s",
				volumeAttachments[i-1].VolumeId, zone, a.VolumeId, v.AvailabilityZone,
			)
		}
	}
	return zone, nil
}

func isNoValidHostsError(err error) bool {
	if cause := errors.Cause(err); cause != nil {
		return strings.Contains(cause.Error(), "No valid host was found")
	}
	return false
}

func (e *Environ) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	// If in instance firewall mode, gather the security group names.
	securityGroupNames, err := e.firewaller.GetSecurityGroups(ctx, ids...)
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
		return e.firewaller.DeleteGroups(ctx, securityGroupNames...)
	}
	return nil
}

func (e *Environ) isAliveServer(server nova.ServerDetail) bool {
	switch server.Status {
	case nova.StatusActive, nova.StatusBuild, nova.StatusBuildSpawning, nova.StatusShutoff, nova.StatusSuspended:
		return true
	}
	return false
}

func (e *Environ) listServers(ctx context.ProviderCallContext, ids []instance.Id) ([]nova.ServerDetail, error) {
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
	// List all instances in the environment.
	instances, err := e.AllInstances(ctx)
	if err != nil {
		return nil, err
	}
	// Return only servers with the wanted ids that are currently alive
	for _, inst := range instances {
		inst := inst.(*openstackInstance)
		serverDetail := *inst.serverDetail
		if !e.isAliveServer(serverDetail) {
			continue
		}
		for _, id := range ids {
			if inst.Id() != id {
				continue
			}
			wantedServers = append(wantedServers, serverDetail)
			break
		}
	}
	return wantedServers, nil
}

// updateFloatingIPAddresses updates the instances with any floating IP address
// that have been assigned to those instances.
func (e *Environ) updateFloatingIPAddresses(instances map[string]instance.Instance) error {
	servers, err := e.nova().ListServersDetail(jujuMachineFilter())
	if err != nil {
		return err
	}
	for _, server := range servers {
		// server.Addresses is a map with entries containing []nova.IPAddress
		for _, net := range server.Addresses {
			for _, addr := range net {
				if addr.Type == "floating" {
					instId := server.Id
					if inst, ok := instances[instId]; ok {
						instFip := &addr.Address
						inst.(*openstackInstance).floatingIP = instFip
					}
				}
			}
		}
	}
	return nil
}

func (e *Environ) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Make a series of requests to cope with eventual consistency.
	// Each request will attempt to add more instances to the requested
	// set.
	var foundServers []nova.ServerDetail
	for a := shortAttempt.Start(); a.Next(); {
		var err error
		foundServers, err = e.listServers(ctx, ids)
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

// AdoptResources is part of the Environ interface.
func (e *Environ) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	var failed []string
	controllerTag := map[string]string{tags.JujuController: controllerUUID}

	instances, err := e.AllInstances(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, instance := range instances {
		err := e.TagInstance(ctx, instance.Id(), controllerTag)
		if err != nil {
			logger.Errorf("error updating controller tag for instance %s: %v", instance.Id(), err)
			failed = append(failed, string(instance.Id()))
		}
	}

	failedVolumes, err := e.adoptVolumes(controllerTag)
	if err != nil {
		return errors.Trace(err)
	}
	failed = append(failed, failedVolumes...)

	err = e.firewaller.UpdateGroupController(ctx, controllerUUID)
	if err != nil {
		return errors.Trace(err)
	}
	if len(failed) != 0 {
		return errors.Errorf("error updating controller tag for some resources: %v", failed)
	}
	return nil
}

func (e *Environ) adoptVolumes(controllerTag map[string]string) ([]string, error) {
	cinder, err := e.cinderProvider()
	if errors.IsNotSupported(err) {
		logger.Debugf("volumes not supported: not transferring ownership for volumes")
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(axw): fix the storage API.
	storageConfig, err := storage.NewConfig("cinder", CinderProviderType, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeSource, err := cinder.VolumeSource(storageConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeIds, err := volumeSource.ListVolumes()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var failed []string
	for _, volumeId := range volumeIds {
		_, err := cinder.storageAdapter.SetVolumeMetadata(volumeId, controllerTag)
		if err != nil {
			logger.Errorf("error updating controller tag for volume %s: %v", volumeId, err)
			failed = append(failed, volumeId)
		}
	}
	return failed, nil
}

// AllInstances returns all instances in this environment.
func (e *Environ) AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error) {
	tagFilter := tagValue{tags.JujuModel, e.ecfg().UUID()}
	return e.allInstances(tagFilter, e.ecfg().useFloatingIP())
}

// allControllerManagedInstances returns all instances managed by this
// environment's controller, matching the optionally specified filter.
func (e *Environ) allControllerManagedInstances(controllerUUID string, updateFloatingIPAddresses bool) ([]instance.Instance, error) {
	tagFilter := tagValue{tags.JujuController, controllerUUID}
	return e.allInstances(tagFilter, updateFloatingIPAddresses)
}

type tagValue struct {
	tag, value string
}

// allControllerManagedInstances returns all instances managed by this
// environment's controller, matching the optionally specified filter.
func (e *Environ) allInstances(tagFilter tagValue, updateFloatingIPAddresses bool) ([]instance.Instance, error) {
	servers, err := e.nova().ListServersDetail(jujuMachineFilter())
	if err != nil {
		return nil, err
	}
	instsById := make(map[string]instance.Instance)
	for _, server := range servers {
		if server.Metadata[tagFilter.tag] != tagFilter.value {
			continue
		}
		if e.isAliveServer(server) {
			var s = server
			// TODO(wallyworld): lookup the flavor details to fill in the instance type data
			instsById[s.Id] = &openstackInstance{e: e, serverDetail: &s}
		}
	}
	if updateFloatingIPAddresses {
		if err := e.updateFloatingIPAddresses(instsById); err != nil {
			return nil, err
		}
	}
	insts := make([]instance.Instance, 0, len(instsById))
	for _, inst := range instsById {
		insts = append(insts, inst)
	}
	return insts, nil
}

func (e *Environ) Destroy(ctx context.ProviderCallContext) error {
	err := common.Destroy(e, ctx)
	if err != nil {
		return errors.Trace(err)
	}
	// Delete all security groups remaining in the model.
	return e.firewaller.DeleteAllModelGroups(ctx)
}

// DestroyController implements the Environ interface.
func (e *Environ) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	if err := e.Destroy(ctx); err != nil {
		return errors.Annotate(err, "destroying controller model")
	}
	// In case any hosted environment hasn't been cleaned up yet,
	// we also attempt to delete their resources when the controller
	// environment is destroyed.
	if err := e.destroyControllerManagedEnvirons(controllerUUID); err != nil {
		return errors.Annotate(err, "destroying managed models")
	}
	return e.firewaller.DeleteAllControllerGroups(ctx, controllerUUID)
}

// destroyControllerManagedEnvirons destroys all environments managed by this
// models's controller.
func (e *Environ) destroyControllerManagedEnvirons(controllerUUID string) error {
	// Terminate all instances managed by the controller.
	insts, err := e.allControllerManagedInstances(controllerUUID, false)
	if err != nil {
		return errors.Annotate(err, "listing instances")
	}
	instIds := make([]instance.Id, len(insts))
	for i, inst := range insts {
		instIds[i] = inst.Id()
	}
	if err := e.terminateInstances(instIds); err != nil {
		return errors.Annotate(err, "terminating instances")
	}

	// Delete all volumes managed by the controller.
	cinder, err := e.cinderProvider()
	if err == nil {
		volumes, err := controllerCinderVolumes(cinder.storageAdapter, controllerUUID)
		if err != nil {
			return errors.Annotate(err, "listing volumes")
		}
		volIds := volumeInfoToVolumeIds(cinderToJujuVolumeInfos(volumes))
		errs := foreachVolume(cinder.storageAdapter, volIds, destroyVolume)
		for i, err := range errs {
			if err == nil {
				continue
			}
			return errors.Annotatef(err, "destroying volume %q", volIds[i], err)
		}
	} else if !errors.IsNotSupported(err) {
		return errors.Trace(err)
	}

	// Security groups for hosted models are destroyed by the
	// DeleteAllControllerGroups method call from Destroy().
	return nil
}

func resourceName(namespace instance.Namespace, envName, resourceId string) string {
	return namespace.Value(envName + "-" + resourceId)
}

// jujuMachineFilter returns a nova.Filter matching machines created by Juju.
// The machines are not filtered to any particular environment. To do that,
// instance tags must be compared.
func jujuMachineFilter() *nova.Filter {
	filter := nova.NewFilter()
	filter.Set(nova.FilterServer, "juju-.*")
	return filter
}

// rulesToRuleInfo maps ingress rules to nova rules
func rulesToRuleInfo(groupId string, rules []network.IngressRule) []neutron.RuleInfoV2 {
	var result []neutron.RuleInfoV2
	for _, r := range rules {
		ruleInfo := neutron.RuleInfoV2{
			Direction:     "ingress",
			ParentGroupId: groupId,
			PortRangeMin:  r.FromPort,
			PortRangeMax:  r.ToPort,
			IPProtocol:    r.Protocol,
		}
		sourceCIDRs := r.SourceCIDRs
		if len(sourceCIDRs) == 0 {
			sourceCIDRs = []string{"0.0.0.0/0"}
		}
		for _, sr := range sourceCIDRs {
			ruleInfo.RemoteIPPrefix = sr
			result = append(result, ruleInfo)
		}
	}
	return result
}

func (e *Environ) OpenPorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return e.firewaller.OpenPorts(ctx, rules)
}

func (e *Environ) ClosePorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return e.firewaller.ClosePorts(ctx, rules)
}

func (e *Environ) IngressRules(ctx context.ProviderCallContext) ([]network.IngressRule, error) {
	return e.firewaller.IngressRules(ctx)
}

func (e *Environ) Provider() environs.EnvironProvider {
	return providerInstance
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
		region = e.cloud.Region
	}
	cloudSpec, err := e.cloudSpec(region)
	if err != nil {
		return nil, err
	}
	return &simplestreams.MetadataLookupParams{
		Series:   config.PreferredSeries(e.ecfg()),
		Region:   cloudSpec.Region,
		Endpoint: cloudSpec.Endpoint,
	}, nil
}

// Region is specified in the HasRegion interface.
func (e *Environ) Region() (simplestreams.CloudSpec, error) {
	return e.cloudSpec(e.cloud.Region)
}

func (e *Environ) cloudSpec(region string) (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   region,
		Endpoint: e.cloud.Endpoint,
	}, nil
}

// TagInstance implements environs.InstanceTagger.
func (e *Environ) TagInstance(ctx context.ProviderCallContext, id instance.Id, tags map[string]string) error {
	if err := e.nova().SetServerMetadata(string(id), tags); err != nil {
		return errors.Annotate(err, "setting server metadata")
	}
	return nil
}

func (e *Environ) SetClock(clock clock.Clock) {
	e.clock = clock
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := validateAuthURL(spec.Endpoint); err != nil {
		return errors.Annotate(err, "validating auth-url")
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	switch authType := spec.Credential.AuthType(); authType {
	case cloud.UserPassAuthType:
	case cloud.AccessKeyAuthType:
	default:
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}

func validateAuthURL(authURL string) error {
	parts, err := url.Parse(authURL)
	if err != nil || parts.Host == "" || parts.Scheme == "" {
		return errors.NotValidf("auth-url %q", authURL)
	}
	return nil
}

// Subnets is specified on environs.Networking.
func (e *Environ) Subnets(ctx context.ProviderCallContext, instId instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	return e.networking.Subnets(instId, subnetIds)
}

// NetworkInterfaces is specified on environs.Networking.
func (e *Environ) NetworkInterfaces(ctx context.ProviderCallContext, instId instance.Id) ([]network.InterfaceInfo, error) {
	return e.networking.NetworkInterfaces(instId)
}

// SupportsSpaces is specified on environs.Networking.
func (e *Environ) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	return false, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (e *Environ) SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error) {
	return false, nil
}

// Spaces is specified on environs.Networking.
func (e *Environ) Spaces(ctx context.ProviderCallContext) ([]network.SpaceInfo, error) {
	return nil, errors.NotSupportedf("spaces")
}

// SupportsContainerAddresses is specified on environs.Networking.
func (e *Environ) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	return false, errors.NotSupportedf("container address")
}

// SuperSubnets is specified on environs.Networking
func (e *Environ) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	subnets, err := e.networking.Subnets("", nil)
	if err != nil {
		return nil, err
	}
	cidrs := make([]string, len(subnets))
	for i, subnet := range subnets {
		cidrs[i] = subnet.CIDR
	}
	return cidrs, nil
}

// AllocateContainerAddresses is specified on environs.Networking.
func (e *Environ) AllocateContainerAddresses(ctx context.ProviderCallContext, hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("allocate container address")
}

// ReleaseContainerAddresses is specified on environs.Networking.
func (e *Environ) ReleaseContainerAddresses(ctx context.ProviderCallContext, interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("release container address")
}

// ProviderSpaceInfo is specified on environs.NetworkingEnviron.
func (*Environ) ProviderSpaceInfo(ctx context.ProviderCallContext, space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable is specified on environs.NetworkingEnviron.
func (*Environ) AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses is specified on environs.SSHAddresses.
func (*Environ) SSHAddresses(ctx context.ProviderCallContext, addresses []network.Address) ([]network.Address, error) {
	return addresses, nil
}
