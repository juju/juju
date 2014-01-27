// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"launchpad.net/goose/client"
	gooseerrors "launchpad.net/goose/errors"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.provider.openstack")

type environProvider struct{}

var _ environs.EnvironProvider = (*environProvider)(nil)

var providerInstance environProvider

// Use shortAttempt to poll for short-term events.
// TODO: This was kept to a long timeout because Nova needs more time than EC2.
// For example, HP Cloud takes around 9.1 seconds (10 samples) to return a
// BUILD(spawning) status. But storage delays are handled separately now, and
// perhaps other polling attempts can time out faster.
var shortAttempt = utils.AttemptStrategy{
	Total: 15 * time.Second,
	Delay: 200 * time.Millisecond,
}

func init() {
	environs.RegisterProvider("openstack", environProvider{})
}

func (p environProvider) BoilerplateConfig() string {
	return `
# https://juju.ubuntu.com/docs/config-openstack.html
openstack:
    type: openstack
    # use-floating-ip specifies whether a floating IP address is required
    # to give the nodes a public IP address. Some installations assign public IP
    # addresses by default without requiring a floating IP address.
    # use-floating-ip: false

    # use-default-secgroup specifies whether new machine instances should have the "default"
    # Openstack security group assigned.
    # use-default-secgroup: false

    # network specifies the network label or uuid to bring machines up on, in
    # the case where multiple networks exist. It may be omitted otherwise.
    # network: <your network label or uuid>

    # tools-metadata-url specifies the location of the Juju tools and metadata. It defaults to the
    # global public tools metadata location https://streams.canonical.com/tools.
    # tools-metadata-url:  https://you-tools-metadata-url

    # image-metadata-url specifies the location of Ubuntu cloud image metadata. It defaults to the
    # global public image metadata location https://cloud-images.ubuntu.com/releases.
    # image-metadata-url:  https://you-tools-metadata-url

    # auth-url defaults to the value of the environment variable OS_AUTH_URL,
    # but can be specified here.
    # auth-url: https://yourkeystoneurl:443/v2.0/

    # tenant-name holds the openstack tenant name. It defaults to
    # the environment variable OS_TENANT_NAME.
    # tenant-name: <your tenant name>

    # region holds the openstack region.  It defaults to
    # the environment variable OS_REGION_NAME.
    # region: <your region>

    # The auth-mode, username and password attributes
    # are used for userpass authentication (the default).

    # auth-mode holds the authentication mode. For user-password
    # authentication, auth-mode should be "userpass" and username
    # and password should be set appropriately; they default to
    # the environment variables OS_USERNAME and OS_PASSWORD
     # respectively.
    # auth-mode: userpass
    # username: <your username>
    # password: <secret>
     
    # For key-pair authentication, auth-mode should  be "keypair"
    # and access-key and secret-key should be  set appropriately; they default to
    # the environment variables OS_ACCESS_KEY and OS_SECRET_KEY
    # respectively.
    # auth-mode: keypair
    # access-key: <secret>
    # secret-key: <secret>

# https://juju.ubuntu.com/docs/config-hpcloud.html
hpcloud:
    type: openstack
    
    # use-floating-ip specifies whether a floating IP address is required
    # to give the nodes a public IP address. Some installations assign public IP
    # addresses by default without requiring a floating IP address.
    # use-floating-ip: false

    # use-default-secgroup specifies whether new machine instances should have the "default"
    # Openstack security group assigned.
    # use-default-secgroup: false

    # tenant-name holds the openstack tenant name. In HPCloud, this is 
    # synonymous with the project-name  It defaults to
    # the environment variable OS_TENANT_NAME.
    # tenant-name: <your tenant name>
    
    # auth-url holds the keystone url for authentication. 
    # It defaults to the value of the environment variable OS_AUTH_URL.
    # auth-url: https://region-a.geo-1.identity.hpcloudsvc.com:35357/v2.0/

    # region holds the HP Cloud region (e.g. az-1.region-a.geo-1).  
    # It defaults to the environment variable OS_REGION_NAME.
    # region: <your region>
    
    # auth-mode holds the authentication mode. For user-password
    # authentication, auth-mode should be "userpass" and username
    # and password should be set appropriately; they default to
    # the environment variables OS_USERNAME and OS_PASSWORD
    # respectively.
    # auth-mode: userpass
    # username: <your_username>
    # password: <your_password>
    
    # For key-pair authentication, auth-mode should  be "keypair"
    # and access-key and secret-key should be  set appropriately; they default to
    # the environment variables OS_ACCESS_KEY and OS_SECRET_KEY
    # respectively.
    # auth-mode: keypair
    # access-key: <secret>
    # secret-key: <secret>
`[1:]
}

