// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goose/client"
	gooseerrors "launchpad.net/goose/errors"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/swift"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
	"net/http"
	"strings"
	"sync"
	"time"
)

type environProvider struct{}

var _ environs.EnvironProvider = (*environProvider)(nil)

var providerInstance environProvider

// Use shortAttempt to poll for short-term events.
var shortAttempt = utils.AttemptStrategy{
	Total: 10 * time.Second, // it seems Nova needs more time than EC2
	Delay: 200 * time.Millisecond,
}

func init() {
	environs.RegisterProvider("openstack", environProvider{})
}

func (p environProvider) BoilerplateConfig() string {
	return `
## https://juju.ubuntu.com/get-started/openstack/
openstack:
  type: openstack
  # Specifies whether the use of a floating IP address is required to give the nodes
  # a public IP address. Some installations assign public IP addresses by default without
  # requiring a floating IP address.
  # use-floating-ip: false
  admin-secret: {{rand}}
  # Globally unique swift bucket name
  control-bucket: juju-{{rand}}
  # Usually set via the env variable OS_AUTH_URL, but can be specified here
  # auth-url: https://yourkeystoneurl:443/v2.0/
  # override if your workstation is running a different series to which you are deploying
  # default-series: precise
  # The following are used for userpass authentication (the default)
  auth-mode: userpass
  # Usually set via the env variable OS_USERNAME, but can be specified here
  # username: <your username>
  # Usually set via the env variable OS_PASSWORD, but can be specified here
  # password: <secret>
  # Usually set via the env variable OS_TENANT_NAME, but can be specified here
  # tenant-name: <your tenant name>
  # Usually set via the env variable OS_REGION_NAME, but can be specified here
  # region: <your region>

## https://juju.ubuntu.com/get-started/hp-cloud/
hpcloud:
  type: openstack
  # Specifies whether the use of a floating IP address is required to give the nodes
  # a public IP address. Some installations assign public IP addresses by default without
  # requiring a floating IP address.
  use-floating-ip: false
  admin-secret: {{rand}}
  # Globally unique swift bucket name
  control-bucket: juju-{{rand}}
  # Not required if env variable OS_AUTH_URL is set
  auth-url: https://yourkeystoneurl:35357/v2.0/
  # URL denoting a public container holding the juju tools.
  public-bucket-url: https://region-a.geo-1.objects.hpcloudsvc.com/v1/60502529753910
  # override if your workstation is running a different series to which you are deploying
  # default-series: precise
  # The following are used for userpass authentication (the default)
  auth-mode: userpass
  # Usually set via the env variable OS_USERNAME, but can be specified here
  # username: <your username>
  # Usually set via the env variable OS_PASSWORD, but can be specified here
  # password: <secret>
  # Usually set via the env variable OS_TENANT_NAME, but can be specified here
  # tenant-name: <your tenant name>
  # Usually set via the env variable OS_REGION_NAME, but can be specified here
  # region: <your region>
  # The following are used for keypair authentication
  # auth-mode: keypair
  # Usually set via the env variable AWS_ACCESS_KEY_ID, but can be specified here
  # access-key: <secret>
  # Usually set via the env variable AWS_SECRET_ACCESS_KEY, but can be specified here
  # secret-key: <secret>

`[1:]
}

func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	log.Infof("environs/openstack: opening environment %q", cfg.Name())
	e := new(environ)
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	e.name = cfg.Name()
	return e, nil
}

func (p environProvider) SecretAttrs(cfg *config.Config) (map[string]interface{}, error) {
	m := make(map[string]interface{})
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

	ecfgMutex             sync.Mutex
	publicStorageMutex    sync.Mutex
	imageBaseMutex        sync.Mutex
	ecfgUnlocked          *environConfig
	client                client.AuthenticatingClient
	novaUnlocked          *nova.Client
	storageUnlocked       environs.Storage
	publicStorageUnlocked environs.StorageReader // optional.
	// An ordered list of paths in which to find the simplestreams index files used to
	// look up image ids.
	imageBaseURLs []string
}

var _ environs.Environ = (*environ)(nil)

type openstackInstance struct {
	*nova.ServerDetail
	e        *environ
	instType *instances.InstanceType
	arch     *string
	address  string
}

