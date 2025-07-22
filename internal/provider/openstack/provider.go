// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/rand"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-goose/goose/v5/cinder"
	"github.com/go-goose/goose/v5/client"
	gooseerrors "github.com/go-goose/goose/v5/errors"
	"github.com/go-goose/goose/v5/identity"
	"github.com/go-goose/goose/v5/neutron"
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/retry"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/interact"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/http"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/storage"
)

var logger = internallogger.GetLogger("juju.provider.openstack")

type EnvironProvider struct {
	environs.ProviderCredentials
	Configurator      ProviderConfigurator
	FirewallerFactory FirewallerFactory
	FlavorFilter      FlavorFilter

	// ClientFromEndpoint returns an Openstack client for the given endpoint.
	ClientFromEndpoint func(endpoint string) client.AuthenticatingClient
}

var (
	_ environs.CloudEnvironProvider = (*EnvironProvider)(nil)
	_ environs.ProviderSchema       = (*EnvironProvider)(nil)
)

var providerInstance = &EnvironProvider{
	ProviderCredentials: OpenstackCredentials{},
	Configurator:        &defaultConfigurator{},
	FirewallerFactory:   &firewallerFactory{},
	FlavorFilter:        FlavorFilterFunc(AcceptAllFlavors),
	ClientFromEndpoint:  newGooseClient,
}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Order:    []string{cloud.EndpointKey, cloud.CertFilenameKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the API endpoint url for the cloud",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
			Default:  "",
			EnvVars:  []string{"OS_AUTH_URL"},
		},
		cloud.CertFilenameKey: {
			Singular:      "a path to the CA certificate for your cloud if one is required to access it. (optional)",
			Type:          []jsonschema.Type{jsonschema.StringType},
			Format:        interact.FormatCertFilename,
			Default:       "",
			PromptDefault: "none",
			EnvVars:       []string{"OS_CACERT"},
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
			Default:  "",
			EnvVars:  []string{"OS_REGION_NAME"},
			AdditionalProperties: &jsonschema.Schema{
				Type:          []jsonschema.Type{jsonschema.ObjectType},
				Required:      []string{cloud.EndpointKey},
				MaxProperties: jsonschema.Int(1),
				Properties: map[string]*jsonschema.Schema{
					cloud.EndpointKey: {
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

const (
	// provider version 1 introduces tags to security groups.
	providerVersion1 = 1

	currentProviderVersion = providerVersion1
)

// Version is part of the EnvironProvider interface.
func (EnvironProvider) Version() int {
	return currentProviderVersion
}

func (p EnvironProvider) Open(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
	logger.Infof(ctx, "opening model %q", args.Config.Name())
	uuid := args.Config.UUID()
	namespace, err := instance.NewNamespace(uuid)
	if err != nil {
		return nil, errors.Annotate(err, "creating instance namespace")
	}

	e := &Environ{
		CredentialInvalidator: common.NewCredentialInvalidator(invalidator, IsAuthorisationFailure),
		name:                  args.Config.Name(),
		modelUUID:             uuid,
		namespace:             namespace,
		clock:                 clock.WallClock,
		configurator:          p.Configurator,
		flavorFilter:          p.FlavorFilter,
		controllerUUID:        args.ControllerUUID,
	}

	if err := e.SetConfig(ctx, args.Config); err != nil {
		return nil, errors.Trace(err)
	}
	if err := e.SetCloudSpec(ctx, args.Cloud); err != nil {
		return nil, errors.Trace(err)
	}

	e.networking, e.firewaller, err = p.getEnvironNetworkingFirewaller(ctx, e)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return e, nil
}

// getEnvironNetworkingFirewaller returns Networking and Firewaller for the
// new Environ.  Both require Neutron to be support by the OpenStack cloud,
// so create together.
func (p EnvironProvider) getEnvironNetworkingFirewaller(ctx context.Context, e *Environ) (Networking, Firewaller, error) {
	// TODO (hml) 2019-12-05
	// We want to ensure a failure if an old nova networking OpenStack is
	// added as a new model to a multi-cloud controller.  However the
	// current OpenStack testservice does not implement EndpointsForRegions(),
	// thus causing failures and panics in the setup of the majority of
	// provider unit tests.  Or a rewrite of code and/or tests.
	// See LP:1855343
	if err := authenticateClient(ctx, e.client()); err != nil {
		return nil, nil, errors.Trace(err)
	}
	if !e.supportsNeutron() {
		// This should turn into a failure, left as an Error message for now to help
		// provide context for failing networking calls by this environ.  Previously
		// this was covered by switchingNetworking{} and switchingFirewaller{}.
		logger.Errorf(ctx, "Using unsupported OpenStack APIs. Neutron networking "+
			"is not supported by this OpenStack cloud.\n  Please use OpenStack Queens or "+
			"newer to maintain compatibility.")
	}
	networking := newNetworking(e)
	return networking, p.FirewallerFactory.GetFirewaller(e), nil
}

// DetectRegions implements environs.CloudRegionDetector.
func (EnvironProvider) DetectRegions() ([]cloud.Region, error) {
	// If OS_REGION_NAME and OS_AUTH_URL are both set,
	// return a region using them.
	creds, err := identity.CredentialsFromEnv()
	if err != nil {
		return nil, errors.Errorf("failed to retrieve credential from env : %v", err)
	}
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
func (p EnvironProvider) Ping(ctx context.Context, endpoint string) error {
	c := p.ClientFromEndpoint(endpoint)
	if _, err := c.IdentityAuthOptions(); err != nil {
		return errors.Annotatef(err, "No Openstack server running at %s", endpoint)
	}
	return nil
}

// newGooseClient is the default function in EnvironProvider.ClientFromEndpoint.
func newGooseClient(endpoint string) client.AuthenticatingClient {
	// Use NonValidatingClient, in case the endpoint is behind a cert
	return client.NewNonValidatingClient(&identity.Credentials{URL: endpoint}, 0, nil)
}

// ValidateCloud is specified in the EnvironProvider interface.
func (p EnvironProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return errors.Annotate(validateCloudSpec(spec), "validating cloud spec")
}

// AgentMetadataLookupParams returns parameters which are used to query agent metadata to
// find matching image information.
func (p EnvironProvider) AgentMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	return p.metadataLookupParams(region)
}

// ImageMetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p EnvironProvider) ImageMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	return p.metadataLookupParams(region)
}

func (p EnvironProvider) metadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, errors.Errorf("region must be specified")
	}
	return &simplestreams.MetadataLookupParams{
		Region: region,
	}, nil
}

func (p EnvironProvider) newConfig(ctx context.Context, cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(ctx, cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

type Environ struct {
	common.CredentialInvalidator
	environs.NoSpaceDiscoveryEnviron
	environs.NoContainerAddressesEnviron

	name           string
	modelUUID      string
	controllerUUID string
	namespace      instance.Namespace

	ecfgMutex       sync.Mutex
	ecfgUnlocked    *environConfig
	cloudUnlocked   environscloudspec.CloudSpec
	clientUnlocked  client.AuthenticatingClient
	novaUnlocked    *nova.Client
	neutronUnlocked NetworkingNeutron
	volumeURL       *url.URL

	// keystoneImageDataSource caches the result of getKeystoneImageSource.
	keystoneImageDataSourceMutex sync.Mutex
	keystoneImageDataSource      simplestreams.DataSource

	// keystoneToolsDataSource caches the result of getKeystoneToolsSource.
	keystoneToolsDataSourceMutex sync.Mutex
	keystoneToolsDataSource      simplestreams.DataSource

	// usingSecurityGroups tracks whether this model is using security groups
	// for firewalling. This will be false if a network has port_security disabled,
	// true otherwise.
	// However, once a model security group is created, it is not removed if such a model
	// is added, this option sticks to true
	usingSecurityGroups bool

	firewaller   Firewaller
	networking   Networking
	configurator ProviderConfigurator
	flavorFilter FlavorFilter

	// Clock is defined so it can be replaced for testing
	clock clock.Clock

	publicIPMutex sync.Mutex
}

var _ environs.Environ = (*Environ)(nil)

var _ environs.NetworkingEnviron = (*Environ)(nil)

var _ simplestreams.HasRegion = (*Environ)(nil)

var _ environs.InstanceTagger = (*Environ)(nil)

type openstackInstance struct {
	e        *Environ
	instType *instances.InstanceType
	arch     *string

	mu           sync.Mutex
	serverDetail *nova.ServerDetail
	// floatingIP is non-nil iff use-floating-ip is true.
	floatingIP *string

	// runOpts is only set in the response from StartInstance.
	runOpts *nova.RunServerOpts
}

// NovaInstanceStartedWithOpts exposes run options used to start an instance.
// Used by unit testing.
func (inst *openstackInstance) NovaInstanceStartedWithOpts() *nova.RunServerOpts {
	return inst.runOpts
}

func (inst *openstackInstance) String() string {
	return string(inst.Id())
}

var _ instances.Instance = (*openstackInstance)(nil)

func (inst *openstackInstance) Refresh(ctx context.Context) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	server, err := inst.e.nova().GetServer(inst.serverDetail.Id)
	if err != nil {
		return inst.e.HandleCredentialError(ctx, err)
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

func (inst *openstackInstance) Status(ctx context.Context) instance.Status {
	instStatus := inst.getServerDetail().Status
	var jujuStatus status.Status
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
	return instance.Status{
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
	if inst.serverDetail.AvailabilityZone != "" {
		hc.AvailabilityZone = &inst.serverDetail.AvailabilityZone
	}
	// If the instance was started with a volume block device mapping, select the first
	// boot disk as the reported RootDisk size.
	if inst.runOpts != nil {
		for _, blockDevice := range inst.runOpts.BlockDeviceMappings {
			if blockDevice.BootIndex == 0 &&
				blockDevice.DestinationType == rootDiskSourceVolume {
				rootDiskSize := uint64(blockDevice.VolumeSize * 1024)
				hc.RootDisk = &rootDiskSize
				break
			}
		}
	}
	return hc
}

// getAddresses returns the existing server information on addresses,
// but fetches the details over the api again if no addresses exist.
func (inst *openstackInstance) getAddresses(ctx context.Context) (map[string][]nova.IPAddress, error) {
	addrs := inst.getServerDetail().Addresses
	if len(addrs) == 0 {
		server, err := inst.e.nova().GetServer(string(inst.Id()))
		if err != nil {
			return nil, inst.e.HandleCredentialError(ctx, err)
		}
		addrs = server.Addresses
	}
	return addrs, nil
}

// Addresses implements network.Addresses() returning generic address
// details for the instances, and calling the openstack api if needed.
func (inst *openstackInstance) Addresses(ctx context.Context) (network.ProviderAddresses, error) {
	addresses, err := inst.getAddresses(ctx)
	if err != nil {
		return nil, err
	}
	var floatingIP string
	if inst.floatingIP != nil {
		floatingIP = *inst.floatingIP
		logger.Debugf(ctx, "instance %v has floating IP address: %v", inst.Id(), floatingIP)
	}
	return convertNovaAddresses(ctx, floatingIP, addresses), nil
}

// convertNovaAddresses returns nova addresses in generic format
func convertNovaAddresses(ctx context.Context, publicIP string, addresses map[string][]nova.IPAddress) network.ProviderAddresses {
	var machineAddresses []network.ProviderAddress
	if publicIP != "" {
		publicAddr := network.NewMachineAddress(publicIP, network.WithScope(network.ScopePublic)).AsProviderAddress()
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
			addrType := network.IPv4Address
			if address.Version == 6 {
				addrType = network.IPv6Address
			}
			machineAddr := network.NewMachineAddress(address.Address, network.WithScope(networkScope)).AsProviderAddress()
			if machineAddr.Type != addrType {
				logger.Warningf(ctx, "derived address type %v, nova reports %v", machineAddr.Type, addrType)
			}
			machineAddresses = append(machineAddresses, machineAddr)
		}
	}
	return machineAddresses
}

func (inst *openstackInstance) OpenPorts(ctx context.Context, machineId string, rules firewall.IngressRules) error {
	return inst.e.firewaller.OpenInstancePorts(ctx, inst, machineId, rules)
}

func (inst *openstackInstance) ClosePorts(ctx context.Context, machineId string, rules firewall.IngressRules) error {
	return inst.e.firewaller.CloseInstancePorts(ctx, inst, machineId, rules)
}

func (inst *openstackInstance) IngressRules(ctx context.Context, machineId string) (firewall.IngressRules, error) {
	return inst.e.firewaller.InstanceIngressRules(ctx, inst, machineId)
}

func (e *Environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *Environ) cloud() environscloudspec.CloudSpec {
	e.ecfgMutex.Lock()
	cloud := e.cloudUnlocked
	e.ecfgMutex.Unlock()
	return cloud
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

func (e *Environ) neutron() NetworkingNeutron {
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
func (e *Environ) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterConflicts(
		[]string{constraints.InstanceType},
		// TODO: move to a dynamic conflict for arch when openstack supports defining arch in flavors
		[]string{constraints.Mem, constraints.Cores})
	// NOTE: RootDiskSource and RootDisk constraints are validated in PrecheckInstance.
	validator.RegisterUnsupported(unsupportedConstraints)
	novaClient := e.nova()
	flavors, err := novaClient.ListFlavorsDetail()
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	instTypeNames := make([]string, len(flavors))
	for i, flavor := range flavors {
		instTypeNames[i] = flavor.Name
	}
	sort.Strings(instTypeNames)
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)
	validator.RegisterVocabulary(constraints.VirtType, []string{"kvm", "lxd"})
	validator.RegisterVocabulary(constraints.RootDiskSource, []string{rootDiskSourceVolume, rootDiskSourceLocal})
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
func (e *Environ) AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	zones, err := novaListAvailabilityZones(e.nova())
	if gooseerrors.IsNotImplemented(err) {
		return nil, errors.NotImplementedf("availability zones")
	}
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	availabilityZones := make(network.AvailabilityZones, len(zones))
	for i, z := range zones {
		availabilityZones[i] = &openstackAvailabilityZone{z}
	}
	return availabilityZones, nil
}

// InstanceAvailabilityZoneNames returns the availability zone names for each
// of the specified instances.
func (e *Environ) InstanceAvailabilityZoneNames(ctx context.Context, ids []instance.Id) (map[instance.Id]string, error) {
	instances, err := e.Instances(ctx, ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, e.HandleCredentialError(ctx, err)
	}
	zones := make(map[instance.Id]string)
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		oInst, ok := inst.(*openstackInstance)
		if !ok {
			continue
		}
		zones[inst.Id()] = oInst.serverDetail.AvailabilityZone
	}
	return zones, nil
}

type openstackPlacement struct {
	zoneName string
}

// DeriveAvailabilityZones is part of the common.ZonedEnviron interface.
func (e *Environ) DeriveAvailabilityZones(ctx context.Context, args environs.StartInstanceParams) ([]string, error) {
	availabilityZone, err := e.deriveAvailabilityZone(ctx, args.Placement, args.VolumeAttachments)
	if err != nil && !errors.Is(err, errors.NotImplemented) {
		return nil, e.HandleCredentialError(ctx, err)
	}
	if availabilityZone != "" {
		return []string{availabilityZone}, nil
	}
	return nil, nil
}

func (e *Environ) parsePlacement(ctx context.Context, placement string) (*openstackPlacement, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		return nil, errors.Errorf("unknown placement directive: %v", placement)
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		zones, err := e.AvailabilityZones(ctx)
		if err != nil {
			return nil, e.HandleCredentialError(ctx, err)
		}
		if err := zones.Validate(value); err != nil {
			return nil, errors.Trace(err)
		}

		return &openstackPlacement{zoneName: value}, nil
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

// PrecheckInstance is defined on the environs.InstancePrechecker interface.
func (e *Environ) PrecheckInstance(ctx context.Context, args environs.PrecheckInstanceParams) error {
	if _, err := e.deriveAvailabilityZone(ctx, args.Placement, args.VolumeAttachments); err != nil {
		return errors.Trace(err)
	}
	usingVolumeRootDisk := false
	if args.Constraints.HasRootDiskSource() && args.Constraints.HasRootDisk() &&
		*args.Constraints.RootDiskSource == rootDiskSourceVolume {
		usingVolumeRootDisk = true
	}
	if args.Constraints.HasRootDisk() && args.Constraints.HasInstanceType() && !usingVolumeRootDisk {
		return errors.Errorf("constraint %s cannot be specified with %s unless constraint %s=%s",
			constraints.RootDisk, constraints.InstanceType,
			constraints.RootDiskSource, rootDiskSourceVolume)
	}
	if args.Constraints.HasInstanceType() {
		// Constraint has an instance-type constraint so let's see if it is valid.
		novaClient := e.nova()
		flavors, err := novaClient.ListFlavorsDetail()
		if err != nil {
			return e.HandleCredentialError(ctx, err)
		}
		flavorFound := false
		for _, flavor := range flavors {
			if flavor.Name == *args.Constraints.InstanceType {
				flavorFound = true
				break
			}
		}
		if !flavorFound {
			return errors.Errorf("invalid Openstack flavour %q specified", *args.Constraints.InstanceType)
		}
	}

	return nil
}

// PrepareForBootstrap is part of the Environ interface.
func (e *Environ) PrepareForBootstrap(ctx environs.BootstrapContext, _ string) error {
	// Verify credentials.
	if err := authenticateClient(ctx, e.client()); err != nil {
		return err
	}
	if !e.supportsNeutron() {
		logger.Errorf(ctx, `Using unsupported OpenStack APIs.

  Neutron networking is not supported by this OpenStack cloud.

  Please use OpenStack Queens or newer to maintain compatibility.

`,
		)
		return errors.NewNotFound(nil, "OpenStack Neutron service")
	}
	return nil
}

func (e *Environ) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	// The client's authentication may have been reset when finding tools if the agent-version
	// attribute was updated so we need to re-authenticate. This will be a no-op if already authenticated.
	// An authenticated client is needed for the URL() call below.
	if err := authenticateClient(ctx, e.client()); err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	result, err := common.Bootstrap(ctx, e, args)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	return result, nil
}

func (e *Environ) supportsNeutron() bool {
	client := e.client()
	endpointMap := client.EndpointsForRegion(e.cloud().Region)
	_, ok := endpointMap["network"]
	return ok
}

func (e *Environ) ControllerInstances(ctx context.Context, controllerUUID string) ([]instance.Id, error) {
	// Find all instances tagged with tags.JujuIsController.
	instances, err := e.allControllerManagedInstances(ctx, controllerUUID)
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

func newCredentials(spec environscloudspec.CloudSpec) (identity.Credentials, identity.AuthMode, error) {
	credAttrs := spec.Credential.Attributes()
	cred := identity.Credentials{
		Region:     spec.Region,
		URL:        spec.Endpoint,
		TenantName: credAttrs[CredAttrTenantName],
		TenantID:   credAttrs[CredAttrTenantID],
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
		if credAttrs[CredAttrVersion] != "" {
			version, err := strconv.Atoi(credAttrs[CredAttrVersion])
			if err != nil {
				return identity.Credentials{}, 0,
					errors.Errorf("cred.Version is not a valid integer type : %v", err)
			}
			if version < 3 {
				authMode = identity.AuthUserPass
			} else {
				authMode = identity.AuthUserPassV3
			}
			cred.Version = version
		} else if cred.Domain != "" || cred.UserDomain != "" || cred.ProjectDomain != "" {
			authMode = identity.AuthUserPassV3
		} else {
			authMode = identity.AuthUserPass
		}
	case cloud.AccessKeyAuthType:
		cred.User = credAttrs[CredAttrAccessKey]
		cred.Secrets = credAttrs[CredAttrSecretKey]
		authMode = identity.AuthKeyPair
	}
	return cred, authMode, nil
}

func tlsConfig(certStrs []string) *tls.Config {
	pool := x509.NewCertPool()
	for _, cert := range certStrs {
		pool.AppendCertsFromPEM([]byte(cert))
	}
	tlsConfig := http.SecureTLSConfig()
	tlsConfig.RootCAs = pool
	return tlsConfig
}

type authenticator interface {
	Authenticate() error
}

var authenticateClient = func(ctx context.Context, auth authenticator) error {
	err := auth.Authenticate()
	if err != nil {
		// Log the error in case there are any useful hints,
		// but provide a readable and helpful error message
		// to the user.
		logger.Debugf(ctx, "Authenticate() failed: %v", err)
		if gooseerrors.IsUnauthorised(err) {
			return errors.Errorf("authentication failed : %v\n"+
				"Please ensure the credentials are correct. A common mistake is\n"+
				"to specify the wrong tenant. Use the OpenStack project name\n"+
				"for tenant-name in your model configuration. \n", err)
		}
		return errors.Annotate(err, "authentication failed.")
	}
	return nil
}

func (e *Environ) SetConfig(ctx context.Context, cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(ctx, cfg)
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

// SetCloudSpec is specified in the environs.Environ interface.
func (e *Environ) SetCloudSpec(_ context.Context, spec environscloudspec.CloudSpec) error {
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()

	if err := validateCloudSpec(spec); err != nil {
		return errors.Annotate(err, "validating cloud spec")
	}
	e.cloudUnlocked = spec

	// Create a new client factory, that creates the clients according to the
	// auth client version, cloudspec and configuration.
	//
	// In theory we should be able to create one client factory and then every
	// time openstack wants a goose client, we should be just get one from
	// the factory.
	factory := NewClientFactory(spec, e.ecfgUnlocked)
	if err := factory.Init(); err != nil {
		return errors.Trace(err)
	}
	e.clientUnlocked = factory.AuthClient()

	// The following uses different clients for the different openstack clients
	// and we create them in the factory.
	var err error
	e.novaUnlocked, err = factory.Nova()
	if err != nil {
		return errors.Trace(err)
	}
	e.neutronUnlocked, err = factory.Neutron()
	if err != nil {
		return errors.Trace(err)
	}
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
	var tail string
	urlpath := strings.ToLower(url.Path)
	urlpath, tail = path.Split(urlpath)
	if len(tail) == 0 && len(urlpath) > 2 {
		// trailing /, remove it and split again
		_, tail = path.Split(strings.TrimRight(urlpath, "/"))
	}
	versionNumStr := strings.TrimPrefix(tail, "v")
	logger.Debugf(context.TODO(), "authURL: %s", authURL)
	major, _, err := semversion.ParseMajorMinor(versionNumStr)
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
	return e.getKeystoneDataSource(context.TODO(), &e.keystoneImageDataSourceMutex, &e.keystoneImageDataSource, "product-streams")
}

// getKeystoneToolsSource is a tools.ToolsDataSourceFunc that
// returns a DataSource using the "juju-tools" keystone URL.
func getKeystoneToolsSource(env environs.Environ) (simplestreams.DataSource, error) {
	e, ok := env.(*Environ)
	if !ok {
		return nil, errors.NotSupportedf("non-openstack model")
	}
	return e.getKeystoneDataSource(context.TODO(), &e.keystoneToolsDataSourceMutex, &e.keystoneToolsDataSource, "juju-tools")
}

func (e *Environ) getKeystoneDataSource(ctx context.Context, mu *sync.Mutex, datasource *simplestreams.DataSource, keystoneName string) (simplestreams.DataSource, error) {
	mu.Lock()
	defer mu.Unlock()
	if *datasource != nil {
		return *datasource, nil
	}

	cl := e.client()
	if !cl.IsAuthenticated() {
		if err := authenticateClient(ctx, cl); err != nil {
			return nil, err
		}
	}

	serviceURL, err := makeServiceURL(cl, keystoneName, "", nil)
	if err != nil {
		return nil, errors.NewNotSupported(err, fmt.Sprintf("cannot make service URL: %v", err))
	}
	cfg := simplestreams.Config{
		Description:          "keystone catalog",
		BaseURL:              serviceURL,
		HostnameVerification: e.Config().SSLHostnameVerification(),
		Priority:             simplestreams.SPECIFIC_CLOUD_DATA,
		CACertificates:       e.cloudUnlocked.CACertificates,
	}
	if err := cfg.Validate(); err != nil {
		return nil, errors.Annotate(err, "simplestreams config validation failed")
	}
	*datasource = simplestreams.NewDataSource(cfg)
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

// StartInstance is specified in the InstanceBroker interface.
func (e *Environ) StartInstance(
	ctx context.Context, args environs.StartInstanceParams,
) (*environs.StartInstanceResult, error) {
	res, err := e.startInstance(ctx, args)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	return res, nil
}

func (e *Environ) startInstance(
	ctx context.Context, args environs.StartInstanceParams,
) (_ *environs.StartInstanceResult, err error) {
	if err := e.validateAvailabilityZone(ctx, args); err != nil {
		return nil, errors.Trace(err)
	}

	arch, err := args.Tools.OneArch()
	if err != nil {
		return nil, errors.Trace(err)
	}
	spec, err := findInstanceSpec(e, instances.InstanceConstraint{
		Region:      e.cloud().Region,
		Base:        args.InstanceConfig.Base,
		Arch:        arch,
		Constraints: args.Constraints,
	}, args.ImageMetadata)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	if err := args.InstanceConfig.SetTools(args.Tools); err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, e.Config()); err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	cloudCfg, err := e.configurator.GetCloudConfig(args)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	networks, err := e.networksForInstance(ctx, args, cloudCfg)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}

	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudCfg, OpenstackRenderer{})
	if err != nil {
		return nil, environs.ZoneIndependentError(errors.Annotate(err, "cannot make user data"))
	}
	logger.Debugf(ctx, "openstack user data; %d bytes", len(userData))

	machineName := resourceName(
		e.namespace,
		e.name,
		args.InstanceConfig.MachineId,
	)

	if e.ecfg().useOpenstackGBP() {
		neutronClient := e.neutron()
		ptArg := neutron.PolicyTargetV2{
			Name:                fmt.Sprintf("juju-policytarget-%s", machineName),
			PolicyTargetGroupId: e.ecfg().policyTargetGroup(),
		}
		pt, err := neutronClient.CreatePolicyTargetV2(ptArg)
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
		neutronClient := e.neutron()
		for _, n := range networks {
			if n.NetworkId == "" {
				// It's a GBP network.
				continue
			}
			net, err := neutronClient.GetNetworkV2(n.NetworkId)
			if err != nil {
				return nil, environs.ZoneIndependentError(err)
			}
			if net.PortSecurityEnabled != nil && !*net.PortSecurityEnabled {
				createSecurityGroups = *net.PortSecurityEnabled
				logger.Infof(ctx, "network %q has port_security_enabled set to false. Not using security groups.", net.Id)
				break
			}
		}
	}
	e.usingSecurityGroups = e.usingSecurityGroups || createSecurityGroups

	var novaGroupNames []nova.SecurityGroupName
	if createSecurityGroups {
		groupNames, err := e.firewaller.SetUpGroups(ctx, args.ControllerUUID, args.InstanceConfig.MachineId)
		if err != nil {
			return nil, environs.ZoneIndependentError(errors.Annotate(err, "cannot set up groups"))
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
				_ = args.StatusCallback(
					ctx,
					status.Provisioning,
					fmt.Sprintf("%s, wait 10 seconds before retry, attempt %d", lastError, attempt), nil)
			},
			IsFatalError: func(err error) bool {
				return err != errStillBuilding
			},
			Stop: ctx.Done(),
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
			if server == nil {
				logger.Warningf(ctx, "may have lost contact with nova api while creating instances, some stray instances may be around and need to be deleted")
				break
			}
			var serverDetail *nova.ServerDetail
			serverDetail, err = waitForActiveServerDetails(client, server.Id, 5*time.Minute)
			if err != nil || serverDetail == nil {
				// If we got an error back (eg. StillBuilding)
				// we need to terminate the instance before
				// retrying to avoid leaking resources.
				logger.Warningf(ctx, "Unable to retrieve details for created instance %q: %v; attempting to terminate it", server.Id, err)
				if termErr := e.terminateInstances(ctx, []instance.Id{instance.Id(server.Id)}); termErr != nil {
					logger.Errorf(ctx, "Failed to delete instance %q: %v; manual cleanup required", server.Id, termErr)
				}
				server = nil
				break
			} else if serverDetail.Status == nova.StatusActive {
				break
			} else if serverDetail.Status == nova.StatusError {
				// Perhaps there is an error case where a retry in the same AZ
				// is a good idea.
				faultMsg := " unable to determine fault details"
				if serverDetail.Fault != nil {
					faultMsg = fmt.Sprintf(" with fault %q", serverDetail.Fault.Message)
				} else {
					logger.Debugf(ctx, "getting active server details from nova failed without fault details")
				}
				logger.Infof(ctx, "Deleting instance %q in ERROR state%v", server.Id, faultMsg)
				if err = e.terminateInstances(ctx, []instance.Id{instance.Id(server.Id)}); err != nil {
					logger.Errorf(ctx, "Failed to delete instance in ERROR state %q: %v; manual cleanup required", server.Id, err)
				}
				server = nil
				err = errors.New(faultMsg)
				break
			}
		}
		return server, err
	}

	opts := nova.RunServerOpts{
		Name:               machineName,
		FlavorId:           spec.InstanceType.Id,
		UserData:           userData,
		SecurityGroupNames: novaGroupNames,
		Networks:           networks,
		Metadata:           args.InstanceConfig.Tags,
		AvailabilityZone:   args.AvailabilityZone,
	}

	err = e.configureRootDisk(ctx, args, spec, &opts)
	if err != nil {
		return nil, environs.ZoneIndependentError(err)
	}
	e.configurator.ModifyRunServerOptions(&opts)

	server, err := tryStartNovaInstance(shortAttempt, e.nova(), opts)
	if err != nil || server == nil {
		// Attempt to clean up any security groups we created.
		if err := e.firewaller.DeleteMachineGroup(ctx, args.InstanceConfig.MachineId); err != nil {
			// If we failed to clean up the security groups, we need the user
			// to manually clean them up.
			logger.Errorf(ctx, "cannot cleanup security groups: %v", err)
		}

		logger.Debugf(ctx, "cannot run instance full error: %q", err)
		err = errors.Annotate(errors.Cause(err), "cannot run instance")
		// Improve the error message if there is no valid network.
		if isInvalidNetworkError(err) {
			err = e.userFriendlyInvalidNetworkError(err)
		}
		// 'No valid host available' is typically a resource error,
		// let the provisioner know it is a good idea to try another
		// AZ if available.
		if !isNoValidHostsError(err) {
			err = environs.ZoneIndependentError(err)
		}
		return nil, err
	}

	detail, err := e.nova().GetServer(server.Id)
	if err != nil {
		return nil, environs.ZoneIndependentError(errors.Annotate(err, "cannot get started instance"))
	}

	inst := &openstackInstance{
		e:            e,
		serverDetail: detail,
		arch:         &spec.Image.Arch,
		instType:     &spec.InstanceType,
		runOpts:      &opts,
	}
	logger.Infof(ctx, "started instance %q", inst.Id())
	var withPublicIP bool
	// Any machine constraint for allocating a public IP address
	// overrides the (deprecated) model config.
	if args.Constraints.HasAllocatePublicIP() {
		withPublicIP = *args.Constraints.AllocatePublicIP
	}
	if withPublicIP {
		// If we don't lock here, AllocatePublicIP() can return the same
		// public IP for 2 different instances. Only one will successfully
		// be assigned the public IP, the other will not have one.
		e.publicIPMutex.Lock()
		defer e.publicIPMutex.Unlock()
		var publicIP *string
		logger.Debugf(ctx, "allocating public IP address for openstack node")
		if fip, err := e.networking.AllocatePublicIP(inst.Id()); err != nil {
			if err := e.terminateInstances(ctx, []instance.Id{inst.Id()}); err != nil {
				// ignore the failure at this stage, just log it
				logger.Debugf(ctx, "failed to terminate instance %q: %v", inst.Id(), err)
			}
			return nil, environs.ZoneIndependentError(errors.Annotate(err, "cannot allocate a public IP as needed"))
		} else {
			publicIP = fip
			logger.Infof(ctx, "allocated public IP %s", *publicIP)
		}

		if err := e.assignPublicIP(publicIP, string(inst.Id())); err != nil {
			if err := e.terminateInstances(ctx, []instance.Id{inst.Id()}); err != nil {
				// ignore the failure at this stage, just log it
				logger.Debugf(ctx, "failed to terminate instance %q: %v", inst.Id(), err)
			}
			return nil, environs.ZoneIndependentError(errors.Annotatef(err,
				"cannot assign public address %s to instance %q",
				*publicIP, inst.Id(),
			))
		}
		inst.floatingIP = publicIP
	}

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: inst.hardwareCharacteristics(),
	}, nil
}

func (e *Environ) userFriendlyInvalidNetworkError(err error) error {
	msg := fmt.Sprintf("%s\n\t%s\n\t%s", err.Error(),
		"This error was caused by juju attempting to create an OpenStack instance with no network defined.",
		"No network has been configured.")
	networks, err := e.networking.FindNetworks(true)
	if err != nil {
		msg += fmt.Sprintf("\n\t%s\n\t\t%s", "Error attempting to find internal networks:", err.Error())
	} else {
		msg += fmt.Sprintf(" %s\n\t\t%q", "The following internal networks are available: ", strings.Join(networks.Values(), ", "))
	}
	return errors.New(msg)
}

// validateAvailabilityZone validates AZs supplied in StartInstanceParams.
// args.AvailabilityZone should only be set if this OpenStack supports zones.
// We need to validate it if supplied.
func (e *Environ) validateAvailabilityZone(ctx context.Context, args environs.StartInstanceParams) error {
	if args.AvailabilityZone == "" {
		return nil
	}

	volumeAttachmentsZone, err := e.volumeAttachmentsZone(args.VolumeAttachments)
	if err != nil {
		return environs.ZoneIndependentError(err)
	}
	if err := validateAvailabilityZoneConsistency(args.AvailabilityZone, volumeAttachmentsZone); err != nil {
		return environs.ZoneIndependentError(err)
	}

	zones, err := e.AvailabilityZones(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(zones.Validate(args.AvailabilityZone))

}

// networksForInstance returns networks that will be attached
// to a new Openstack instance.
// Network info for all ports created is represented in the input cloud-config
// reference.
// This is necessary so that the correct Netplan representation for the
// associated NICs is rendered in the instance that they will be attached to.
func (e *Environ) networksForInstance(
	ctx context.Context,
	args environs.StartInstanceParams, cloudCfg cloudinit.NetworkingConfig,
) ([]nova.ServerNetworks, error) {
	networks, err := e.networksForModel(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If there are no constraints or bindings to accommodate,
	// the instance will have a NIC for each configured internal network.
	// Note that uif there is no configured network, this means a NIC in
	// all *available* networks.
	if len(args.SubnetsToZones) == 0 {
		toServerNet := func(n neutron.NetworkV2) nova.ServerNetworks { return nova.ServerNetworks{NetworkId: n.Id} }
		return transform.Slice(networks, toServerNet), nil
	}

	if len(networks) == 0 {
		return nil, errors.New(
			"space constraints and/or bindings were supplied, but no OpenStack networks can be determined")
	}

	subnetIDForZone, err := subnetInZone(ctx, args.AvailabilityZone, args.SubnetsToZones)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Set the subnetID on the network for all networks.
	// For each of the subnetIDs selected, create a port for each one.
	subnetNetworks := make([]nova.ServerNetworks, 0, len(subnetIDForZone))
	netInfo := make(network.InterfaceInfos, len(subnetIDForZone))
	for i, subnetID := range subnetIDForZone {
		subnetNet, err := networkForSubnet(networks, subnetID)
		if err != nil {
			return nil, errors.Trace(err)
		}

		var port *neutron.PortV2
		port, err = e.networking.CreatePort(e.modelUUID, subnetNet.Id, subnetID)
		if err != nil {
			break
		}

		logger.Infof(ctx, "created new port %q connected to Openstack subnet %q", port.Id, subnetID)
		subnetNetworks = append(subnetNetworks, nova.ServerNetworks{
			NetworkId: subnetNet.Id,
			PortId:    port.Id,
		})

		// We expect a single address,
		// but for correctness we add all from the created port.
		ips := make([]string, len(port.FixedIPs))
		for j, fixedIP := range port.FixedIPs {
			ips[j] = fixedIP.IPAddress
		}

		netInfo[i] = network.InterfaceInfo{
			InterfaceName: fmt.Sprintf("eth%d", i),
			MACAddress:    port.MACAddress,
			Addresses:     network.NewMachineAddresses(ips).AsProviderAddresses(),
			ConfigType:    network.ConfigDHCP,
			Origin:        network.OriginProvider,
		}
	}

	err = cloudCfg.AddNetworkConfig(netInfo)

	if err != nil {
		err1 := e.DeletePorts(subnetNetworks)
		if err1 != nil {
			logger.Errorf(ctx, "Unable to delete ports from the provider %+v", subnetNetworks)
		}
		return nil, errors.Annotatef(err, "creating ports for instance")
	}

	return subnetNetworks, nil
}

// subnetInZone chooses a subnet at random for each entry in the input slice of
// subnet:zones that is in the input availability zone.
func subnetInZone(ctx context.Context, az string, subnetsToZones []map[network.Id][]string) ([]network.Id, error) {
	// Attempt to filter the constraint/binding subnet IDs for the supplied
	// availability zone.
	// The zone is supplied by the provisioner based on its attempt to maintain
	// distribution of units across zones.
	// The zones recorded against O7k subnets in Juju are affected by the
	// `availability_zone_hints` attribute on the network where they reside,
	// and the default AZ configuration for the project.
	// They are a read-only attribute.
	// If a subnet in the subnets-to-zones map has no zones, we assume a basic
	// O7k set-up where networks are not zone-limited. We log a warning and
	// consider all the supplied subnets.
	// See:
	// - https://docs.openstack.org/neutron/latest/admin/config-az.html#availability-zone-related-attributes
	// - https://docs.openstack.org/neutron/latest/admin/ovn/availability_zones.html#network-availability-zones
	subnetIDsForZone := make([][]network.Id, len(subnetsToZones))
	for i, nic := range subnetsToZones {
		subnetIDs, err := network.FindSubnetIDsForAvailabilityZone(az, nic)
		if err != nil {
			// We found no subnets in the zone.
			// Add subnets without zone-limited networks.
			for subnetID, zones := range nic {
				if len(zones) == 0 {
					logger.Warningf(ctx,
						"subnet %q is not in a network with availability zones listed; assuming availability in zone %q",
						subnetID, az)
					subnetIDs = append(subnetIDs, subnetID)
				}
			}

			if len(subnetIDs) == 0 {
				// If we still have no candidate subnets, then they are all in
				// networks with availability zones, and none of those zones
				// match the input one. Return the error we have in hand.
				return nil, errors.Annotatef(err, "determining subnets in zone %q", az)
			}
		}

		subnetIDsForZone[i] = network.FilterInFanNetwork(subnetIDs)
	}

	// For each list of subnet IDs that satisfy space and zone constraints,
	// choose a single one at random.
	subnetIDForZone := make([]network.Id, len(subnetIDsForZone))
	for i, subnetIDs := range subnetIDsForZone {
		if len(subnetIDs) == 1 {
			subnetIDForZone[i] = subnetIDs[0]
			continue
		}
		subnetIDForZone[i] = subnetIDs[rand.Intn(len(subnetIDs))]
	}

	return subnetIDForZone, nil
}

// DeletePorts goes through and attempts to delete any ports that have been
// created during the creation of the networks for the given instance.
func (e *Environ) DeletePorts(networks []nova.ServerNetworks) error {
	var errs []error
	for _, network := range networks {
		if network.NetworkId != "" && network.PortId != "" {
			err := e.networking.DeletePortByID(network.PortId)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		// It would be nice to generalize this so we have the same expected
		// behaviour from all our slices of errors.
		for _, err := range errs {
			logger.Errorf(context.TODO(), "Unable to delete port with error: %v", err)
		}
		return errs[0]
	}
	return nil
}

// networksForModel returns the Openstack network list
// based on current model configuration.
func (e *Environ) networksForModel(ctx context.Context) ([]neutron.NetworkV2, error) {
	var resolvedNetworks []neutron.NetworkV2
	networkIDs := set.NewStrings()
	cfgNets := e.ecfg().networks()

	for _, cfgNet := range cfgNets {
		networks, err := e.networking.ResolveNetworks(cfgNet, false)
		if err != nil {
			logger.Warningf(ctx, "filtering networks for %q", cfgNet)
		}

		for _, net := range networks {
			if networkIDs.Contains(net.Id) {
				continue
			}

			resolvedNetworks = append(resolvedNetworks, net)
			networkIDs.Add(net.Id)
		}
	}

	if networkIDs.Size() == 0 {
		if len(cfgNets) == 1 && cfgNets[0] == "" {
			return nil, nil
		}
		return nil, errors.Errorf("unable to determine networks for configured list: %v", cfgNets)
	}

	logger.Debugf(ctx, "using network IDs %v", networkIDs.Values())
	return resolvedNetworks, nil
}

func (e *Environ) configureRootDisk(_ context.Context, args environs.StartInstanceParams,
	spec *instances.InstanceSpec, runOpts *nova.RunServerOpts) error {
	rootDiskSource := rootDiskSourceLocal
	if args.Constraints.HasRootDiskSource() {
		rootDiskSource = *args.Constraints.RootDiskSource
	}
	rootDiskMapping := nova.BlockDeviceMapping{
		BootIndex:  0,
		UUID:       spec.Image.Id,
		SourceType: "image",
		// NB constraints.RootDiskSource in the case of OpenStack represents
		// the type of block device to use. Either "local" to represent a local
		// block device or "volume" to represent a block device from the cinder
		// block storage service.
		DestinationType:     rootDiskSource,
		DeleteOnTermination: true,
	}
	switch rootDiskSource {
	case rootDiskSourceLocal:
		if args.Constraints.HasImageID() {
			runOpts.ImageId = *args.Constraints.ImageID
		} else {
			runOpts.ImageId = spec.Image.Id
		}
	case rootDiskSourceVolume:
		if args.Constraints.HasImageID() {
			runOpts.ImageId = *args.Constraints.ImageID
		}
		size := uint64(0)
		if args.Constraints.HasRootDisk() {
			size = *args.Constraints.RootDisk
		}
		if size <= 0 {
			size = defaultRootDiskSize
		}
		sizeGB := common.MiBToGiB(size)
		rootDiskMapping.VolumeSize = int(sizeGB)
	default:
		return errors.Errorf("invalid %s %s", constraints.RootDiskSource, rootDiskSource)
	}
	runOpts.BlockDeviceMappings = []nova.BlockDeviceMapping{rootDiskMapping}
	return nil
}

func (e *Environ) deriveAvailabilityZone(
	ctx context.Context,
	placement string,
	volumeAttachments []storage.VolumeAttachmentParams,
) (string, error) {
	volumeAttachmentsZone, err := e.volumeAttachmentsZone(volumeAttachments)
	if err != nil {
		return "", e.HandleCredentialError(ctx, err)
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
	volumes, err := modelCinderVolumes(cinderProvider.storageAdaptor, cinderProvider.modelUUID)
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

func isInvalidNetworkError(err error) bool {
	if cause := errors.Cause(err); cause != nil {
		return strings.Contains(errors.Cause(err).Error(), "Invalid input for field/attribute networks")
	}
	return false
}

func (e *Environ) StopInstances(ctx context.Context, ids ...instance.Id) error {
	logger.Debugf(ctx, "terminating instances %v", ids)
	if err := e.terminateInstances(ctx, ids); err != nil {
		return e.HandleCredentialError(ctx, err)
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

func (e *Environ) listServers(ctx context.Context, ids []instance.Id) ([]nova.ServerDetail, error) {
	wantedServers := make([]nova.ServerDetail, 0, len(ids))
	if len(ids) == 1 {
		// Common case, single instance, may return NotFound
		var maybeServer *nova.ServerDetail
		maybeServer, err := e.nova().GetServer(string(ids[0]))
		if err != nil {
			return nil, e.HandleCredentialError(ctx, err)
		}
		// Only return server details if it is currently alive
		if maybeServer != nil && e.isAliveServer(*maybeServer) {
			wantedServers = append(wantedServers, *maybeServer)
		}
		return wantedServers, nil
	}
	// List all instances in the environment.
	instances, err := e.AllRunningInstances(ctx)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
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
func (e *Environ) updateFloatingIPAddresses(ctx context.Context, instances map[string]instances.Instance) error {
	servers, err := e.nova().ListServersDetail(jujuMachineFilter())
	if err != nil {
		return e.HandleCredentialError(ctx, err)
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

func (e *Environ) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	foundServers, err := e.listServers(ctx, ids)
	if err != nil {
		logger.Debugf(ctx, "error listing servers: %v", err)
		if !isNotFoundError(err) {
			return nil, e.HandleCredentialError(ctx, err)
		}
	}

	logger.Tracef(ctx, "%d/%d live servers found", len(foundServers), len(ids))
	if len(foundServers) == 0 {
		return nil, environs.ErrNoInstances
	}

	instsById := make(map[string]instances.Instance, len(foundServers))
	for i, server := range foundServers {
		instsById[server.Id] = &openstackInstance{
			e:            e,
			serverDetail: &foundServers[i],
		}
	}

	// Update the instance structs with any floating IP address
	// that has been assigned to the instance.
	if err = e.updateFloatingIPAddresses(ctx, instsById); err != nil {
		return nil, err
	}

	insts := make([]instances.Instance, len(ids))
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
func (e *Environ) AdoptResources(ctx context.Context, controllerUUID string, fromVersion semversion.Number) error {
	var failed []string
	controllerTag := map[string]string{tags.JujuController: controllerUUID}

	instances, err := e.AllInstances(ctx)
	if err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	for _, instance := range instances {
		err := e.TagInstance(ctx, instance.Id(), controllerTag)
		if err != nil {
			logger.Errorf(ctx, "error updating controller tag for instance %s: %v", instance.Id(), err)
			failed = append(failed, instance.Id().String())
			if denied, _ := e.MaybeInvalidateCredentialError(ctx, err); denied {
				// If we have an invvalid credential, there is no need to proceed: we'll fail 100%.
				break
			}
		}
	}

	failedVolumes, err := e.adoptVolumes(controllerTag, ctx)
	if err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	failed = append(failed, failedVolumes...)

	err = e.firewaller.UpdateGroupController(ctx, controllerUUID)
	if err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	if len(failed) != 0 {
		return errors.Errorf("error updating controller tag for some resources: %v", failed)
	}
	return nil
}

func (e *Environ) adoptVolumes(controllerTag map[string]string, ctx context.Context) ([]string, error) {
	cinder, err := e.cinderProvider()
	if errors.Is(err, errors.NotSupported) {
		logger.Debugf(ctx, "volumes not supported: not transferring ownership for volumes")
		return nil, nil
	}
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	// TODO(axw): fix the storage API.
	storageConfig, err := storage.NewConfig("cinder", CinderProviderType, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeSource, err := cinder.VolumeSource(storageConfig)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	volumeIds, err := volumeSource.ListVolumes(ctx)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}

	var failed []string
	for _, volumeId := range volumeIds {
		_, err := cinder.storageAdaptor.SetVolumeMetadata(volumeId, controllerTag)
		if err != nil {
			logger.Errorf(ctx, "error updating controller tag for volume %s: %v", volumeId, err)
			failed = append(failed, volumeId)
			if denied, _ := e.MaybeInvalidateCredentialError(ctx, err); denied {
				// If we have an invvalid credential, there is no need to proceed: we'll fail 100%.
				break
			}
		}
	}
	return failed, nil
}

// AllInstances returns all instances in this environment.
func (e *Environ) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	tagFilter := tagValue{tags.JujuModel, e.ecfg().UUID()}
	instances, err := e.allInstances(ctx, tagFilter)
	if err != nil {
		return instances, e.HandleCredentialError(ctx, err)
	}
	return instances, nil
}

// AllRunningInstances returns all running, available instances in this environment.
func (e *Environ) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	// e.allInstances(...) already handles all instances irrespective of the state, so
	// here 'all' is also 'all running'.
	return e.AllInstances(ctx)
}

// allControllerManagedInstances returns all instances managed by this
// environment's controller, matching the optionally specified filter.
func (e *Environ) allControllerManagedInstances(ctx context.Context, controllerUUID string) ([]instances.Instance, error) {
	tagFilter := tagValue{tags.JujuController, controllerUUID}
	instances, err := e.allInstances(ctx, tagFilter)
	if err != nil {
		return instances, e.HandleCredentialError(ctx, err)
	}
	return instances, nil
}

type tagValue struct {
	tag, value string
}

// allControllerManagedInstances returns all instances managed by this
// environment's controller, matching the optionally specified filter.
func (e *Environ) allInstances(ctx context.Context, tagFilter tagValue) ([]instances.Instance, error) {
	servers, err := e.nova().ListServersDetail(jujuMachineFilter())
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	instsById := make(map[string]instances.Instance)
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
	if err := e.updateFloatingIPAddresses(ctx, instsById); err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	insts := make([]instances.Instance, 0, len(instsById))
	for _, inst := range instsById {
		insts = append(insts, inst)
	}
	return insts, nil
}

func (e *Environ) Destroy(ctx context.Context) error {
	err := common.Destroy(e, ctx)
	if err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	// Delete all security groups remaining in the model.
	if err := e.firewaller.DeleteAllModelGroups(ctx); err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	return nil
}

// DestroyController implements the Environ interface.
func (e *Environ) DestroyController(ctx context.Context, controllerUUID string) error {
	if err := e.Destroy(ctx); err != nil {
		return errors.Annotate(e.HandleCredentialError(ctx, err), "destroying controller model")
	}
	// In case any hosted environment hasn't been cleaned up yet,
	// we also attempt to delete their resources when the controller
	// environment is destroyed.
	if err := e.destroyControllerManagedEnvirons(ctx, controllerUUID); err != nil {
		return errors.Annotate(e.HandleCredentialError(ctx, err), "destroying managed models")
	}
	if err := e.firewaller.DeleteAllControllerGroups(ctx, controllerUUID); err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	return nil
}

// destroyControllerManagedEnvirons destroys all environments managed by this
// models's controller.
func (e *Environ) destroyControllerManagedEnvirons(ctx context.Context, controllerUUID string) error {
	// Terminate all instances managed by the controller.
	insts, err := e.allControllerManagedInstances(ctx, controllerUUID)
	if err != nil {
		return errors.Annotate(err, "listing instances")
	}
	instIds := make([]instance.Id, len(insts))
	for i, inst := range insts {
		instIds[i] = inst.Id()
	}
	if err := e.terminateInstances(ctx, instIds); err != nil {
		return errors.Annotate(e.HandleCredentialError(ctx, err), "terminating instances")
	}

	// Delete all volumes managed by the controller.
	cinder, err := e.cinderProvider()
	if err == nil {
		volumes, err := controllerCinderVolumes(cinder.storageAdaptor, controllerUUID)
		if err != nil {
			return errors.Annotate(e.HandleCredentialError(ctx, err), "listing volumes")
		}
		volIds := volumeInfoToVolumeIds(cinderToJujuVolumeInfos(volumes))
		errs := foreachVolume(ctx, e.CredentialInvalidator, cinder.storageAdaptor, volIds, destroyVolume)
		for i, err := range errs {
			if err == nil {
				continue
			}
			return errors.Annotatef(e.HandleCredentialError(ctx, err), "destroying volume %q", volIds[i])
		}
	} else if !errors.Is(err, errors.NotSupported) {
		return e.HandleCredentialError(ctx, err)
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
func rulesToRuleInfo(groupId string, rules firewall.IngressRules) []neutron.RuleInfoV2 {
	var result []neutron.RuleInfoV2
	for _, r := range rules {
		ruleInfo := neutron.RuleInfoV2{
			Direction:     "ingress",
			ParentGroupId: groupId,
			IPProtocol:    r.PortRange.Protocol,
		}
		if ruleInfo.IPProtocol != "icmp" {
			ruleInfo.PortRangeMin = r.PortRange.FromPort
			ruleInfo.PortRangeMax = r.PortRange.ToPort
		}
		sourceCIDRs := r.SourceCIDRs.Values()
		if len(sourceCIDRs) == 0 {
			sourceCIDRs = append(sourceCIDRs, firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR)
		}
		for _, sr := range sourceCIDRs {
			addrType, _ := network.CIDRAddressType(sr)
			if addrType == network.IPv4Address {
				ruleInfo.EthernetType = "IPv4"
			} else if addrType == network.IPv6Address {
				ruleInfo.EthernetType = "IPv6"
			} else {
				// Should never happen; ignore CIDR
				continue
			}
			ruleInfo.RemoteIPPrefix = sr
			result = append(result, ruleInfo)
		}
	}
	return result
}

func (e *Environ) OpenPorts(ctx context.Context, rules firewall.IngressRules) error {
	if err := e.firewaller.OpenPorts(ctx, rules); err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	return nil
}

func (e *Environ) ClosePorts(ctx context.Context, rules firewall.IngressRules) error {
	if err := e.firewaller.ClosePorts(ctx, rules); err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	return nil
}

func (e *Environ) IngressRules(ctx context.Context) (firewall.IngressRules, error) {
	rules, err := e.firewaller.IngressRules(ctx)
	if err != nil {
		return rules, e.HandleCredentialError(ctx, err)
	}
	return rules, nil
}

func (e *Environ) OpenModelPorts(ctx context.Context, rules firewall.IngressRules) error {
	if err := e.firewaller.OpenModelPorts(ctx, rules); err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	return nil
}

func (e *Environ) CloseModelPorts(ctx context.Context, rules firewall.IngressRules) error {
	if err := e.firewaller.CloseModelPorts(ctx, rules); err != nil {
		return e.HandleCredentialError(ctx, err)
	}
	return nil
}

func (e *Environ) ModelIngressRules(ctx context.Context) (firewall.IngressRules, error) {
	rules, err := e.firewaller.ModelIngressRules(ctx)
	if err != nil {
		return rules, e.HandleCredentialError(ctx, err)
	}
	return rules, nil
}

func (e *Environ) Provider() environs.EnvironProvider {
	return providerInstance
}

func (e *Environ) terminateInstances(ctx context.Context, ids []instance.Id) error {
	if len(ids) == 0 {
		return nil
	}

	novaClient := e.nova()

	// If in instance firewall mode, gather the security group names.
	securityGroupNames, err := e.firewaller.GetSecurityGroups(ctx, ids...)
	if err == environs.ErrNoInstances {
		return nil
	}
	if err != nil {
		logger.Debugf(ctx, "error retrieving security groups for %v: %v", ids, err)
		if denied, _ := e.MaybeInvalidateCredentialError(ctx, err); denied {
			// We'll likely fail all subsequent calls if we have an invalid credential.
			return errors.Trace(err)
		}
	}

	// Record the first error we encounter, as that's the one we're currently
	// reporting to the user.
	var firstErr error
	for _, id := range ids {
		// Attempt to destroy the ports that could have been created when using
		// spaces.
		if err := e.terminateInstanceNetworkPorts(id); err != nil {
			logger.Errorf(ctx, "error attempting to remove ports associated with instance %q: %v", id, err)
			// Unfortunately there is nothing we can do here, there could be
			// orphan ports left.
		}

		// Once ports have been deleted, attempt to delete the server.
		err = novaClient.DeleteServer(string(id))
		if isNotFoundError(err) {
			continue
		}
		if err != nil {
			logger.Debugf(ctx, "error terminating instance %q: %v", id, err)
			if firstErr == nil {
				firstErr = err
			}
			if denied, _ := e.MaybeInvalidateCredentialError(ctx, err); denied {
				// We'll likely fail all subsequent calls if we have an invalid credential.
				return errors.Trace(err)
			}
		}
	}

	if len(securityGroupNames) > 0 {
		logger.Tracef(ctx, "deleting security groups %v", securityGroupNames)
		if err := e.firewaller.DeleteGroups(ctx, securityGroupNames...); err != nil {
			return err
		}
	}

	return firstErr
}

func (e *Environ) terminateInstanceNetworkPorts(id instance.Id) error {
	novaClient := e.nova()
	osInterfaces, err := novaClient.ListOSInterfaces(string(id))
	if err != nil {
		return errors.Trace(err)
	}

	client := e.neutron()
	ports, err := client.ListPortsV2()
	if err != nil {
		return errors.Trace(err)
	}

	// Unfortunately we're unable to bulk delete these ports, so we have to go
	// over them, one by one.
	changes := set.NewStrings()
	for _, port := range ports {
		if !strings.HasPrefix(port.Name, fmt.Sprintf("juju-%s", e.modelUUID)) {
			continue
		}
		changes.Add(port.Id)
	}

	for _, osInterface := range osInterfaces {
		if osInterface.PortID == "" {
			continue
		}

		// Ensure we created the port by first checking the name.
		port, err := client.PortByIdV2(osInterface.PortID)
		if err != nil || !strings.HasPrefix(port.Name, "juju-") {
			continue
		}

		changes.Add(osInterface.PortID)
	}

	var errs []error
	for _, change := range changes.SortedValues() {
		// Delete a port. If we encounter an error add it to the list of errors
		// and continue until we've exhausted all the ports to delete.
		if err := client.DeletePortV2(change); err != nil {
			errs = append(errs, err)
			continue
		}
	}

	if len(errs) > 0 {
		return errors.Errorf("error terminating network ports: %v", errs)
	}

	return nil
}

// AgentMetadataLookupParams returns parameters which are used to query agent simple-streams metadata.
func (e *Environ) AgentMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	base := config.PreferredBase(e.ecfg())
	return e.metadataLookupParams(region, base.OS)
}

// ImageMetadataLookupParams returns parameters which are used to query image simple-streams metadata.
func (e *Environ) ImageMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	base := config.PreferredBase(e.ecfg())
	return e.metadataLookupParams(region, base.Channel.Track)
}

// MetadataLookupParams returns parameters which are used to query simple-streams metadata.
func (e *Environ) metadataLookupParams(region, release string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		region = e.cloud().Region
	}
	cloudSpec, err := e.cloudSpec(region)
	if err != nil {
		return nil, err
	}
	return &simplestreams.MetadataLookupParams{
		Release:  release,
		Region:   cloudSpec.Region,
		Endpoint: cloudSpec.Endpoint,
	}, nil
}

// Region is specified in the HasRegion interface.
func (e *Environ) Region() (simplestreams.CloudSpec, error) {
	return e.cloudSpec(e.cloud().Region)
}

func (e *Environ) cloudSpec(region string) (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   region,
		Endpoint: e.cloud().Endpoint,
	}, nil
}

// TagInstance implements environs.InstanceTagger.
func (e *Environ) TagInstance(ctx context.Context, id instance.Id, tags map[string]string) error {
	if err := e.nova().SetServerMetadata(string(id), tags); err != nil {
		return errors.Annotate(e.HandleCredentialError(ctx, err), "setting server metadata")
	}
	return nil
}

func (e *Environ) SetClock(clock clock.Clock) {
	e.clock = clock
}

func validateCloudSpec(spec environscloudspec.CloudSpec) error {
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
func (e *Environ) Subnets(ctx context.Context, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	subnets, err := e.networking.Subnets(subnetIds)
	if err != nil {
		return subnets, e.HandleCredentialError(ctx, err)
	}
	return subnets, nil
}

// NetworkInterfaces is specified on environs.Networking.
func (e *Environ) NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]network.InterfaceInfos, error) {
	infos, err := e.networking.NetworkInterfaces(ids)
	if err != nil {
		return infos, e.HandleCredentialError(ctx, err)
	}

	return infos, nil
}

// SupportsSpaces is specified on environs.Networking.
func (e *Environ) SupportsSpaces() (bool, error) {
	return true, nil
}

// SupportsRulesWithIPV6CIDRs returns true if the environment supports ingress
// rules containing IPV6 CIDRs. It is part of the FirewallFeatureQuerier
// interface.
func (e *Environ) SupportsRulesWithIPV6CIDRs(ctx context.Context) (bool, error) {
	return true, nil
}

// ValidateCloudEndpoint returns nil if the current model can talk to the openstack
// endpoint. Used as validation during model upgrades.
// Implements environs.CloudEndpointChecker
func (env *Environ) ValidateCloudEndpoint(ctx context.Context) error {
	err := env.Provider().Ping(ctx, env.cloud().Endpoint)
	return errors.Trace(err)
}