func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening environment %q", cfg.Name())
	e := new(environ)
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	e.name = cfg.Name()
	return e, nil
}

func (p environProvider) Prepare(cfg *config.Config) (environs.Environ, error) {
	attrs := cfg.UnknownAttrs()
	if _, ok := attrs["control-bucket"]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, err
		}
		attrs["control-bucket"] = fmt.Sprintf("%x", uuid.Raw())
	}
	cfg, err := cfg.Apply(attrs)
	if err != nil {
		return nil, err
	}
	return p.Open(cfg)
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p environProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, fmt.Errorf("region must be specified")
	}
	return &simplestreams.MetadataLookupParams{
		Region:        region,
		Architectures: []string{"amd64", "arm"},
	}, nil
}

func (p environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	m := make(map[string]string)
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	m["username"] = ecfg.username()
	m["password"] = ecfg.password()
	m["tenant-name"] = ecfg.tenantName()
	return m, nil
}

func (p environProvider) PublicAddress() (string, error) {
	if addr, err := fetchMetadata("public-ipv4"); err != nil {
		return "", err
	} else if addr != "" {
		return addr, nil
	}
	return p.PrivateAddress()
}

func (p environProvider) PrivateAddress() (string, error) {
	return fetchMetadata("local-ipv4")
}

// metadataHost holds the address of the instance metadata service.
// It is a variable so that tests can change it to refer to a local
// server when needed.
var metadataHost = "http://169.254.169.254"

// fetchMetadata fetches a single atom of data from the openstack instance metadata service.
// http://docs.amazonwebservices.com/AWSEC2/latest/UserGuide/AESDG-chapter-instancedata.html
// (the same specs is implemented in ec2, hence the reference)
func fetchMetadata(name string) (value string, err error) {
	uri := fmt.Sprintf("%s/latest/meta-data/%s", metadataHost, name)
	data, err := retryGet(uri)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
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

type environ struct {
	name string

	ecfgMutex       sync.Mutex
	imageBaseMutex  sync.Mutex
	toolsBaseMutex  sync.Mutex
	ecfgUnlocked    *environConfig
	client          client.AuthenticatingClient
	novaUnlocked    *nova.Client
	storageUnlocked storage.Storage
	// An ordered list of sources in which to find the simplestreams index files used to
	// look up image ids.
	imageSources []simplestreams.DataSource
	// An ordered list of paths in which to find the simplestreams index files used to
	// look up tools ids.
	toolsSources []simplestreams.DataSource
}

var _ environs.Environ = (*environ)(nil)
var _ imagemetadata.SupportsCustomSources = (*environ)(nil)
var _ envtools.SupportsCustomSources = (*environ)(nil)
var _ simplestreams.HasRegion = (*environ)(nil)

type openstackInstance struct {
	e        *environ
	instType *instances.InstanceType
	arch     *string

	mu           sync.Mutex
	serverDetail *nova.ServerDetail
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
	return hc
}

// getAddress returns the existing server information on addresses,
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

// Addresses implements instance.Addresses() returning generic address
// details for the instances, and calling the openstack api if needed.
func (inst *openstackInstance) Addresses() ([]instance.Address, error) {
	addresses, err := inst.getAddresses()
	if err != nil {
		return nil, err
	}
	return convertNovaAddresses(addresses), nil
}

// convertNovaAddresses returns nova addresses in generic format
func convertNovaAddresses(addresses map[string][]nova.IPAddress) []instance.Address {
	// TODO(gz) Network ordering may be significant but is not preserved by
	// the map, see lp:1188126 for example. That could potentially be fixed
	// in goose, or left to be derived by other means.
	var machineAddresses []instance.Address
	for network, ips := range addresses {
		networkscope := instance.NetworkUnknown
		// For canonistack and hpcloud, public floating addresses may
		// be put in networks named something other than public. Rely
		// on address sanity logic to catch and mark them corectly.
		if network == "public" {
			networkscope = instance.NetworkPublic
		}
		for _, address := range ips {
			// Assume ipv4 unless specified otherwise
			addrtype := instance.Ipv4Address
			if address.Version == 6 {
				addrtype = instance.Ipv6Address
			}
			// TODO(gz): Use NewAddress... with sanity checking
			machineAddr := instance.Address{
				Value:        address.Address,
				Type:         addrtype,
				NetworkName:  network,
				NetworkScope: networkscope,
			}
			machineAddresses = append(machineAddresses, machineAddr)
		}
	}
	return machineAddresses
}

func (inst *openstackInstance) DNSName() (string, error) {
	addresses, err := inst.Addresses()
	if err != nil {
		return "", err
	}
	addr := instance.SelectPublicAddress(addresses)
	if addr == "" {
		return "", instance.ErrNoDNSName
	}
	return addr, nil
}

func (inst *openstackInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(inst)
}

// TODO: following 30 lines nearly verbatim from environs/ec2

func (inst *openstackInstance) OpenPorts(machineId string, ports []instance.Port) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.openPortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("opened ports in security group %s: %v", name, ports)
	return nil
}