func (inst *openstackInstance) String() string {
	return inst.ServerDetail.Id
}

var _ instance.Instance = (*openstackInstance)(nil)

func (inst *openstackInstance) Id() instance.Id {
	return instance.Id(inst.ServerDetail.Id)
}

func (inst *openstackInstance) hardwareCharacteristics() *instance.HardwareCharacteristics {
	hc := &instance.HardwareCharacteristics{Arch: inst.arch}
	if inst.instType != nil {
		hc.Mem = &inst.instType.Mem
		hc.CpuCores = &inst.instType.CpuCores
		hc.CpuPower = inst.instType.CpuPower
	}
	return hc
}

// instanceAddress processes a map of networks to lists of IP
// addresses, as returned by Nova.GetServer(), extracting the proper
// public (or private, if public is not available) IPv4 address, and
// returning it, or an error.
func instanceAddress(addresses map[string][]nova.IPAddress) (string, error) {
	var private, public, privateNet string
	for network, ips := range addresses {
		for _, address := range ips {
			if address.Version == 4 {
				if network == "public" {
					public = address.Address
				} else {
					privateNet = network
					// Some setups use custom network name, treat as "private"
					private = address.Address
				}
				break
			}
		}
	}
	// HP cloud/canonistack specific: public address is 2nd in the private network
	if prv, ok := addresses[privateNet]; public == "" && ok {
		if len(prv) > 1 && prv[1].Version == 4 {
			public = prv[1].Address
		}
	}
	// Juju assumes it always needs a public address and loops waiting for one.
	// In fact a private address is generally fine provided it can be sshed to.
	// (ported from py-juju/providers/openstack)
	if public == "" && private != "" {
		public = private
	}
	if public == "" {
		return "", instance.ErrNoDNSName
	}
	return public, nil
}

func (inst *openstackInstance) DNSName() (string, error) {
	if inst.address != "" {
		return inst.address, nil
	}
	// Fetch the instance information again, in case
	// the addresses have become available.
	server, err := inst.e.nova().GetServer(string(inst.Id()))
	if err != nil {
		return "", err
	}
	inst.address, err = instanceAddress(server.Addresses)
	if err != nil {
		return "", err
	}
	return inst.address, nil
}

func (inst *openstackInstance) WaitDNSName() (string, error) {
	return environs.WaitDNSName(inst)
}

// TODO: following 30 lines nearly verbatim from environs/ec2

func (inst *openstackInstance) OpenPorts(machineId string, ports []instance.Port) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode for opening ports on instance: %q",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.openPortsInGroup(name, ports); err != nil {
		return err
	}
	log.Infof("environs/openstack: opened ports in security group %s: %v", name, ports)
	return nil
}

func (inst *openstackInstance) ClosePorts(machineId string, ports []instance.Port) error {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode for closing ports on instance: %q",
			inst.e.Config().FirewallMode())
	}
	name := inst.e.machineGroupName(machineId)
	if err := inst.e.closePortsInGroup(name, ports); err != nil {
		return err
	}
	log.Infof("environs/openstack: closed ports in security group %s: %v", name, ports)
	return nil
}