func (inst *openstackInstance) ClosePorts(machineId string, ports []instance.Port) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.closePortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("closed ports in security group %s: %v", name, ports)
	return nil
}

func (inst *openstackInstance) Ports(machineId string) ([]instance.Port, error) {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from instance",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	return inst.e.portsInGroup(name)
}

func (e *environ) ecfg() *environConfig {
	e.ecfgMutex.Lock()
	ecfg := e.ecfgUnlocked
	e.ecfgMutex.Unlock()
	return ecfg
}

func (e *environ) nova() *nova.Client {
	e.ecfgMutex.Lock()
	nova := e.novaUnlocked
	e.ecfgMutex.Unlock()
	return nova
}

// PrecheckInstance is specified in the environs.Prechecker interface.
func (*environ) PrecheckInstance(series string, cons constraints.Value) error {
	return nil
}

// PrecheckContainer is specified in the environs.Prechecker interface.
func (*environ) PrecheckContainer(series string, kind instance.ContainerType) error {
	// This check can either go away or be relaxed when the openstack
	// provider manages container addressibility.
	return environs.NewContainersUnsupported("openstack provider does not support containers")
}

func (e *environ) Name() string {
	return e.name
}

func (e *environ) Storage() storage.Storage {
	e.ecfgMutex.Lock()
	stor := e.storageUnlocked
	e.ecfgMutex.Unlock()
	return stor
}

func (e *environ) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	// The client's authentication may have been reset when finding tools if the agent-version
	// attribute was updated so we need to re-authenticate. This will be a no-op if already authenticated.
	// An authenticated client is needed for the URL() call below.
	err := e.client.Authenticate()
	if err != nil {
		return err
	}
	return common.Bootstrap(ctx, e, cons)
}

func (e *environ) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(e)
}

func (e *environ) Config() *config.Config {
	return e.ecfg().Config
}

func (e *environ) authClient(ecfg *environConfig, authModeCfg AuthMode) client.AuthenticatingClient {
	cred := &identity.Credentials{
		User:       ecfg.username(),
		Secrets:    ecfg.password(),
		Region:     ecfg.region(),
		TenantName: ecfg.tenantName(),
		URL:        ecfg.authURL(),
	}
	// authModeCfg has already been validated so we know it's one of the values below.
	var authMode identity.AuthMode
	switch authModeCfg {
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
	return newClient(cred, authMode, nil)
}

func (e *environ) SetConfig(cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}
	// At this point, the authentication method config value has been validated so we extract it's value here
	// to avoid having to validate again each time when creating the OpenStack client.
	var authModeCfg AuthMode
	e.ecfgMutex.Lock()
	defer e.ecfgMutex.Unlock()
	authModeCfg = AuthMode(ecfg.authMode())
	e.ecfgUnlocked = ecfg

	e.client = e.authClient(ecfg, authModeCfg)
	e.novaUnlocked = nova.New(e.client)

	// create new control storage instance, existing instances continue
	// to reference their existing configuration.
	// public storage instance creation is deferred until needed since authenticated
	// access to the identity service is required so that any juju-tools endpoint can be used.
	e.storageUnlocked = &openstackstorage{
		containerName: ecfg.controlBucket(),
		// this is possibly just a hack - if the ACL is swift.Private,
		// the machine won't be able to get the tools (401 error)
		containerACL: swift.PublicRead,
		swift:        swift.New(e.client)}
	return nil
}

// GetImageSources returns a list of sources which are used to search for simplestreams image metadata.
func (e *environ) GetImageSources() ([]simplestreams.DataSource, error) {
	e.imageBaseMutex.Lock()
	defer e.imageBaseMutex.Unlock()

	if e.imageSources != nil {
		return e.imageSources, nil
	}
	if !e.client.IsAuthenticated() {
		err := e.client.Authenticate()
		if err != nil {
			return nil, err
		}
	}
	// Add the simplestreams source off the control bucket.
	e.imageSources = append(e.imageSources, storage.NewStorageSimpleStreamsDataSource(
		e.Storage(), storage.BaseImagesPath))
	// Add the simplestreams base URL from keystone if it is defined.
	productStreamsURL, err := e.client.MakeServiceURL("product-streams", nil)
	if err == nil {
		verify := simplestreams.VerifySSLHostnames
		if !e.Config().SSLHostnameVerification() {
			verify = simplestreams.NoVerifySSLHostnames
		}
		source := simplestreams.NewURLDataSource(productStreamsURL, verify)
		e.imageSources = append(e.imageSources, source)
	}
	return e.imageSources, nil
}

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *environ) GetToolsSources() ([]simplestreams.DataSource, error) {
	e.toolsBaseMutex.Lock()
	defer e.toolsBaseMutex.Unlock()

	if e.toolsSources != nil {
		return e.toolsSources, nil
	}
	if !e.client.IsAuthenticated() {
		err := e.client.Authenticate()
		if err != nil {
			return nil, err
		}
	}
	verify := simplestreams.VerifySSLHostnames
	if !e.Config().SSLHostnameVerification() {
		verify = simplestreams.NoVerifySSLHostnames
	}
	// Add the simplestreams source off the control bucket.
	e.toolsSources = append(e.toolsSources, storage.NewStorageSimpleStreamsDataSource(e.Storage(), storage.BaseToolsPath))
	// Add the simplestreams base URL from keystone if it is defined.
	toolsURL, err := e.client.MakeServiceURL("juju-tools", nil)
	if err == nil {
		source := simplestreams.NewURLDataSource(toolsURL, verify)
		e.toolsSources = append(e.toolsSources, source)
	}
	return e.toolsSources, nil
}

// TODO(gz): Move this somewhere more reusable
const uuidPattern = "^([a-fA-F0-9]{8})-([a-fA-f0-9]{4})-([1-5][a-fA-f0-9]{3})-([a-fA-f0-9]{4})-([a-fA-f0-9]{12})$"

var uuidRegexp = regexp.MustCompile(uuidPattern)