func (inst *openstackInstance) Ports(machineId string) ([]instance.Port, error) {
	if inst.e.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode for retrieving ports from instance: %q",
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

func (e *environ) Name() string {
	return e.name
}

func (e *environ) Storage() environs.Storage {
	e.ecfgMutex.Lock()
	storage := e.storageUnlocked
	e.ecfgMutex.Unlock()
	return storage
}

// publicBucketURL gets the public bucket URL, either from env or keystone catalog.
func (e *environ) publicBucketURL() string {
	ecfg := e.ecfg()
	publicBucketURL := ecfg.publicBucketURL()
	if publicBucketURL == "" {
		// No public bucket in env, so authenticate and look in keystone catalog.
		if !e.client.IsAuthenticated() {
			e.client.Authenticate()
		}
		var err error
		publicBucketURL, err = e.client.MakeServiceURL("juju-tools", nil)
		if err != nil {
			return ""
		}
	}
	return publicBucketURL
}

func (e *environ) PublicStorage() environs.StorageReader {
	e.publicStorageMutex.Lock()
	defer e.publicStorageMutex.Unlock()
	ecfg := e.ecfg()
	// If public storage has already been determined, return that instance.
	publicStorage := e.publicStorageUnlocked
	if publicStorage == nil && ecfg.publicBucket() == "" {
		// If there is no public bucket name, then there can be no public storage.
		e.publicStorageUnlocked = environs.EmptyStorage
		publicStorage = e.publicStorageUnlocked
	}
	if publicStorage != nil {
		return publicStorage
	}
	// If there is a public bucket URL defined, set up a public storage client referencing that URL,
	// otherwise create a new public bucket using the user's credentials on the authenticated client.
	publicBucketURL := e.publicBucketURL()
	if publicBucketURL == "" {
		e.publicStorageUnlocked = &storage{
			containerName: ecfg.publicBucket(),
			// this is possibly just a hack - if the ACL is swift.Private,
			// the machine won't be able to get the tools (401 error)
			containerACL: swift.PublicRead,
			swift:        swift.New(e.client)}
	} else {
		pc := client.NewPublicClient(publicBucketURL, nil)
		e.publicStorageUnlocked = &storage{
			containerName: ecfg.publicBucket(),
			containerACL:  swift.PublicRead,
			swift:         swift.New(pc)}
	}
	publicStorage = e.publicStorageUnlocked
	return publicStorage
}

// TODO(bug 1199847): This work can be shared between providers.
func (e *environ) Bootstrap(cons constraints.Value) error {
	// The bootstrap instance gets machine id "0".  This is not related
	// to instance ids.  Juju assigns the machine ID.
	const machineID = "0"
	log.Infof("environs/openstack: bootstrapping environment %q", e.name)

	possibleTools, err := environs.FindBootstrapTools(e, cons)
	if err != nil {
		return err
	}
	err = environs.CheckToolsSeries(possibleTools, e.Config().DefaultSeries())
	if err != nil {
		return err
	}
	stateFileURL, err := e.Storage().URL(environs.StateFile)
	if err != nil {
		return fmt.Errorf("cannot create bootstrap state file: %v", err)
	}

	machineConfig := environs.NewBootstrapMachineConfig(machineID, stateFileURL)

	// TODO(wallyworld) - save bootstrap machine metadata
	inst, characteristics, err := e.internalStartInstance(cons, possibleTools, machineConfig, e.ecfg().useFloatingIP())
	if err != nil {
		return fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	err = environs.SaveState(e.Storage(), &environs.BootstrapState{
		StateInstances:  []instance.Id{inst.Id()},
		Characteristics: []instance.HardwareCharacteristics{*characteristics},
	})
	if err != nil {
		// ignore error on StopInstance because the previous error is
		// more important.
		e.StopInstances([]instance.Instance{inst})
		return fmt.Errorf("cannot save state: %v", err)
	}
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to use Swift to make sure that only one succeeds.
	// Perhaps consider using SimpleDB for state storage
	// which would enable that possibility.

	return nil
}

func (e *environ) StateInfo() (*state.Info, *api.Info, error) {
	return environs.StateInfo(e)
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
	return client.NewClient(cred, authMode, nil)
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
	e.storageUnlocked = &storage{
		containerName: ecfg.controlBucket(),
		// this is possibly just a hack - if the ACL is swift.Private,
		// the machine won't be able to get the tools (401 error)
		containerACL: swift.PublicRead,
		swift:        swift.New(e.client)}
	e.publicStorageUnlocked = nil
	return nil
}

// getImageBaseURLs returns a list of URLs which are used to search for simplestreams image metadata.
func (e *environ) getImageBaseURLs() ([]string, error) {
	e.imageBaseMutex.Lock()
	defer e.imageBaseMutex.Unlock()

	if e.imageBaseURLs != nil {
		return e.imageBaseURLs, nil
	}
	if !e.client.IsAuthenticated() {
		err := e.client.Authenticate()
		if err != nil {
			return nil, err
		}
	}
	// Add the simplestreams base URL off the public bucket.
	jujuToolsURL, err := e.PublicStorage().URL("")
	if err == nil {
		e.imageBaseURLs = append(e.imageBaseURLs, jujuToolsURL)
	}
	// Add the simplestreams base URL from keystone if it is defined.
	productStreamsURL, err := e.client.MakeServiceURL("product-streams", nil)
	if err == nil {
		e.imageBaseURLs = append(e.imageBaseURLs, productStreamsURL)
	}
	// Add the default simplestreams base URL.
	e.imageBaseURLs = append(e.imageBaseURLs, imagemetadata.DefaultBaseURL)

	return e.imageBaseURLs, nil
}

// TODO(bug 1199847): This work can be shared between providers.
func (e *environ) StartInstance(machineId, machineNonce string, series string, cons constraints.Value,
	stateInfo *state.Info, apiInfo *api.Info) (instance.Instance, *instance.HardwareCharacteristics, error) {
	possibleTools, err := environs.FindInstanceTools(e, series, cons)
	if err != nil {
		return nil, nil, err
	}
	err = environs.CheckToolsSeries(possibleTools, series)
	if err != nil {
		return nil, nil, err
	}

	machineConfig := environs.NewMachineConfig(machineId, machineNonce, stateInfo, apiInfo)
	return e.internalStartInstance(cons, possibleTools, machineConfig, e.ecfg().useFloatingIP())
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
	for a := environs.LongAttempt.Start(); a.Next(); {
		err = e.nova().AddServerFloatingIP(serverId, fip.IP)
		if err == nil {
			return nil
		}
	}
	return err
}

// internalStartInstance is the internal version of StartInstance, used by
// Bootstrap as well as via StartInstance itself.
// Setting withPublicIP to true causes a floating IP to be assigned to the
// server after starting.
// machineConfig will be filled out with further details, but should contain
// MachineID, MachineNonce, StateInfo, and APIInfo.
// TODO(bug 1199847): Some of this work can be shared between providers.
func (e *environ) internalStartInstance(cons constraints.Value, possibleTools tools.List, machineConfig *cloudinit.MachineConfig, withPublicIP bool) (instance.Instance, *instance.HardwareCharacteristics, error) {
	series := possibleTools.Series()
	if len(series) != 1 {
		panic(fmt.Errorf("should have gotten tools for one series, got %v", series))
	}
	arches := possibleTools.Arches()
	spec, err := findInstanceSpec(e, &instances.InstanceConstraint{
		Region:      e.ecfg().region(),
		Series:      series[0],
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
	log.Debugf("environs/openstack: openstack user data; %d bytes", len(userData))

	var publicIP *nova.FloatingIP
	if withPublicIP {
		if fip, err := e.allocatePublicIP(); err != nil {
			return nil, nil, fmt.Errorf("cannot allocate a public IP as needed: %v", err)
		} else {
			publicIP = fip
			log.Infof("environs/openstack: allocated public IP %s", publicIP.IP)
		}
	}
	config := e.Config()
	groups, err := e.setUpGroups(machineConfig.MachineId, config.StatePort(), config.APIPort())
	if err != nil {
		return nil, nil, fmt.Errorf("cannot set up groups: %v", err)
	}
	var groupNames = make([]nova.SecurityGroupName, len(groups))
	for i, g := range groups {
		groupNames[i] = nova.SecurityGroupName{g.Name}
	}

	var server *nova.Entity
	for a := shortAttempt.Start(); a.Next(); {
		server, err = e.nova().RunServer(nova.RunServerOpts{
			Name:               e.machineFullName(machineConfig.MachineId),
			FlavorId:           spec.InstanceType.Id,
			ImageId:            spec.Image.Id,
			UserData:           userData,
			SecurityGroupNames: groupNames,
		})
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
		ServerDetail: detail,
		arch:         &spec.Image.Arch,
		instType:     &spec.InstanceType,
	}
	log.Infof("environs/openstack: started instance %q", inst.Id())
	if withPublicIP {
		if err := e.assignPublicIP(publicIP, string(inst.Id())); err != nil {
			if err := e.terminateInstances([]instance.Id{inst.Id()}); err != nil {
				// ignore the failure at this stage, just log it
				log.Debugf("environs/openstack: failed to terminate instance %q: %v", inst.Id(), err)
			}
			return nil, nil, fmt.Errorf("cannot assign public address %s to instance %q: %v", publicIP.IP, inst.Id(), err)
		}
		log.Infof("environs/openstack: assigned public IP %s to %q", publicIP.IP, inst.Id())
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
	log.Debugf("environs/openstack: terminating instances %v", ids)
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
			if server.Status == nova.StatusActive || server.Status == nova.StatusBuild {
				// TODO(wallyworld): lookup the flavor details to fill in the instance type data
				out[id] = &openstackInstance{e: e, ServerDetail: &server}
			}
			continue
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
				ServerDetail: &s,
			})
		}
	}
	return insts, err
}

func (e *environ) Destroy(ensureInsts []instance.Instance) error {
	log.Infof("environs/openstack: destroying environment %q", e.name)
	insts, err := e.AllInstances()
	if err != nil {
		return fmt.Errorf("cannot get instances: %v", err)
	}
	found := make(map[instance.Id]bool)
	var ids []instance.Id
	for _, inst := range insts {
		ids = append(ids, inst.Id())
		found[inst.Id()] = true
	}

	// Add any instances we've been told about but haven't yet shown
	// up in the instance list.
	for _, inst := range ensureInsts {
		id := instance.Id(inst.(*openstackInstance).Id())
		if !found[id] {
			ids = append(ids, id)
			found[id] = true
		}
	}
	err = e.terminateInstances(ids)
	if err != nil {
		return err
	}

	return e.Storage().RemoveAll()
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
	return fmt.Sprintf("juju-%s-%s", e.Name(), state.MachineTag(machineId))
}

// machinesFilter returns a nova.Filter matching all machines in the environment.
func (e *environ) machinesFilter() *nova.Filter {
	filter := nova.NewFilter()
	filter.Set(nova.FilterServer, fmt.Sprintf("juju-%s-.*", e.Name()))
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
			log.Debugf("error creating security group rule: %v", err.Error())
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
	state.SortPorts(ports)
	return ports, nil
}

// TODO: following 30 lines nearly verbatim from environs/ec2

func (e *environ) OpenPorts(ports []instance.Port) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode for opening ports on environment: %q",
			e.Config().FirewallMode())
	}
	if err := e.openPortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	log.Infof("environs/openstack: opened ports in global group: %v", ports)
	return nil
}