// resolveNetwork takes either a network id or label and returns a network id
func (e *environ) resolveNetwork(networkName string) (string, error) {
	if uuidRegexp.MatchString(networkName) {
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
func (e *environ) allocatePublicIP() (*nova.FloatingIP, error) {
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
	}
	return newfip, nil
}

// assignPublicIP tries to assign the given floating IP address to the
// specified server, or returns an error.
func (e *environ) assignPublicIP(fip *nova.FloatingIP, serverId string) (err error) {
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

// StartInstance is specified in the InstanceBroker interface.
func (e *environ) StartInstance(cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {

	series := possibleTools.OneSeries()
	arches := possibleTools.Arches()
	spec, err := findInstanceSpec(e, &instances.InstanceConstraint{
		Region:      e.ecfg().region(),
		Series:      series,
		Arches:      arches,
		Constraints: cons,
	})
	if err != nil {
		return nil, nil, err
	}
	tools, err := possibleTools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, nil, fmt.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	machineConfig.Tools = tools[0]

	if err := environs.FinishMachineConfig(machineConfig, e.Config(), cons); err != nil {
		return nil, nil, err
	}
	userData, err := environs.ComposeUserData(machineConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot make user data: %v", err)
	}
	logger.Debugf("openstack user data; %d bytes", len(userData))
	var networks = []nova.ServerNetworks{}
	usingNetwork := e.ecfg().network()
	if usingNetwork != "" {
		networkId, err := e.resolveNetwork(usingNetwork)
		if err != nil {
			return nil, nil, err
		}
		logger.Debugf("using network id %q", networkId)
		networks = append(networks, nova.ServerNetworks{NetworkId: networkId})
	}
	withPublicIP := e.ecfg().useFloatingIP()
	var publicIP *nova.FloatingIP
	if withPublicIP {
		if fip, err := e.allocatePublicIP(); err != nil {
			return nil, nil, fmt.Errorf("cannot allocate a public IP as needed: %v", err)
		} else {
			publicIP = fip
			logger.Infof("allocated public IP %s", publicIP.IP)
		}
	}
	cfg := e.Config()
	groups, err := e.setUpGroups(machineConfig.MachineId, cfg.StatePort(), cfg.APIPort())
	if err != nil {
		return nil, nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	var groupNames = make([]nova.SecurityGroupName, len(groups))
	for i, g := range groups {
		groupNames[i] = nova.SecurityGroupName{g.Name}
	}
	var opts = nova.RunServerOpts{
		Name:               e.machineFullName(machineConfig.MachineId),
		FlavorId:           spec.InstanceType.Id,
		ImageId:            spec.Image.Id,
		UserData:           userData,
		SecurityGroupNames: groupNames,
		Networks:           networks,
	}
	var server *nova.Entity
	for a := shortAttempt.Start(); a.Next(); {
		server, err = e.nova().RunServer(opts)
		if err == nil || !gooseerrors.IsNotFound(err) {
			break
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot run instance: %v", err)
	}
	detail, err := e.nova().GetServer(server.Id)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get started instance: %v", err)
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
			return nil, nil, fmt.Errorf("cannot assign public address %s to instance %q: %v", publicIP.IP, inst.Id(), err)
		}
		logger.Infof("assigned public IP %s to %q", publicIP.IP, inst.Id())
	}
	return inst, inst.hardwareCharacteristics(), nil
}

func (e *environ) StopInstances(insts []instance.Instance) error {
	ids := make([]instance.Id, len(insts))
	for i, inst := range insts {
		instanceValue, ok := inst.(*openstackInstance)
		if !ok {
			return errors.New("Incompatible instance.Instance supplied")
		}
		ids[i] = instanceValue.Id()
	}
	logger.Debugf("terminating instances %v", ids)
	return e.terminateInstances(ids)
}

// collectInstances tries to get information on each instance id in ids.
// It fills the slots in the given map for known servers with status
// either ACTIVE or BUILD. Returns a list of missing ids.
func (e *environ) collectInstances(ids []instance.Id, out map[instance.Id]instance.Instance) []instance.Id {
	var err error
	serversById := make(map[string]nova.ServerDetail)
	if len(ids) == 1 {
		// most common case - single instance
		var server *nova.ServerDetail
		server, err = e.nova().GetServer(string(ids[0]))
		if server != nil {
			serversById[server.Id] = *server
		}
	} else {
		var servers []nova.ServerDetail
		servers, err = e.nova().ListServersDetail(e.machinesFilter())
		for _, server := range servers {
			serversById[server.Id] = server
		}
	}
	if err != nil {
		return ids
	}
	var missing []instance.Id
	for _, id := range ids {
		if server, found := serversById[string(id)]; found {
			// HPCloud uses "BUILD(spawning)" as an intermediate BUILD states once networking is available.
			switch server.Status {
			case nova.StatusActive, nova.StatusBuild, nova.StatusBuildSpawning:
				// TODO(wallyworld): lookup the flavor details to fill in the instance type data
				out[id] = &openstackInstance{e: e, serverDetail: &server}
				continue
			}
		}
		missing = append(missing, id)
	}
	return missing
}

func (e *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	missing := ids
	found := make(map[instance.Id]instance.Instance)
	// Make a series of requests to cope with eventual consistency.
	// Each request will attempt to add more instances to the requested
	// set.
	for a := shortAttempt.Start(); a.Next(); {
		if missing = e.collectInstances(missing, found); len(missing) == 0 {
			break
		}
	}
	if len(found) == 0 {
		return nil, environs.ErrNoInstances
	}
	insts := make([]instance.Instance, len(ids))
	var err error
	for i, id := range ids {
		if inst := found[id]; inst != nil {
			insts[i] = inst
		} else {
			err = environs.ErrPartialInstances
		}
	}
	return insts, err
}

func (e *environ) AllInstances() (insts []instance.Instance, err error) {
	servers, err := e.nova().ListServersDetail(e.machinesFilter())
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		if server.Status == nova.StatusActive || server.Status == nova.StatusBuild {
			var s = server
			// TODO(wallyworld): lookup the flavor details to fill in the instance type data
			insts = append(insts, &openstackInstance{
				e:            e,
				serverDetail: &s,
			})
		}
	}
	return insts, err
}

func (e *environ) Destroy() error {
	return common.Destroy(e)
}

func (e *environ) globalGroupName() string {
	return fmt.Sprintf("%s-global", e.jujuGroupName())
}

func (e *environ) machineGroupName(machineId string) string {
	return fmt.Sprintf("%s-%s", e.jujuGroupName(), machineId)
}

func (e *environ) jujuGroupName() string {
	return fmt.Sprintf("juju-%s", e.name)
}

func (e *environ) machineFullName(machineId string) string {
	return fmt.Sprintf("juju-%s-%s", e.Name(), names.MachineTag(machineId))
}

// machinesFilter returns a nova.Filter matching all machines in the environment.
func (e *environ) machinesFilter() *nova.Filter {
	filter := nova.NewFilter()
	filter.Set(nova.FilterServer, fmt.Sprintf("juju-%s-machine-\\d*", e.Name()))
	return filter
}

func (e *environ) openPortsInGroup(name string, ports []instance.Port) error {
	novaclient := e.nova()
	group, err := novaclient.SecurityGroupByName(name)
	if err != nil {
		return err
	}
	for _, port := range ports {
		_, err := novaclient.CreateSecurityGroupRule(nova.RuleInfo{
			ParentGroupId: group.Id,
			FromPort:      port.Number,
			ToPort:        port.Number,
			IPProtocol:    port.Protocol,
			Cidr:          "0.0.0.0/0",
		})
		if err != nil {
			// TODO: if err is not rule already exists, raise?
			logger.Debugf("error creating security group rule: %v", err.Error())
		}
	}
	return nil
}

func (e *environ) closePortsInGroup(name string, ports []instance.Port) error {
	if len(ports) == 0 {
		return nil
	}
	novaclient := e.nova()
	group, err := novaclient.SecurityGroupByName(name)
	if err != nil {
		return err
	}
	// TODO: Hey look ma, it's quadratic
	for _, port := range ports {
		for _, p := range (*group).Rules {
			if p.IPProtocol == nil || *p.IPProtocol != port.Protocol ||
				p.FromPort == nil || *p.FromPort != port.Number ||
				p.ToPort == nil || *p.ToPort != port.Number {
				continue
			}
			err := novaclient.DeleteSecurityGroupRule(p.Id)
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (e *environ) portsInGroup(name string) (ports []instance.Port, err error) {
	group, err := e.nova().SecurityGroupByName(name)
	if err != nil {
		return nil, err
	}
	for _, p := range (*group).Rules {
		for i := *p.FromPort; i <= *p.ToPort; i++ {
			ports = append(ports, instance.Port{
				Protocol: *p.IPProtocol,
				Number:   i,
			})
		}
	}
	instance.SortPorts(ports)
	return ports, nil
}

// TODO: following 30 lines nearly verbatim from environs/ec2

func (e *environ) OpenPorts(ports []instance.Port) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on environment",
			e.Config().FirewallMode())
	}
	if err := e.openPortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("opened ports in global group: %v", ports)
	return nil
}

func (e *environ) ClosePorts(ports []instance.Port) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on environment",
			e.Config().FirewallMode())
	}
	if err := e.closePortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("closed ports in global group: %v", ports)
	return nil
}

func (e *environ) Ports() ([]instance.Port, error) {
	if e.Config().FirewallMode() != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from environment",
			e.Config().FirewallMode())
	}
	return e.portsInGroup(e.globalGroupName())
}

func (e *environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

func (e *environ) setUpGlobalGroup(groupName string, statePort, apiPort int) (nova.SecurityGroup, error) {
	return e.ensureGroup(groupName,
		[]nova.RuleInfo{
			{
				IPProtocol: "tcp",
				FromPort:   22,
				ToPort:     22,
				Cidr:       "0.0.0.0/0",
			},
			{
				IPProtocol: "tcp",
				FromPort:   statePort,
				ToPort:     statePort,
				Cidr:       "0.0.0.0/0",
			},
			{
				IPProtocol: "tcp",
				FromPort:   apiPort,
				ToPort:     apiPort,
				Cidr:       "0.0.0.0/0",
			},
			{
				IPProtocol: "tcp",
				FromPort:   1,
				ToPort:     65535,
			},
			{
				IPProtocol: "udp",
				FromPort:   1,
				ToPort:     65535,
			},
			{
				IPProtocol: "icmp",
				FromPort:   -1,
				ToPort:     -1,
			},
		})
}

// setUpGroups creates the security groups for the new machine, and
// returns them.
//
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same OpenStack account.
// In addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
//
// Note: ideally we'd have a better way to determine group membership so that 2
// people that happen to share an openstack account and name their environment
// "openstack" don't end up destroying each other's machines.
func (e *environ) setUpGroups(machineId string, statePort, apiPort int) ([]nova.SecurityGroup, error) {
	jujuGroup, err := e.setUpGlobalGroup(e.jujuGroupName(), statePort, apiPort)
	if err != nil {
		return nil, err
	}
	var machineGroup nova.SecurityGroup
	switch e.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = e.ensureGroup(e.machineGroupName(machineId), nil)
	case config.FwGlobal:
		machineGroup, err = e.ensureGroup(e.globalGroupName(), nil)
	}
	if err != nil {
		return nil, err
	}
	groups := []nova.SecurityGroup{jujuGroup, machineGroup}
	if e.ecfg().useDefaultSecurityGroup() {
		defaultGroup, err := e.nova().SecurityGroupByName("default")
		if err != nil {
			return nil, fmt.Errorf("loading default security group: %v", err)
		}
		groups = append(groups, *defaultGroup)
	}
	return groups, nil
}