func (e *environ) ClosePorts(ports []instance.Port) error {
	if e.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode for closing ports on environment: %q",
			e.Config().FirewallMode())
	}
	if err := e.closePortsInGroup(e.globalGroupName(), ports); err != nil {
		return err
	}
	log.Infof("environs/openstack: closed ports in global group: %v", ports)
	return nil
}

func (e *environ) Ports() ([]instance.Port, error) {
	if e.Config().FirewallMode() != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode for retrieving ports from environment: %q",
			e.Config().FirewallMode())
	}
	return e.portsInGroup(e.globalGroupName())
}

func (e *environ) Provider() environs.EnvironProvider {
	return &providerInstance
}

// setUpGroups creates the security groups for the new machine, and
// returns them.
//
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same OpenStack account.
// In addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
func (e *environ) setUpGroups(machineId string, statePort, apiPort int) ([]nova.SecurityGroup, error) {
	jujuGroup, err := e.ensureGroup(e.jujuGroupName(),
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
	return []nova.SecurityGroup{jujuGroup, machineGroup}, nil
}

// zeroGroup holds the zero security group.
var zeroGroup nova.SecurityGroup

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
func (e *environ) ensureGroup(name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
	novaClient := e.nova()
	group, err := novaClient.CreateSecurityGroup(name, "juju group")
	if err != nil {
		if !gooseerrors.IsDuplicateValue(err) {
			return zeroGroup, err
		} else {
			// We just tried to create a duplicate group, so load the existing group.
			group, err = novaClient.SecurityGroupByName(name)
			if err != nil {
				return zeroGroup, err
			}
		}
	}
	// The group is created so now add the rules.
	for _, rule := range rules {
		rule.ParentGroupId = group.Id
		_, err := novaClient.CreateSecurityGroupRule(rule)
		if err != nil && !gooseerrors.IsDuplicateValue(err) {
			return zeroGroup, err
		}
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
			log.Debugf("environs/openstack: error terminating instance %q: %v", id, err)
			firstErr = err
		}
	}
	return firstErr
}