// zeroGroup holds the zero security group.
var zeroGroup nova.SecurityGroup

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
func (e *environ) ensureGroup(name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
	novaClient := e.nova()
	// First attempt to look up an existing group by name.
	group, err := novaClient.SecurityGroupByName(name)
	if err == nil {
		// Group exists, so assume it is correctly set up and return it.
		// TODO(jam): 2013-09-18 http://pad.lv/121795
		// We really should verify the group is set up correctly,
		// because deleting and re-creating environments can get us bad
		// groups (especially if they were set up under Python)
		return *group, nil
	}
	// Doesn't exist, so try and create it.
	group, err = novaClient.CreateSecurityGroup(name, "juju group")
	if err != nil {
		if !gooseerrors.IsDuplicateValue(err) {
			return zeroGroup, err
		} else {
			// We just tried to create a duplicate group, so load the existing group.
			group, err = novaClient.SecurityGroupByName(name)
			if err != nil {
				return zeroGroup, err
			}
			return *group, nil
		}
	}
	// The new group is created so now add the rules.
	group.Rules = make([]nova.SecurityGroupRule, len(rules))
	for i, rule := range rules {
		rule.ParentGroupId = group.Id
		if rule.Cidr == "" {
			// http://pad.lv/1226996 Rules that don't have a CIDR
			// are meant to apply only to this group. If you don't
			// supply CIDR or GroupId then openstack assumes you
			// mean CIDR=0.0.0.0/0
			rule.GroupId = &group.Id
		}
		groupRule, err := novaClient.CreateSecurityGroupRule(rule)
		if err != nil && !gooseerrors.IsDuplicateValue(err) {
			return zeroGroup, err
		}
		group.Rules[i] = *groupRule
	}
	return *group, nil
}

func (e *environ) terminateInstances(ids []instance.Id) error {
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
func (e *environ) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		region = e.ecfg().region()
	}
	return &simplestreams.MetadataLookupParams{
		Series:        e.ecfg().DefaultSeries(),
		Region:        region,
		Endpoint:      e.ecfg().authURL(),
		Architectures: []string{"amd64", "arm"},
	}, nil
}

// Region is specified in the HasRegion interface.
func (e *environ) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   e.ecfg().region(),
		Endpoint: e.ecfg().authURL(),
	}, nil
}
